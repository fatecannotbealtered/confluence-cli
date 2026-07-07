package cmd

import (
	"net/http"
	"testing"
)

// ─── user current / get ─────────────────────────────────────────────────────

func TestUserCurrent_HappyPath(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe","userKey":"key-1","type":"known"}`))
	})
	stdout, _ := runRootOK(t, "user", "current")
	var data struct {
		Username    string   `json:"username"`
		DisplayName string   `json:"display_name"`
		Untrusted   []string `json:"_untrusted"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Username != "jdoe" || data.DisplayName != "John Doe" {
		t.Fatalf("data=%+v", data)
	}
	if len(data.Untrusted) != 1 || data.Untrusted[0] != "display_name" {
		t.Fatalf("untrusted=%v", data.Untrusted)
	}
}

func TestUserGet_HappyPath(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
		case "/rest/api/user":
			if r.URL.Query().Get("username") != "asmith" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"username":"asmith","displayName":"Alice Smith"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	stdout, _ := runRootOK(t, "user", "get", "asmith")
	var data struct {
		Username string `json:"username"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Username != "asmith" {
		t.Fatalf("data=%+v", data)
	}
}

func TestUserGet_NotFound(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"message":"no user"}`))
	})
	stdout, _ := runRootExpectSilent(t, ExitNotFound, "user", "get", "ghost")
	if decodeEnvelopeError(t, stdout)["code"] != "E_NOT_FOUND" {
		t.Fatal("want E_NOT_FOUND")
	}
}

// ─── user search ────────────────────────────────────────────────────────────

func TestUserSearch_HappyPath(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
		case "/rest/api/search":
			_, _ = w.Write([]byte(`{"results":[{"user":{"username":"jdoe","displayName":"John Doe"}}],"start":0,"limit":25,"size":1,"totalSize":1,"_links":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	stdout, _ := runRootOK(t, "user", "search", "John")
	var data struct {
		Users []struct {
			Username string `json:"username"`
		} `json:"items"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Users) != 1 || data.Users[0].Username != "jdoe" {
		t.Fatalf("users=%+v", data.Users)
	}
}

// ─── task get ───────────────────────────────────────────────────────────────

func TestTaskGet_HappyPath(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
		case "/rest/api/longtask/task-9":
			_, _ = w.Write([]byte(`{"id":"task-9","name":{"key":"delete-space"},"percentageComplete":100,"successful":true,"finished":true,"messages":[{"translation":"done"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	stdout, _ := runRootOK(t, "task", "get", "task-9")
	var data struct {
		ID                 string   `json:"id"`
		Name               string   `json:"name"`
		PercentageComplete int      `json:"percentage_complete"`
		Successful         bool     `json:"successful"`
		Finished           bool     `json:"finished"`
		Messages           []string `json:"messages"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.ID != "task-9" || data.Name != "delete-space" || !data.Finished || !data.Successful {
		t.Fatalf("data=%+v", data)
	}
	if len(data.Messages) != 1 || data.Messages[0] != "done" {
		t.Fatalf("messages=%v", data.Messages)
	}
}

func TestTaskGet_NotFound(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"message":"no task"}`))
	})
	stdout, _ := runRootExpectSilent(t, ExitNotFound, "task", "get", "ghost")
	if decodeEnvelopeError(t, stdout)["code"] != "E_NOT_FOUND" {
		t.Fatal("want E_NOT_FOUND")
	}
}
