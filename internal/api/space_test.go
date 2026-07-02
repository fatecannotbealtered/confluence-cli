package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListSpaces_WithTypeFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/space" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "global" {
			t.Errorf("type = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"key":"DEV","name":"Dev","type":"global","_links":{}}],"start":0,"limit":25,"size":1,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Spaces.ListSpaces("global", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Key != "DEV" {
		t.Errorf("items = %+v", page.Items)
	}
}

func TestListSpaces_NoTypeFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("type") {
			t.Errorf("type param should be absent, got %q", r.URL.Query().Get("type"))
		}
		_, _ = fmt.Fprint(w, `{"results":[],"start":0,"limit":25,"size":0,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.Spaces.ListSpaces("", 0, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSpace_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/space/DEV" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("expand"); got != "description.plain" {
			t.Errorf("expand = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"id":42,"key":"DEV","name":"Dev Space","type":"global","description":{"plain":{"value":"the dev space","representation":"plain"}},"_links":{"webui":"/display/DEV"}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	s, err := c.Spaces.GetSpace("DEV")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Key != "DEV" || s.Name != "Dev Space" || s.ID != 42 {
		t.Errorf("space = %+v", s)
	}
	if s.Description == nil || s.Description.Plain.Value != "the dev space" {
		t.Errorf("description = %+v", s.Description)
	}
}

func TestGetSpace_Forbidden(t *testing.T) {
	ts := statusServer(403, `{"statusCode":403,"message":"no permission"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Spaces.GetSpace("SECRET")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_FORBIDDEN" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestCreateSpace_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/space" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var req CreateSpaceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Key != "NEW" || req.Name != "New Space" {
			t.Errorf("req = %+v", req)
		}
		_, _ = fmt.Fprint(w, `{"id":7,"key":"NEW","name":"New Space","_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	s, err := c.Spaces.CreateSpace(&CreateSpaceRequest{Key: "NEW", Name: "New Space"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Key != "NEW" {
		t.Errorf("space = %+v", s)
	}
}

func TestUpdateSpace_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/rest/api/space/DEV" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"key":"DEV","name":"Renamed","_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	s, err := c.Spaces.UpdateSpace("DEV", &UpdateSpaceRequest{Name: "Renamed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "Renamed" {
		t.Errorf("space = %+v", s)
	}
}

func TestDeleteSpace_ReturnsLongTask(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/rest/api/space/OLD" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprint(w, `{"id":"task-123","links":{"status":"/rest/api/longtask/task-123"}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	lt, err := c.Spaces.DeleteSpace("OLD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lt.ID != "task-123" {
		t.Errorf("ID = %q", lt.ID)
	}
	if lt.Links.Status != "/rest/api/longtask/task-123" {
		t.Errorf("status link = %q", lt.Links.Status)
	}
}

func TestDeleteSpace_InvalidJSON(t *testing.T) {
	ts := statusServer(202, `{invalid`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.Spaces.DeleteSpace("OLD"); err == nil {
		t.Fatal("expected parse error")
	}
}
