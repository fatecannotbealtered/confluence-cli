package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemInfo_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/settings/systemInfo" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"baseUrl":"https://confluence.example.com","version":"8.5.4","buildNumber":"9012"}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	si, err := c.System.SystemInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if si.Version != "8.5.4" || si.BaseURL != "https://confluence.example.com" || si.BuildNumber != "9012" {
		t.Errorf("systemInfo = %+v", si)
	}
}

func TestSystemInfo_Forbidden(t *testing.T) {
	ts := statusServer(403, `{"statusCode":403,"message":"admin only"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.System.SystemInfo()
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_FORBIDDEN" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestSystemInfo_InvalidJSON(t *testing.T) {
	ts := statusServer(200, `{invalid`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	if _, err := c.System.SystemInfo(); err == nil {
		t.Fatal("expected parse error")
	}
}
