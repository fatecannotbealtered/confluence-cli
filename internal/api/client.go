// Package api implements a Confluence Data Center REST v1 HTTP client.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/contract"
)

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
)

func maxRetries() int {
	if s := strings.TrimSpace(os.Getenv("CONFLUENCE_CLI_MAX_RETRIES")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return n
		}
	}
	return defaultMaxRetries
}

func retryBaseWait() time.Duration {
	if s := strings.TrimSpace(os.Getenv("CONFLUENCE_CLI_RETRY_BASE_MS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return time.Second
}

func retryAfterWait(h http.Header) time.Duration {
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return retryBaseWait()
}

// APIError maps a Confluence DC failure onto the canonical E_* taxonomy.
type APIError struct {
	Code      string         // E_* code from the contract table
	Message   string         // human readable; server message when present
	Details   map[string]any // structured context (http_status, server_message, ...)
	Retryable bool
	Status    int // upstream HTTP status; 0 for pure network errors
}

func (e *APIError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("confluence api %s (HTTP %d): %s", e.Code, e.Status, e.Message)
	}
	return fmt.Sprintf("confluence api %s: %s", e.Code, e.Message)
}

// confluenceErrorBody mirrors the DC error payload:
// {"statusCode":400,"message":"...","reason":"...","data":{...}}.
type confluenceErrorBody struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Reason     string `json:"reason"`
}

// mapStatusError converts an HTTP status + body into an APIError. Single
// mapping point so status->code->retryable cannot drift.
func mapStatusError(status int, body []byte) *APIError {
	var eb confluenceErrorBody
	if len(body) > 0 {
		_ = json.Unmarshal(body, &eb)
	}

	var code, fallback string
	switch {
	case status == http.StatusBadRequest:
		code, fallback = "E_USAGE", "bad request"
	case status == http.StatusUnauthorized:
		code, fallback = "E_AUTH", "authentication failed: check your PAT (Bearer token)"
	case status == http.StatusForbidden:
		code, fallback = "E_FORBIDDEN", "permission denied: check your PAT permissions"
	case status == http.StatusNotFound:
		code, fallback = "E_NOT_FOUND", "resource not found"
	case status == http.StatusRequestTimeout:
		code, fallback = "E_TIMEOUT", "upstream request timeout"
	case status == http.StatusConflict:
		code, fallback = "E_CONFLICT", "conflict: resource changed concurrently"
	case status == http.StatusTooManyRequests:
		code, fallback = "E_RATE_LIMITED", "rate limited by Confluence"
	case status >= 500:
		code, fallback = "E_SERVER", "Confluence server error"
	default:
		code, fallback = "E_UNKNOWN", fmt.Sprintf("unexpected status code %d", status)
	}

	details := map[string]any{"http_status": status}
	// Pass the server-side message through (important for CQL syntax errors).
	if eb.Message != "" {
		details["server_message"] = eb.Message
	}
	if eb.Reason != "" {
		details["server_reason"] = eb.Reason
	}

	msg := fallback
	if eb.Message != "" {
		msg = eb.Message
	}

	return &APIError{
		Code:      code,
		Message:   msg,
		Details:   details,
		Retryable: contract.Retryable(code),
		Status:    status,
	}
}

// mapNetworkError converts a transport-level failure into an APIError.
func mapNetworkError(err error) *APIError {
	code := "E_NETWORK"
	if ue, ok := err.(*url.Error); ok && ue.Timeout() {
		code = "E_TIMEOUT"
	}
	return &APIError{
		Code:      code,
		Message:   err.Error(),
		Details:   map[string]any{},
		Retryable: contract.Retryable(code),
	}
}

// notFoundError synthesizes an E_NOT_FOUND for client-side "no match" cases
// (e.g. an exact-match CQL lookup that returned zero results).
func notFoundError(msg string) *APIError {
	return &APIError{
		Code:      "E_NOT_FOUND",
		Message:   msg,
		Details:   map[string]any{"http_status": http.StatusNotFound},
		Retryable: false,
		Status:    http.StatusNotFound,
	}
}

