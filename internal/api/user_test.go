package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCurrentUser_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/user/current" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"type":"known","username":"me","userKey":"abc123","displayName":"Me Myself","_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	u, err := c.Users.CurrentUser()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Username != "me" || u.DisplayName != "Me Myself" || u.UserKey != "abc123" {
		t.Errorf("user = %+v", u)
	}
}

func TestCurrentUser_AuthError(t *testing.T) {
	ts := statusServer(401, ``)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Users.CurrentUser()
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_AUTH" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestGetUser_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/user" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("username"); got != "jdoe" {
			t.Errorf("username = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"username":"jdoe","displayName":"John Doe","_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	u, err := c.Users.GetUser("jdoe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Username != "jdoe" {
		t.Errorf("user = %+v", u)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	ts := statusServer(404, `{"statusCode":404,"message":"No user found with key : ghost"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Users.GetUser("ghost")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_NOT_FOUND" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestGetUser_InvalidJSON(t *testing.T) {
	ts := statusServer(200, `{invalid`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Users.GetUser("jdoe")
	if err == nil || !strings.Contains(err.Error(), "parsing user") {
		t.Errorf("err = %v", err)
	}
}
