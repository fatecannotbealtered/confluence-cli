package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListComments_Inline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123/child/comment" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("location") != "inline" {
			t.Errorf("location = %q", q.Get("location"))
		}
		if q.Get("expand") != "extensions.resolution,body.storage" {
			t.Errorf("expand = %q", q.Get("expand"))
		}
		_, _ = fmt.Fprint(w, `{
			"results":[{
				"id":"c1","type":"comment","title":"Re: Page",
				"body":{"storage":{"value":"<p>fix this</p>","representation":"storage"}},
				"extensions":{"location":"inline","resolution":{"status":"open"}},
				"_links":{}
			}],
			"start":0,"limit":25,"size":1,"_links":{}
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Comments.ListComments("123", "inline", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cm := page.Items[0]
	if cm.ID != "c1" {
		t.Errorf("comment = %+v", cm)
	}
	if cm.Extensions == nil || cm.Extensions.Location != "inline" || cm.Extensions.Resolution.Status != "open" {
		t.Errorf("extensions = %+v", cm.Extensions)
	}
	if cm.Body.Storage.Value != "<p>fix this</p>" {
		t.Errorf("body = %+v", cm.Body)
	}
}

func TestListComments_AllLocations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("location") {
			t.Errorf("location should be absent, got %q", r.URL.Query().Get("location"))
		}
		_, _ = fmt.Fprint(w, `{"results":[],"start":0,"limit":25,"size":0,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.Comments.ListComments("123", "", 0, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateComment_TopLevel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/content" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var req createCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Type != "comment" || req.Container.ID != "123" {
			t.Errorf("req = %+v", req)
		}
		if len(req.Ancestors) != 0 {
			t.Errorf("ancestors = %+v, want empty for top-level", req.Ancestors)
		}
		if req.Body.Storage.Value != "<p>nice</p>" || req.Body.Storage.Representation != "storage" {
			t.Errorf("body = %+v", req.Body)
		}
		_, _ = fmt.Fprint(w, `{"id":"c9","type":"comment","_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	got, err := c.Comments.CreateComment("123", "<p>nice</p>", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "c9" {
		t.Errorf("comment = %+v", got)
	}
}

func TestCreateComment_Reply(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(req.Ancestors) != 1 || req.Ancestors[0].ID != "c1" {
			t.Errorf("ancestors = %+v", req.Ancestors)
		}
		_, _ = fmt.Fprint(w, `{"id":"c10","type":"comment","_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	got, err := c.Comments.CreateComment("123", "<p>reply</p>", "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "c10" {
		t.Errorf("comment = %+v", got)
	}
}

func TestCreateComment_Unauthorized(t *testing.T) {
	ts := statusServer(401, `{"statusCode":401,"message":"token expired"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Comments.CreateComment("123", "<p>x</p>", "")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_AUTH" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestDeleteComment(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/rest/api/content/c1" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if err := c.Comments.DeleteComment("c1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