// Options configures the client at construction time.
type Options struct {
	Timeout time.Duration // defaults to 30s
	Version string        // CLI version for the User-Agent, defaults to "dev"
}

// Client wraps the Confluence Data Center REST API v1.
type Client struct {
	baseURL    string // no trailing slash
	baseHost   string // lowercased scheme://host of baseURL, for redirect host checks
	authHeader string // "Bearer <PAT>"
	userAgent  string
	httpClient *http.Client

	Content     *ContentAPI
	Search      *SearchAPI
	Spaces      *SpaceAPI
	Attachments *AttachmentAPI
	Comments    *CommentAPI
	Labels      *LabelAPI
	Users       *UserAPI
	LongTasks   *LongTaskAPI
	System      *SystemAPI
}

// originOf returns the lowercased "scheme://host" of a URL, or "" if it cannot
// be parsed. Used to compare redirect targets against the configured base host
// so credentials are never re-attached across origins.
func originOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// NewClient creates a Confluence DC client with PAT Bearer authentication.
func NewClient(baseURL, token string, opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	version := opts.Version
	if version == "" {
		version = "dev"
	}

	trimmedBase := strings.TrimRight(baseURL, "/")
	c := &Client{
		baseURL:    trimmedBase,
		baseHost:   originOf(trimmedBase),
		authHeader: "Bearer " + strings.TrimSpace(token),
		userAgent:  "confluence-cli/" + version,
	}
	c.httpClient = &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if len(via) > 0 {
				// Go strips Authorization on cross-origin redirects to avoid
				// leaking credentials. Re-apply the Bearer PAT ONLY when the
				// redirect target is the same origin as the configured base
				// host, so an SSO hop within the instance keeps working while
				// a redirect to a foreign host never receives the PAT.
				if originOf(req.URL.String()) == c.baseHost {
					if auth := via[len(via)-1].Header.Get("Authorization"); auth != "" {
						req.Header.Set("Authorization", auth)
					}
				} else {
					req.Header.Del("Authorization")
				}
				if ua := via[len(via)-1].Header.Get("User-Agent"); ua != "" {
					req.Header.Set("User-Agent", ua)
				}
			}
			return nil
		},
	}

	c.Content = &ContentAPI{client: c}
	c.Search = &SearchAPI{client: c}
	c.Spaces = &SpaceAPI{client: c}
	c.Attachments = &AttachmentAPI{client: c}
	c.Comments = &CommentAPI{client: c}
	c.Labels = &LabelAPI{client: c}
	c.Users = &UserAPI{client: c}
	c.LongTasks = &LongTaskAPI{client: c}
	c.System = &SystemAPI{client: c}

	return c
}

// BaseURL returns the configured Confluence base URL (no trailing slash).
func (c *Client) BaseURL() string { return c.baseURL }

func restPath(resource string) string { return "/rest/api" + resource }

func (c *Client) applyHeaders(req *http.Request, jsonBody bool) {
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if jsonBody {
		req.Header.Set("Content-Type", "application/json")
	}
}

// doWithRetry executes a request with retry logic (429 Retry-After, 5xx
// exponential backoff) and maps failures onto APIError.
func (c *Client) doWithRetry(ctx context.Context, method, path string, body any) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		var reqBody io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("encoding request body: %w", err)
			}
			reqBody = bytes.NewReader(encoded)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		c.applyHeaders(req, body != nil)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, mapNetworkError(err)
		}

		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, mapNetworkError(readErr)
		}

		status := resp.StatusCode
		if status < 400 {
			return data, nil
		}

		// HTTP 429: rate limited, honor Retry-After.
		if status == http.StatusTooManyRequests {
			if attempt >= maxRetries() {
				return nil, mapStatusError(status, data)
			}
			select {
			case <-time.After(retryAfterWait(resp.Header)):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}

		// HTTP 5xx: exponential backoff.
		if status >= 500 {
			if attempt >= maxRetries() {
				return nil, mapStatusError(status, data)
			}
			select {
			case <-time.After(time.Duration(1<<uint(attempt)) * retryBaseWait()):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}

		// Other 4xx: do not retry.
		return nil, mapStatusError(status, data)
	}
}

