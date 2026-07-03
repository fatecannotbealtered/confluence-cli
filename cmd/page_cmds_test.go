package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// readAll drains a request body to a string.
func readAll(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

// decodeRaw unmarshals the full envelope JSON into v without ok assertions.
func decodeRaw(t *testing.T, out string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
}

// pageServer builds a mock DC that always answers /user/current and dispatches
// the remaining paths through the caller's handler.
func pageServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
			return
		}
		handler(w, r)
	})
}

// ─── page get ────────────────────────────────────────────────────────────────

func TestPageGet_MarkdownRoundtrip(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/api/content/") {
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","status":"current","title":"Roadmap","space":{"key":"ENG"},"version":{"number":3},"body":{"storage":{"value":"<h1>Hello</h1><p>world</p>","representation":"storage"}},"_links":{"webui":"/display/ENG/Roadmap"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "get", "12345")
	var data struct {
		ID         string         `json:"id"`
		Body       string         `json:"body"`
		BodyFormat string         `json:"body_format"`
		URL        string         `json:"url"`
		Version    int            `json:"version"`
		Meta       map[string]any `json:"meta"`
		Untrusted  []string       `json:"_untrusted"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.ID != "12345" || data.Version != 3 {
		t.Fatalf("data=%+v", data)
	}
	if data.BodyFormat != "markdown" || !strings.Contains(data.Body, "Hello") {
		t.Fatalf("body=%q format=%q", data.Body, data.BodyFormat)
	}
	if !strings.HasSuffix(data.URL, "/display/ENG/Roadmap") {
		t.Fatalf("url=%q", data.URL)
	}
	if data.Meta["fidelity"] == nil {
		t.Fatalf("missing fidelity meta")
	}
	if !containsStr(data.Untrusted, "body") || !containsStr(data.Untrusted, "title") {
		t.Fatalf("untrusted=%v", data.Untrusted)
	}
}

func TestPageGet_StoragePassthrough(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"1","type":"page","title":"X","body":{"storage":{"value":"<p>raw</p>","representation":"storage"}},"version":{"number":1},"_links":{"webui":"/x"}}`))
	})
	stdout, _ := runRootOK(t, "page", "get", "1", "--body-format", "storage")
	var data struct {
		Body       string `json:"body"`
		BodyFormat string `json:"body_format"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Body != "<p>raw</p>" || data.BodyFormat != "storage" {
		t.Fatalf("data=%+v", data)
	}
}

func TestPageGet_BySpaceTitle(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/content/search" {
			_, _ = w.Write([]byte(`{"results":[{"id":"77","type":"page","title":"Roadmap","space":{"key":"ENG"},"version":{"number":1},"body":{"storage":{"value":"<p>hi</p>","representation":"storage"}},"_links":{"webui":"/display/ENG/Roadmap"}}],"start":0,"limit":1,"size":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "get", "--space", "ENG", "--title", "Roadmap")
	var data struct {
		ID string `json:"id"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.ID != "77" {
		t.Fatalf("data=%+v", data)
	}
}

func TestPageGet_IDAndLocatorConflict(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "page", "get", "12345", "--space", "ENG")
	if decodeEnvelopeError(t, stdout)["code"] != "E_VALIDATION" {
		t.Fatal("want E_VALIDATION")
	}
}

// ─── page list ───────────────────────────────────────────────────────────────

func TestPageList_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/search" {
			_, _ = w.Write([]byte(`{"results":[{"title":"Home","content":{"id":"1","type":"page","_links":{"webui":"/display/ENG/Home"}}}],"start":0,"limit":25,"size":1,"totalSize":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "list", "--space", "ENG")
	var data struct {
		Pages []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"pages"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Pages) != 1 || data.Pages[0].ID != "1" {
		t.Fatalf("pages=%+v", data.Pages)
	}
}

func TestPageList_RequiresSpace(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "page", "list")
	if decodeEnvelopeError(t, stdout)["code"] != "E_VALIDATION" {
		t.Fatal("want E_VALIDATION")
	}
}

// ─── page create (write gate + markdown conversion) ──────────────────────────

