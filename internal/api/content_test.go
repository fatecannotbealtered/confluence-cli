package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetContent_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("expand"); got != "body.storage,version" {
			t.Errorf("expand = %q", got)
		}
		_, _ = fmt.Fprint(w, `{
			"id":"123","type":"page","status":"current","title":"Home",
			"space":{"key":"DEV","name":"Dev Space"},
			"version":{"number":7,"when":"2026-01-01T00:00:00.000Z"},
			"body":{"storage":{"value":"<p>hi</p>","representation":"storage"}},
			"_links":{"webui":"/display/DEV/Home"}
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	got, err := c.Content.GetContent("123", GetContentOptions{Expand: []string{"body.storage", "version"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "123" || got.Title != "Home" || got.Type != "page" {
		t.Errorf("content = %+v", got)
	}
	if got.Version == nil || got.Version.Number != 7 {
		t.Errorf("version = %+v", got.Version)
	}
	if got.Body == nil || got.Body.Storage == nil || got.Body.Storage.Value != "<p>hi</p>" {
		t.Errorf("body = %+v", got.Body)
	}
	if got.Space == nil || got.Space.Key != "DEV" {
		t.Errorf("space = %+v", got.Space)
	}
	if got.Links.WebUI != "/display/DEV/Home" {
		t.Errorf("webui = %q", got.Links.WebUI)
	}
}

func TestGetContent_HistoricalVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("status") != "historical" {
			t.Errorf("status = %q", q.Get("status"))
		}
		if q.Get("version") != "3" {
			t.Errorf("version = %q", q.Get("version"))
		}
		_, _ = fmt.Fprint(w, `{"id":"123","status":"historical","version":{"number":3},"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	got, err := c.Content.GetContent("123", GetContentOptions{Status: "historical", Version: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "historical" || got.Version.Number != 3 {
		t.Errorf("content = %+v", got)
	}
}

func TestGetContent_NotFound(t *testing.T) {
	ts := statusServer(404, `{"statusCode":404,"message":"No content found with id : 999"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Content.GetContent("999", GetContentOptions{})
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_NOT_FOUND" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestGetContentBySpaceTitle_Found(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/search" {
			t.Errorf("path = %q", r.URL.Path)
		}
		cql := r.URL.Query().Get("cql")
		if !strings.Contains(cql, `space = "DEV"`) || !strings.Contains(cql, `title = "My Page"`) {
			t.Errorf("cql = %q", cql)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"id":"55","type":"page","title":"My Page","_links":{}}],"start":0,"limit":1,"size":1,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	got, err := c.Content.GetContentBySpaceTitle("DEV", "My Page", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "55" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestGetContentBySpaceTitle_NoMatch(t *testing.T) {
	ts := statusServer(200, `{"results":[],"start":0,"limit":1,"size":0,"_links":{}}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Content.GetContentBySpaceTitle("DEV", "Missing", nil)
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_NOT_FOUND" {
		t.Errorf("Code = %q, want synthesized E_NOT_FOUND", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "Missing") {
		t.Errorf("Message = %q", apiErr.Message)
	}
}

func TestCreateContent_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/content" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var req CreateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Type != "page" || req.Space.Key != "DEV" || req.Title != "New" {
			t.Errorf("req = %+v", req)
		}
		if req.Body.Storage.Representation != "storage" {
			t.Errorf("representation = %q", req.Body.Storage.Representation)
		}
		if len(req.Ancestors) != 1 || req.Ancestors[0].ID != "100" {
			t.Errorf("ancestors = %+v", req.Ancestors)
		}
		_, _ = fmt.Fprint(w, `{"id":"200","type":"page","title":"New","version":{"number":1},"_links":{"webui":"/display/DEV/New"}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	got, err := c.Content.CreateContent(&CreateContentRequest{
		Type:      "page",
		Title:     "New",
		Space:     SpaceKeyRef{Key: "DEV"},
		Body:      RequestBody{Storage: BodyRepresentation{Value: "<p>x</p>", Representation: "storage"}},
		Ancestors: []ContentRef{{ID: "100"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "200" || got.Version.Number != 1 {
		t.Errorf("content = %+v", got)
	}
}

func TestUpdateContent_VersionConflict(t *testing.T) {
	ts := statusServer(409, `{"statusCode":409,"message":"Version must be incremented on update"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Content.UpdateContent("123", &UpdateContentRequest{
		Type: "page", Title: "T", Version: VersionRef{Number: 2},
	})
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_CONFLICT" {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if apiErr.Retryable {
		t.Error("conflict should not be retryable")
	}
}

func TestUpdateContent_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/rest/api/content/123" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var req UpdateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Version.Number != 8 {
			t.Errorf("version = %d", req.Version.Number)
		}
		_, _ = fmt.Fprint(w, `{"id":"123","type":"page","title":"T","version":{"number":8},"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	got, err := c.Content.UpdateContent("123", &UpdateContentRequest{
		Type: "page", Title: "T", Version: VersionRef{Number: 8},
		Body: &RequestBody{Storage: BodyRepresentation{Value: "<p>v8</p>", Representation: "storage"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version.Number != 8 {
		t.Errorf("version = %d", got.Version.Number)
	}
}

func TestDeleteContent_TrashOnly(t *testing.T) {
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		paths = append(paths, r.URL.RequestURI())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if err := c.Content.DeleteContent("123", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/rest/api/content/123" {
		t.Errorf("paths = %v", paths)
	}
}

func TestDeleteContent_Purge(t *testing.T) {
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if err := c.Content.DeleteContent("123", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"/rest/api/content/123", "/rest/api/content/123?status=trashed"}
	if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestChildPages(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123/child/page" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"id":"1","title":"C1"},{"id":"2","title":"C2"}],"start":0,"limit":25,"size":2,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Content.ChildPages("123", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].Title != "C1" {
		t.Errorf("items = %+v", page.Items)
	}
}

func TestDescendantPages(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123/descendant/page" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"id":"9"}],"start":0,"limit":25,"size":1,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Content.DescendantPages("123", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "9" {
		t.Errorf("items = %+v", page.Items)
	}
}

func TestAncestors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("expand"); got != "ancestors" {
			t.Errorf("expand = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"id":"123","ancestors":[{"id":"1","title":"Root"},{"id":"2","title":"Parent"}],"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	ancestors, err := c.Content.Ancestors("123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ancestors) != 2 || ancestors[0].Title != "Root" || ancestors[1].Title != "Parent" {
		t.Errorf("ancestors = %+v", ancestors)
	}
}

func TestContentHistory(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123/history" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{
			"latest":true,
			"createdBy":{"username":"alice","displayName":"Alice","_links":{}},
			"createdDate":"2025-01-01T00:00:00.000Z",
			"lastUpdated":{"by":{"username":"bob","displayName":"Bob","_links":{}},"when":"2026-01-01T00:00:00.000Z","number":7}
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	h, err := c.Content.ContentHistory("123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.Latest || h.CreatedBy.Username != "alice" {
		t.Errorf("history = %+v", h)
	}
	if h.LastUpdated == nil || h.LastUpdated.Number != 7 || h.LastUpdated.By.Username != "bob" {
		t.Errorf("lastUpdated = %+v", h.LastUpdated)
	}
}

func TestContentHistory_InvalidJSON(t *testing.T) {
	ts := statusServer(200, `{invalid`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.Content.ContentHistory("123"); err == nil {
		t.Fatal("expected parse error")
	}
}