func (c *Client) get(path string) ([]byte, error) {
	return c.doWithRetry(context.Background(), http.MethodGet, path, nil)
}

func (c *Client) post(path string, body any) ([]byte, error) {
	return c.doWithRetry(context.Background(), http.MethodPost, path, body)
}

func (c *Client) put(path string, body any) ([]byte, error) {
	return c.doWithRetry(context.Background(), http.MethodPut, path, body)
}

func (c *Client) del(path string) ([]byte, error) {
	return c.doWithRetry(context.Background(), http.MethodDelete, path, nil)
}

// uploadMultipart POSTs files as multipart/form-data with the mandatory
// X-Atlassian-Token: no-check header (XSRF protection bypass for uploads).
func (c *Client) uploadMultipart(path string, files []string, comment string, minorEdit bool) ([]byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("opening file %s: %w", file, err)
		}
		part, err := w.CreateFormFile("file", filepath.Base(file))
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("creating form file: %w", err)
		}
		if _, err := io.Copy(part, f); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("copying file content: %w", err)
		}
		_ = f.Close()
	}
	if comment != "" {
		if err := w.WriteField("comment", comment); err != nil {
			return nil, fmt.Errorf("writing comment field: %w", err)
		}
	}
	if minorEdit {
		if err := w.WriteField("minorEdit", "true"); err != nil {
			return nil, fmt.Errorf("writing minorEdit field: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return nil, fmt.Errorf("creating upload request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "no-check")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, mapNetworkError(err)
	}
	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, mapNetworkError(readErr)
	}
	if resp.StatusCode >= 400 {
		return nil, mapStatusError(resp.StatusCode, data)
	}
	return data, nil
}

// download streams a resource. rawURL may be a host-relative path ("/...") or
// an absolute URL, which must point at the configured host (SSRF guard). The
// caller must Close the returned reader.
func (c *Client) download(rawURL string) (io.ReadCloser, error) {
	target := rawURL
	switch {
	case strings.HasPrefix(rawURL, "/"):
		target = c.baseURL + rawURL
	case strings.HasPrefix(rawURL, c.baseURL+"/"), rawURL == c.baseURL:
		// same host, allowed as-is
	default:
		return nil, fmt.Errorf("refusing to download from URL outside %s", c.baseURL)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("User-Agent", c.userAgent)

	// Large attachments can exceed the shared client timeout; stream with no
	// overall deadline. A redirect off the configured origin is refused on
	// every hop: the download link is server-supplied (untrusted), so an
	// initial-URL SSRF check is not enough — a same-host link could still 302
	// to an internal address. The shared CheckRedirect additionally strips the
	// PAT from any foreign hop we might otherwise follow.
	dl := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if originOf(req.URL.String()) != c.baseHost {
			return fmt.Errorf("refusing to follow download redirect off %s", c.baseURL)
		}
		return c.httpClient.CheckRedirect(req, via)
	}}
	resp, err := dl.Do(req)
	if err != nil {
		return nil, mapNetworkError(err)
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, mapStatusError(resp.StatusCode, data)
	}
	return resp.Body, nil
}

// ===== API group types (methods implemented in their own files) =====

// ContentAPI wraps page/blogpost content calls.
type ContentAPI struct{ client *Client }

// SearchAPI wraps CQL search calls.
type SearchAPI struct{ client *Client }

// SpaceAPI wraps space calls.
type SpaceAPI struct{ client *Client }

// AttachmentAPI wraps attachment calls.
type AttachmentAPI struct{ client *Client }

// CommentAPI wraps comment calls.
type CommentAPI struct{ client *Client }

// LabelAPI wraps label calls.
type LabelAPI struct{ client *Client }

// UserAPI wraps user calls.
type UserAPI struct{ client *Client }

// LongTaskAPI wraps long-running task calls.
type LongTaskAPI struct{ client *Client }

// SystemAPI wraps system/settings calls.
type SystemAPI struct{ client *Client }