func TestPageCreate_TwoStepGateAndConversion(t *testing.T) {
	var gotStorage string
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/content" && r.Method == http.MethodPost {
			body := readAll(r)
			gotStorage = body
			_, _ = w.Write([]byte(`{"id":"500","type":"page","status":"current","title":"Notes","space":{"key":"ENG"},"version":{"number":1},"_links":{"webui":"/display/ENG/Notes"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	token := dryRunConfirmToken(t, "page", "create", "--space", "ENG", "--title", "Notes", "--body", "# Hi")
	stdout, _ := runRootOK(t, "--confirm", token, "page", "create", "--space", "ENG", "--title", "Notes", "--body", "# Hi")
	var data struct {
		ID string `json:"id"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.ID != "500" {
		t.Fatalf("data=%+v", data)
	}
	if !strings.Contains(gotStorage, "storage") || strings.Contains(gotStorage, "# Hi") {
		t.Fatalf("expected markdown converted to storage, got %s", gotStorage)
	}
}

func TestPageCreate_DryRunPreviewHasStorage(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	stdout, _, err := runRoot(t, "--dry-run", "page", "create", "--space", "ENG", "--title", "Notes", "--body", "# Hi")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(stdout, "storage_preview") {
		t.Fatalf("preview missing storage_preview:\n%s", stdout)
	}
}

// ─── page update (optimistic lock) ───────────────────────────────────────────

func TestPageUpdate_VersionDriftConflict(t *testing.T) {
	calls := 0
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodGet {
			calls++
			ver := 3
			if calls >= 2 {
				ver = 4 // drift after dry-run read
			}
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"T","version":{"number":` + strconv.Itoa(ver) + `}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	token := dryRunConfirmToken(t, "page", "update", "12345", "--title", "New")
	stdout, _ := runRootExpectSilent(t, ExitConflict, "--confirm", token, "page", "update", "12345", "--title", "New")
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFLICT" {
		t.Fatal("want E_CONFLICT")
	}
}

func TestPageUpdate_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"Old","version":{"number":3}}`))
		case strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodPut:
			body := readAll(r)
			if !strings.Contains(body, `"number":4`) {
				t.Errorf("expected version 4 in PUT: %s", body)
			}
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","status":"current","title":"New","version":{"number":4},"_links":{"webui":"/x"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	token := dryRunConfirmToken(t, "page", "update", "12345", "--title", "New")
	stdout, _ := runRootOK(t, "--confirm", token, "page", "update", "12345", "--title", "New")
	var data struct {
		Version int `json:"version"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Version != 4 {
		t.Fatalf("data=%+v", data)
	}
}

// ─── page delete (dangerous double gate + purge + descendant count) ──────────

func TestPageDelete_RequiresDangerousInDryRun(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/api/content/") {
			if strings.HasSuffix(r.URL.Path, "/descendant/page") {
				_, _ = w.Write([]byte(`{"results":[],"start":0,"limit":25,"size":0,"_links":{}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"Doomed"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "--dry-run", "page", "delete", "12345")
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatal("want E_CONFIRMATION_REQUIRED")
	}
}

func TestPageDelete_PurgeDoubleGate(t *testing.T) {
	deletes := 0
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/descendant/page"):
			_, _ = w.Write([]byte(`{"results":[{"id":"a"},{"id":"b"}],"start":0,"limit":25,"size":2,"_links":{}}`))
		case r.Method == http.MethodDelete:
			deletes++
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/rest/api/content/"):
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"Doomed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	// Dry-run preview must include descendant count and irreversible flag.
	stdout, _, err := runRoot(t, "--dangerous", "--dry-run", "page", "delete", "12345", "--purge")
	if err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	if !strings.Contains(stdout, "descendants") || !strings.Contains(stdout, "irreversible") {
		t.Fatalf("preview missing descendants/irreversible:\n%s", stdout)
	}
	token := dryRunConfirmToken(t, "--dangerous", "page", "delete", "12345", "--purge")
	stdout, _ = runRootOK(t, "--dangerous", "--confirm", token, "page", "delete", "12345", "--purge")
	var data struct {
		Purged bool `json:"purged"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if !data.Purged {
		t.Fatalf("data=%+v", data)
	}
	if deletes != 2 { // trash + purge
		t.Fatalf("expected 2 DELETEs, got %d", deletes)
	}
}

// ─── page move / children / ancestors / history / restore ────────────────────

func TestPageMove_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"P","version":{"number":2}}`))
		case r.Method == http.MethodPut:
			body := readAll(r)
			if !strings.Contains(body, "ancestors") || !strings.Contains(body, "67890") {
				t.Errorf("PUT missing new ancestor: %s", body)
			}
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"P","version":{"number":3},"_links":{"webui":"/x"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	token := dryRunConfirmToken(t, "page", "move", "12345", "--parent", "67890")
	stdout, _ := runRootOK(t, "--confirm", token, "page", "move", "12345", "--parent", "67890")
	var data struct {
		Version int `json:"version"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Version != 3 {
		t.Fatalf("data=%+v", data)
	}
}

func TestPageChildren_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/child/page") {
			_, _ = w.Write([]byte(`{"results":[{"id":"2","type":"page","title":"Child","_links":{"webui":"/c"}}],"start":0,"limit":25,"size":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "children", "1")
	var data struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Pages) != 1 || data.Pages[0].ID != "2" {
		t.Fatalf("pages=%+v", data.Pages)
	}
}

func TestPageAncestors_Breadcrumb(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"3","type":"page","title":"Leaf","ancestors":[{"id":"1","title":"Root","_links":{"webui":"/r"}},{"id":"2","title":"Mid","_links":{"webui":"/m"}}]}`))
	})
	stdout, _ := runRootOK(t, "page", "ancestors", "3")
	var data struct {
		Ancestors []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"ancestors"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Ancestors) != 2 || data.Ancestors[0].ID != "1" {
		t.Fatalf("ancestors=%+v", data.Ancestors)
	}
}

