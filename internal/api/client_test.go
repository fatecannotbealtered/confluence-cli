package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	_ = os.Setenv("CONFLUENCE_CLI_RETRY_BASE_MS", "0")
}

func newTestClient(serverURL string) *Client {
	return NewClient(serverURL, "test-pat-token", Options{Version: "1.2.3"})
}

func TestNewClient(t *testing.T) {
	c := NewClient("https://confluence.example.com/", "  mytoken  ", Options{})
	if c.baseURL != "https://confluence.example.com" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", c.baseURL)
	}
	if c.authHeader != "Bearer mytoken" {
		t.Errorf("authHeader = %q", c.authHeader)
	}
	if c.userAgent != "confluence-cli/dev" {
		t.Errorf("userAgent = %q", c.userAgent)
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
	if c.Content == nil || c.Search == nil || c.Spaces == nil || c.Attachments == nil ||
		c.Comments == nil || c.Labels == nil || c.Users == nil || c.LongTasks == nil || c.System == nil {
		t.Error("API group fields should be initialized")
	}
	if c.BaseURL() != "https://confluence.example.com" {
		t.Errorf("BaseURL() = %q", c.BaseURL())
	}
}

func TestNewClient_CustomTimeoutAndVersion(t *testing.T) {
	c := NewClient("https://x.example.com", "t", Options{Timeout: 5 * time.Second, Version: "9.9.9"})
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v", c.httpClient.Timeout)
	}
	if c.userAgent != "confluence-cli/9.9.9" {
		t.Errorf("userAgent = %q", c.userAgent)
	}
}

func TestRequestHeaders(t *testing.T) {
	var gotAuth, gotUA, gotAccept string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.get("/rest/api/user/current"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-pat-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotUA != "confluence-cli/1.2.3" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestRedirectPreservesAuthorization(t *testing.T) {
	var sawAuth atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/first" {
			http.Redirect(w, r, "/second", http.StatusFound)
			return
		}
		sawAuth.Store(r.Header.Get("Authorization"))
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.get("/first"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := sawAuth.Load().(string); got != "Bearer test-pat-token" {
		t.Errorf("Authorization after redirect = %q", got)
	}
}

// statusServer replies with the given status and body on every request.
func statusServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
}

func asAPIError(t *testing.T, err error) *APIError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	return apiErr
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status    int
		wantCode  string
		retryable bool
	}{
		{400, "E_USAGE", false},
		{401, "E_AUTH", false},
		{403, "E_FORBIDDEN", false},
		{404, "E_NOT_FOUND", false},
		{408, "E_TIMEOUT", true},
		{409, "E_CONFLICT", false},
		{429, "E_RATE_LIMITED", true},
		{500, "E_SERVER", true},
		{503, "E_SERVER", true},
		{418, "E_UNKNOWN", false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d->%s", tc.status, tc.wantCode), func(t *testing.T) {
			ts := statusServer(tc.status, `{"statusCode":`+fmt.Sprint(tc.status)+`,"message":"server says no"}`)
			defer ts.Close()

			c := newTestClient(ts.URL)
			_, err := c.get("/rest/api/content/1")
			apiErr := asAPIError(t, err)
			if apiErr.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", apiErr.Code, tc.wantCode)
			}
			if apiErr.Retryable != tc.retryable {
				t.Errorf("Retryable = %v, want %v", apiErr.Retryable, tc.retryable)
			}
			if apiErr.Status != tc.status {
				t.Errorf("Status = %d, want %d", apiErr.Status, tc.status)
			}
			if apiErr.Message != "server says no" {
				t.Errorf("Message = %q, want server message passthrough", apiErr.Message)
			}
			if apiErr.Details["server_message"] != "server says no" {
				t.Errorf("Details[server_message] = %v", apiErr.Details["server_message"])
			}
			if apiErr.Details["http_status"] != tc.status {
				t.Errorf("Details[http_status] = %v", apiErr.Details["http_status"])
			}
		})
	}
}

func TestErrorMapping_CQLSyntaxError(t *testing.T) {
	// The 400 body's server message must survive into Details (CQL errors).
	ts := statusServer(400, `{"statusCode":400,"message":"Could not parse cql : space = ","reason":"Bad Request"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Search.Search("space = ", SearchOptions{})
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_USAGE" {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "Could not parse cql") {
		t.Errorf("Message = %q, want CQL parse message", apiErr.Message)
	}
	if apiErr.Details["server_reason"] != "Bad Request" {
		t.Errorf("Details[server_reason] = %v", apiErr.Details["server_reason"])
	}
}

func TestErrorMapping_EmptyBodyFallbackMessage(t *testing.T) {
	ts := statusServer(404, ``)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.get("/rest/api/content/1")
	apiErr := asAPIError(t, err)
	if apiErr.Message != "resource not found" {
		t.Errorf("Message = %q", apiErr.Message)
	}
	if !strings.Contains(apiErr.Error(), "E_NOT_FOUND") || !strings.Contains(apiErr.Error(), "404") {
		t.Errorf("Error() = %q", apiErr.Error())
	}
}

func TestErrorMapping_NetworkError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // closed: connection refused

	c := newTestClient(ts.URL)
	_, err := c.get("/rest/api/content/1")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_NETWORK" {
		t.Errorf("Code = %q, want E_NETWORK", apiErr.Code)
	}
	if !apiErr.Retryable {
		t.Error("network error should be retryable")
	}
	if apiErr.Status != 0 {
		t.Errorf("Status = %d, want 0", apiErr.Status)
	}
	if !strings.Contains(apiErr.Error(), "E_NETWORK") {
		t.Errorf("Error() = %q", apiErr.Error())
	}
}

func TestErrorMapping_ClientTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "t", Options{Timeout: 20 * time.Millisecond})
	_, err := c.get("/rest/api/content/1")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_TIMEOUT" {
		t.Errorf("Code = %q, want E_TIMEOUT", apiErr.Code)
	}
	if !apiErr.Retryable {
		t.Error("timeout should be retryable")
	}
}

func TestRetry_429ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	data, err := c.get("/rest/api/content/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
	var body map[string]bool
	if err := json.Unmarshal(data, &body); err != nil || !body["ok"] {
		t.Errorf("body = %s", data)
	}
}

func TestRetry_500ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.get("/rest/api/content/1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestRetry_ExhaustedReturnsError(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.get("/rest/api/content/1")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_SERVER" {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if got := calls.Load(); got != int32(defaultMaxRetries)+1 {
		t.Errorf("calls = %d, want %d", got, defaultMaxRetries+1)
	}
}

func TestRetry_NoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, _ = c.get("/rest/api/content/1")
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 400)", calls.Load())
	}
}

func TestRetryAfterWait(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "2")
	if got := retryAfterWait(h); got != 2*time.Second {
		t.Errorf("retryAfterWait = %v, want 2s", got)
	}
	if got := retryAfterWait(http.Header{}); got != retryBaseWait() {
		t.Errorf("retryAfterWait without header = %v", got)
	}
}

func TestMaxRetriesEnv(t *testing.T) {
	t.Setenv("CONFLUENCE_CLI_MAX_RETRIES", "0")
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, _ = c.get("/rest/api/content/1")
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 with MAX_RETRIES=0", calls.Load())
	}
}

func TestDownload_RefusesForeignHost(t *testing.T) {
	c := newTestClient("https://confluence.example.com")
	_, err := c.download("https://evil.example.com/steal")
	if err == nil || !strings.Contains(err.Error(), "refusing to download") {
		t.Errorf("err = %v, want SSRF refusal", err)
	}
}
