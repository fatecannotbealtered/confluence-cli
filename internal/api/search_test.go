package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearch_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/search" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("cql") != `text ~ "roadmap"` {
			t.Errorf("cql = %q", q.Get("cql"))
		}
		if q.Get("excerpt") != "highlight" {
			t.Errorf("excerpt = %q", q.Get("excerpt"))
		}
		if q.Get("start") != "0" || q.Get("limit") != "10" {
			t.Errorf("start/limit = %s/%s", q.Get("start"), q.Get("limit"))
		}
		_, _ = fmt.Fprint(w, `{
			"results":[
				{
					"content":{"id":"11","type":"page","title":"Roadmap 2026","_links":{"webui":"/display/DEV/Roadmap+2026"}},
					"title":"Roadmap 2026",
					"excerpt":"the @@@hl@@@roadmap@@@endhl@@@ for 2026",
					"url":"/display/DEV/Roadmap+2026",
					"entityType":"content",
					"lastModified":"2026-06-01T10:00:00.000Z"
				}
			],
			"start":0,"limit":10,"size":1,"totalSize":1,
			"cqlQuery":"text ~ \"roadmap\"",
			"_links":{}
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Search.Search(`text ~ "roadmap"`, SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Results) != 1 {
		t.Fatalf("results = %d", len(page.Results))
	}
	r := page.Results[0]
	if !strings.Contains(r.Excerpt, "@@@hl@@@") {
		t.Errorf("excerpt = %q", r.Excerpt)
	}
	if r.WebURL != ts.URL+"/display/DEV/Roadmap+2026" {
		t.Errorf("WebURL = %q, want full clickable URL", r.WebURL)
	}
	if page.HasMore {
		t.Error("HasMore should be false")
	}
	if page.TotalSize != 1 {
		t.Errorf("TotalSize = %d", page.TotalSize)
	}
}

func TestSearch_Pagination(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("start"); got != "5" {
			t.Errorf("start = %q", got)
		}
		_, _ = fmt.Fprint(w, `{
			"results":[{"title":"A","url":"/a"},{"title":"B","url":"/b"}],
			"start":5,"limit":2,"size":2,"totalSize":20,
			"_links":{"next":"/rest/api/search?start=7&limit=2"}
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Search.Search("type = page", SearchOptions{Start: 5, Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !page.HasMore {
		t.Error("HasMore should be true")
	}
	if page.NextStart != 7 {
		t.Errorf("NextStart = %d, want 7", page.NextStart)
	}
}

func TestSearch_WebURLFallsBackToResultURL(t *testing.T) {
	// Space results carry no content block; WebURL uses the result url path.
	ts := statusServer(200, `{
		"results":[{"title":"Dev Space","url":"/display/DEV","entityType":"space"}],
		"start":0,"limit":25,"size":1,"_links":{}
	}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Search.Search("type = space", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Results[0].WebURL != ts.URL+"/display/DEV" {
		t.Errorf("WebURL = %q", page.Results[0].WebURL)
	}
}

func TestSearch_CQLError(t *testing.T) {
	ts := statusServer(400, `{"statusCode":400,"message":"Could not parse cql : bogus ~"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Search.Search("bogus ~", SearchOptions{})
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_USAGE" {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if apiErr.Details["server_message"] != "Could not parse cql : bogus ~" {
		t.Errorf("Details = %v", apiErr.Details)
	}
}

func TestSearch_InvalidJSON(t *testing.T) {
	ts := statusServer(200, `{invalid`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.Search.Search("type = page", SearchOptions{}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestSearchUser(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cql := r.URL.Query().Get("cql")
		if cql != `user.fullname ~ "john"` {
			t.Errorf("cql = %q", cql)
		}
		_, _ = fmt.Fprint(w, `{
			"results":[{"user":{"username":"jdoe","displayName":"John Doe","_links":{}},"title":"John Doe","entityType":"user","url":""}],
			"start":0,"limit":25,"size":1,"_links":{}
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Search.SearchUser("john", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Results) != 1 || page.Results[0].User == nil || page.Results[0].User.Username != "jdoe" {
		t.Errorf("results = %+v", page.Results)
	}
}
