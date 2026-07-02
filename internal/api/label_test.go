package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListLabels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123/label" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"prefix":"global","name":"docs","id":"1","label":"docs"}],"start":0,"limit":25,"size":1,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Labels.ListLabels("123", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "docs" || page.Items[0].Prefix != "global" {
		t.Errorf("labels = %+v", page.Items)
	}
}

func TestAddLabels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/content/123/label" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var body []Label
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body) != 2 || body[0].Name != "a" || body[0].Prefix != "global" || body[1].Name != "b" {
			t.Errorf("body = %+v", body)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"prefix":"global","name":"a"},{"prefix":"global","name":"b"}],"start":0,"limit":25,"size":2,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	labels, err := c.Labels.AddLabels("123", []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("labels = %+v", labels)
	}
}

func TestRemoveLabel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/rest/api/content/123/label" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "old-label" {
			t.Errorf("name = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if err := c.Labels.RemoveLabel("123", "old-label"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddLabels_ServerError(t *testing.T) {
	ts := statusServer(500, `{"message":"boom"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Labels.AddLabels("123", []string{"x"})
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_SERVER" || !apiErr.Retryable {
		t.Errorf("Code = %q retryable = %v", apiErr.Code, apiErr.Retryable)
	}
}