func TestPageHistory_Versions(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/history") {
			_, _ = w.Write([]byte(`{"latest":true,"createdBy":{"displayName":"Jane"},"createdDate":"2026-01-01","lastUpdated":{"number":5,"when":"2026-06-01","message":"edit","by":{"displayName":"Jane"}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "history", "12345")
	var data struct {
		Untrusted []string `json:"_untrusted"`
		Versions  []struct {
			Number    int      `json:"number"`
			Untrusted []string `json:"_untrusted"`
		} `json:"versions"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Versions) != 1 || data.Versions[0].Number != 5 {
		t.Fatalf("versions=%+v", data.Versions)
	}
	// External author/comment content must be tagged _untrusted.
	if !contains(data.Untrusted, "created_by") {
		t.Errorf("top-level _untrusted missing created_by: %v", data.Untrusted)
	}
	if !contains(data.Versions[0].Untrusted, "by") || !contains(data.Versions[0].Untrusted, "message") {
		t.Errorf("version _untrusted missing by/message: %v", data.Versions[0].Untrusted)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestPageRestore_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.RawQuery, "status=historical"):
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"OldTitle","body":{"storage":{"value":"<p>old</p>","representation":"storage"}}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/rest/api/content/"):
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"Cur","version":{"number":5}}`))
		case r.Method == http.MethodPut:
			body := readAll(r)
			if !strings.Contains(body, "old") || !strings.Contains(body, `"number":6`) {
				t.Errorf("restore PUT wrong: %s", body)
			}
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"OldTitle","version":{"number":6},"_links":{"webui":"/x"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	token := dryRunConfirmToken(t, "page", "restore", "12345", "--version", "2")
	stdout, _ := runRootOK(t, "--confirm", token, "page", "restore", "12345", "--version", "2")
	var data struct {
		Version int `json:"version"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Version != 6 {
		t.Fatalf("data=%+v", data)
	}
}

// ─── page comment ────────────────────────────────────────────────────────────

