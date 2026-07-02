package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPage_SinglePage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("start"); got != "0" {
			t.Errorf("start = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "25" {
			t.Errorf("limit = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"id":"1","title":"A"},{"id":"2","title":"B"}],"start":0,"limit":25,"size":2,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := getPage[Content](c, "/rest/api/content/1/child/page", nil, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 2 {
		t.Errorf("items = %d", len(page.Items))
	}
	if page.HasMore {
		t.Error("HasMore should be false without _links.next")
	}
	if page.NextStartAt != 0 {
		t.Errorf("NextStartAt = %d", page.NextStartAt)
	}
}

func TestGetPage_HasMore(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"results":[{"id":"1"},{"id":"2"}],"start":4,"limit":2,"size":2,"_links":{"next":"/rest/api/content/1/child/page?start=6&limit=2"}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := getPage[Content](c, "/rest/api/content/1/child/page", nil, 4, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !page.HasMore {
		t.Error("HasMore should be true with _links.next")
	}
	if page.NextStartAt != 6 {
		t.Errorf("NextStartAt = %d, want 6", page.NextStartAt)
	}
}

func TestGetPage_InvalidJSON(t *testing.T) {
	ts := statusServer(200, `{invalid`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := getPage[Content](c, "/rest/api/content/1/child/page", nil, 0, 10)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestGetAllPages_WalksAllPages(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		start := r.URL.Query().Get("start")
		switch start {
		case "0":
			_, _ = fmt.Fprint(w, `{"results":[{"id":"1"}],"start":0,"limit":1,"size":1,"_links":{"next":"/next"}}`)
		case "1":
			_, _ = fmt.Fprint(w, `{"results":[{"id":"2"}],"start":1,"limit":1,"size":1,"_links":{}}`)
		default:
			t.Errorf("unexpected start = %q", start)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	items, err := getAllPages[Content](c, "/rest/api/space", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("items = %d, want 2", len(items))
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if items[0].ID != "1" || items[1].ID != "2" {
		t.Errorf("items order = %s, %s", items[0].ID, items[1].ID)
	}
}

func TestGetAllPages_PropagatesError(t *testing.T) {
	ts := statusServer(403, `{"message":"no"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := getAllPages[Content](c, "/rest/api/space", nil)
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_FORBIDDEN" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestBuildPagePath_PreservesParams(t *testing.T) {
	params := map[string][]string{"expand": {"body.storage"}}
	got := buildPagePath("/rest/api/content", params, 10, 5)
	want := "/rest/api/content?expand=body.storage&limit=5&start=10"
	if got != want {
		t.Errorf("buildPagePath = %q, want %q", got, want)
	}
}
