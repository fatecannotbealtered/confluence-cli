package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetLongTask_Running(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/longtask/task-123" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{
			"id":"task-123",
			"name":{"key":"com.atlassian.confluence.extra.flyingpdf.deletespace"},
			"elapsedTime":1500,
			"percentageComplete":40,
			"successful":false,
			"finished":false,
			"messages":[{"translation":"Deleting space..."}]
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	lt, err := c.LongTasks.GetLongTask("task-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lt.ID != "task-123" || lt.Finished || lt.PercentageComplete != 40 {
		t.Errorf("longtask = %+v", lt)
	}
	if len(lt.Messages) != 1 || lt.Messages[0].Translation != "Deleting space..." {
		t.Errorf("messages = %+v", lt.Messages)
	}
}

func TestGetLongTask_NotFound(t *testing.T) {
	ts := statusServer(404, `{"statusCode":404,"message":"no task"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.LongTasks.GetLongTask("gone")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_NOT_FOUND" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestGetLongTask_InvalidJSON(t *testing.T) {
	ts := statusServer(200, `{invalid`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.LongTasks.GetLongTask("task-123"); err == nil {
		t.Fatal("expected parse error")
	}
}