func TestPageCommentList_LocationFilter(t *testing.T) {
	var gotLocation string
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/child/comment") {
			gotLocation = r.URL.Query().Get("location")
			_, _ = w.Write([]byte(`{"results":[{"id":"9","title":"Re","body":{"storage":{"value":"<p>ok</p>","representation":"storage"}},"extensions":{"location":"inline","resolution":{"status":"open"}}}],"start":0,"limit":25,"size":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "comment", "list", "12345", "--location", "inline")
	if gotLocation != "inline" {
		t.Fatalf("location=%q", gotLocation)
	}
	var data struct {
		Comments []struct {
			ID         string `json:"id"`
			Body       string `json:"body"`
			Resolution string `json:"resolution"`
		} `json:"comments"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Comments) != 1 || data.Comments[0].Resolution != "open" {
		t.Fatalf("comments=%+v", data.Comments)
	}
	if !strings.Contains(data.Comments[0].Body, "ok") {
		t.Fatalf("body not converted: %q", data.Comments[0].Body)
	}
}

func TestPageCommentAdd_MarkdownToStorage(t *testing.T) {
	var gotBody string
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/content" && r.Method == http.MethodPost {
			gotBody = readAll(r)
			_, _ = w.Write([]byte(`{"id":"9","type":"comment","title":"","body":{"storage":{"value":"<p>LGTM</p>","representation":"storage"}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	token := dryRunConfirmToken(t, "page", "comment", "add", "12345", "--body", "**LGTM**")
	stdout, _ := runRootOK(t, "--confirm", token, "page", "comment", "add", "12345", "--body", "**LGTM**")
	if !strings.Contains(gotBody, "storage") {
		t.Fatalf("body not storage: %s", gotBody)
	}
	var data struct {
		ID string `json:"id"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.ID != "9" {
		t.Fatalf("data=%+v", data)
	}
}

func TestPageCommentDelete_DangerousGate(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "--dry-run", "page", "comment", "delete", "9")
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatal("want E_CONFIRMATION_REQUIRED")
	}
}

// ─── page attachment ─────────────────────────────────────────────────────────

func TestPageAttachmentUpload_ConflictWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "diagram.png")
	if err := os.WriteFile(fp, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/child/attachment") && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"results":[{"id":"55","title":"diagram.png"}],"start":0,"limit":25,"size":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	token := dryRunConfirmToken(t, "page", "attachment", "upload", "12345", "--file", fp)
	stdout, _ := runRootExpectSilent(t, ExitConflict, "--confirm", token, "page", "attachment", "upload", "12345", "--file", fp)
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFLICT" {
		t.Fatal("want E_CONFLICT")
	}
}

func TestPageAttachmentUpload_Overwrite(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "diagram.png")
	if err := os.WriteFile(fp, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated := false
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/child/attachment") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"results":[{"id":"55","title":"diagram.png"}],"start":0,"limit":25,"size":1,"_links":{}}`))
		case strings.Contains(r.URL.Path, "/child/attachment/55/data"):
			updated = true
			_, _ = w.Write([]byte(`{"id":"55","type":"attachment","title":"diagram.png","version":{"number":2}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	token := dryRunConfirmToken(t, "page", "attachment", "upload", "12345", "--file", fp, "--overwrite")
	stdout, _ := runRootOK(t, "--confirm", token, "page", "attachment", "upload", "12345", "--file", fp, "--overwrite")
	if !updated {
		t.Fatal("expected UpdateAttachmentData call")
	}
	var data struct {
		Count int `json:"count"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Count != 1 {
		t.Fatalf("data=%+v", data)
	}
}

func TestPageAttachmentDownload_WritesFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "got.png")
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/download/att/55":
			_, _ = w.Write([]byte("BINARYDATA"))
		case strings.HasPrefix(r.URL.Path, "/rest/api/content/"):
			_, _ = w.Write([]byte(`{"id":"55","type":"attachment","title":"diagram.png","_links":{"download":"/download/att/55"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	stdout, _ := runRootOK(t, "page", "attachment", "download", "55", "--out", out)
	var data struct {
		Path string `json:"path"`
		Size int64  `json:"size_bytes"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Size != int64(len("BINARYDATA")) {
		t.Fatalf("data=%+v", data)
	}
	b, _ := os.ReadFile(out)
	if string(b) != "BINARYDATA" {
		t.Fatalf("file=%q", string(b))
	}
}

// ─── page label ──────────────────────────────────────────────────────────────

func TestPageLabelAdd_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/label") && r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"results":[{"name":"adr","prefix":"global"},{"name":"design","prefix":"global"}],"start":0,"limit":25,"size":2,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	token := dryRunConfirmToken(t, "page", "label", "add", "12345", "--labels", "adr,design")
	stdout, _ := runRootOK(t, "--confirm", token, "page", "label", "add", "12345", "--labels", "adr,design")
	var data struct {
		Count int `json:"count"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Count != 2 {
		t.Fatalf("data=%+v", data)
	}
}

func TestPageLabelRemove_PartialFailure(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/label") && r.Method == http.MethodDelete {
			if r.URL.Query().Get("name") == "bad" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"statusCode":404,"message":"no label"}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	token := dryRunConfirmToken(t, "page", "label", "remove", "12345", "--labels", "good,bad")
	stdout, _ := runRootExpectSilent(t, ExitGeneric, "--confirm", token, "page", "label", "remove", "12345", "--labels", "good,bad")
	// Non-JSON path? No — this uses default JSON envelope with ok=false at exit 1.
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			OK    bool `json:"ok"`
			Items []struct {
				Name string `json:"name"`
				OK   bool   `json:"ok"`
			} `json:"items"`
		} `json:"data"`
	}
	decodeRaw(t, stdout, &env)
	if len(env.Data.Items) != 2 {
		t.Fatalf("items=%+v", env.Data.Items)
	}
	if env.Data.OK {
		t.Fatal("expected data.ok=false on partial failure")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// ─── FCC coverage: read/list leaves + attachment delete ──────────────────────

func TestPageDescendants_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/descendant/page") {
			_, _ = w.Write([]byte(`{"results":[{"id":"7","type":"page","title":"Deep","_links":{"webui":"/d"}}],"start":0,"limit":25,"size":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "descendants", "1")
	var data struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Pages) != 1 || data.Pages[0].ID != "7" {
		t.Fatalf("pages=%+v", data.Pages)
	}
}

func TestPageAttachmentList_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/child/attachment") && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"results":[{"id":"55","title":"diagram.png","extensions":{"fileSize":1024,"mediaType":"image/png"}}],"start":0,"limit":25,"size":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "attachment", "list", "12345")
	var data struct {
		Attachments []struct {
			ID string `json:"id"`
		} `json:"attachments"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Attachments) != 1 || data.Attachments[0].ID != "55" {
		t.Fatalf("attachments=%+v", data.Attachments)
	}
}

func TestPageAttachmentDelete_DangerousGate(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	// Missing --dangerous in dry-run must be refused before any confirm token.
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "--dry-run", "page", "attachment", "delete", "55")
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatal("want E_CONFIRMATION_REQUIRED")
	}
	// Full two-step with --dangerous on both steps succeeds.
	token := dryRunConfirmToken(t, "--dangerous", "page", "attachment", "delete", "55")
	runRootOK(t, "--dangerous", "--confirm", token, "page", "attachment", "delete", "55")
}

func TestPageCommentGet_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"9","type":"comment","title":"Re: Draft","body":{"storage":{"value":"<p>looks good</p>","representation":"storage"}},"extensions":{"location":"footer"}}`))
	})
	stdout, _ := runRootOK(t, "page", "comment", "get", "9")
	var data struct {
		ID   string `json:"id"`
		Body string `json:"body"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.ID != "9" || !strings.Contains(data.Body, "looks good") {
		t.Fatalf("comment=%+v", data)
	}
}

func TestPageLabelList_HappyPath(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/label") && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"results":[{"prefix":"global","name":"runbook","id":"100"}],"start":0,"limit":200,"size":1,"_links":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	stdout, _ := runRootOK(t, "page", "label", "list", "12345")
	var data struct {
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Labels) != 1 || data.Labels[0].Name != "runbook" {
		t.Fatalf("labels=%+v", data.Labels)
	}
}

// TestPageDelete_DescendantEndpointUnsupported verifies the delete preview
// degrades gracefully when the recursive /descendant/page endpoint is not
// implemented (HTTP 501 on some Confluence DC versions): it falls back to the
// direct-children count and the delete still proceeds, rather than the whole
// command hard-failing on an informational count.
func TestPageDelete_DescendantEndpointUnsupported(t *testing.T) {
	pageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/descendant/page"):
			w.WriteHeader(http.StatusNotImplemented) // 501, like the real DC
		case strings.HasSuffix(r.URL.Path, "/child/page"):
			_, _ = w.Write([]byte(`{"results":[{"id":"x"}],"start":0,"limit":25,"size":1,"_links":{}}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/rest/api/content/"):
			_, _ = w.Write([]byte(`{"id":"12345","type":"page","title":"Doomed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	// Preview must succeed (fallback to direct children), not 501.
	stdout, _, err := runRoot(t, "--dangerous", "--dry-run", "page", "delete", "12345")
	if err != nil {
		t.Fatalf("dry-run should not hard-fail on 501: %v", err)
	}
	if !strings.Contains(stdout, "direct_children_only") {
		t.Fatalf("preview should report fallback scope:\n%s", stdout)
	}
	// And the delete itself proceeds.
	token := dryRunConfirmToken(t, "--dangerous", "page", "delete", "12345")
	runRootOK(t, "--dangerous", "--confirm", token, "page", "delete", "12345")
}
