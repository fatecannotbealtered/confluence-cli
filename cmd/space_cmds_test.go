package cmd

import (
	"net/http"
	"testing"
	"time"
)

func spaceUserHandler(w http.ResponseWriter) {
	_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
}

// ─── space list ─────────────────────────────────────────────────────────────

func TestSpaceList_HappyPath(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			spaceUserHandler(w)
		case "/rest/api/space":
			_, _ = w.Write([]byte(`{"results":[{"key":"ENG","name":"Engineering","type":"global"}],"start":0,"limit":25,"size":1,"_links":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	stdout, _ := runRootOK(t, "space", "list", "--type", "global")
	var data struct {
		Spaces []struct {
			Key       string   `json:"key"`
			Untrusted []string `json:"_untrusted"`
		} `json:"spaces"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Spaces) != 1 || data.Spaces[0].Key != "ENG" {
		t.Fatalf("spaces=%+v", data.Spaces)
	}
	if len(data.Spaces[0].Untrusted) == 0 {
		t.Fatalf("expected name untrusted marker")
	}
}

func TestSpaceList_InvalidType(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) { spaceUserHandler(w) })
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "space", "list", "--type", "bogus")
	if decodeEnvelopeError(t, stdout)["code"] != "E_VALIDATION" {
		t.Fatal("want E_VALIDATION")
	}
}

// ─── space get ──────────────────────────────────────────────────────────────

func TestSpaceGet_HappyPath(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			spaceUserHandler(w)
		case "/rest/api/space/ENG":
			_, _ = w.Write([]byte(`{"key":"ENG","name":"Engineering","type":"global","description":{"plain":{"value":"team space","representation":"plain"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	stdout, _ := runRootOK(t, "space", "get", "ENG")
	var data struct {
		Key         string `json:"key"`
		Description string `json:"description"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Key != "ENG" || data.Description != "team space" {
		t.Fatalf("data=%+v", data)
	}
}

func TestSpaceGet_NotFound(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			spaceUserHandler(w)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"message":"No space"}`))
	})
	stdout, _ := runRootExpectSilent(t, ExitNotFound, "space", "get", "NOPE")
	if decodeEnvelopeError(t, stdout)["code"] != "E_NOT_FOUND" {
		t.Fatal("want E_NOT_FOUND")
	}
}

// ─── space create (write gate) ──────────────────────────────────────────────

func TestSpaceCreate_TwoStepGate(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			spaceUserHandler(w)
		case "/rest/api/space":
			_, _ = w.Write([]byte(`{"key":"ENG","name":"Engineering","type":"global"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	token := dryRunConfirmToken(t, "space", "create", "--key", "ENG", "--name", "Engineering")
	stdout, _ := runRootOK(t, "--confirm", token, "space", "create", "--key", "ENG", "--name", "Engineering")
	var data struct {
		Key string `json:"key"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Key != "ENG" {
		t.Fatalf("data=%+v", data)
	}
}

func TestSpaceCreate_RequiresConfirm(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) { spaceUserHandler(w) })
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "space", "create", "--key", "ENG", "--name", "Engineering")
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatal("want E_CONFIRMATION_REQUIRED")
	}
}

func TestSpaceCreate_MissingArgs(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) { spaceUserHandler(w) })
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "space", "create", "--key", "ENG")
	if decodeEnvelopeError(t, stdout)["code"] != "E_VALIDATION" {
		t.Fatal("want E_VALIDATION")
	}
}

// ─── space update ───────────────────────────────────────────────────────────

func TestSpaceUpdate_TwoStepGate(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			spaceUserHandler(w)
		case "/rest/api/space/ENG":
			_, _ = w.Write([]byte(`{"key":"ENG","name":"Engineering Team","type":"global"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	token := dryRunConfirmToken(t, "space", "update", "ENG", "--name", "Engineering Team")
	stdout, _ := runRootOK(t, "--confirm", token, "space", "update", "ENG", "--name", "Engineering Team")
	var data struct {
		Name string `json:"name"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Name != "Engineering Team" {
		t.Fatalf("data=%+v", data)
	}
}

// ─── space delete (dangerous double gate + wait) ────────────────────────────

func TestSpaceDelete_RequiresDangerousInDryRun(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) { spaceUserHandler(w) })
	// --dry-run without --dangerous must be rejected by the second gate.
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "--dry-run", "space", "delete", "ENG")
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatal("want E_CONFIRMATION_REQUIRED")
	}
}

func TestSpaceDelete_DoubleGateHappyPath(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			spaceUserHandler(w)
		case "/rest/api/space/ENG":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"task-9","links":{"status":"/rest/api/longtask/task-9"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	// Both steps must carry --dangerous.
	token := dryRunConfirmToken(t, "--dangerous", "space", "delete", "ENG")
	stdout, _ := runRootOK(t, "--dangerous", "--confirm", token, "space", "delete", "ENG")
	var data struct {
		TaskID     string `json:"task_id"`
		StatusLink string `json:"status_link"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.TaskID != "task-9" || data.StatusLink == "" {
		t.Fatalf("data=%+v", data)
	}
}

func TestSpaceDelete_ConfirmStepMissingDangerous(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) { spaceUserHandler(w) })
	token := dryRunConfirmToken(t, "--dangerous", "space", "delete", "ENG")
	// Confirm step drops --dangerous -> second gate blocks.
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "--confirm", token, "space", "delete", "ENG")
	if decodeEnvelopeError(t, stdout)["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatal("want E_CONFIRMATION_REQUIRED")
	}
}

func TestSpaceDelete_WaitTimeoutPreservesTaskID(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			spaceUserHandler(w)
		case "/rest/api/space/ENG":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"task-9","links":{"status":"/rest/api/longtask/task-9"}}`))
		case "/rest/api/longtask/task-9":
			_, _ = w.Write([]byte(`{"id":"task-9","percentageComplete":50,"finished":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	// Force the deadline to be already past and neuter sleep.
	clockNow = func() time.Time { return time.Unix(0, 0) }
	clockSleep = func(time.Duration) {}
	token := dryRunConfirmToken(t, "--dangerous", "space", "delete", "ENG")
	stdout, _ := runRootExpectSilent(t, ExitTimeout, "--dangerous", "--confirm", token, "space", "delete", "ENG", "--wait", "--timeout", "0")
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_TIMEOUT" {
		t.Fatalf("code=%v", errPayload["code"])
	}
	details, _ := errPayload["details"].(map[string]any)
	if details["task_id"] != "task-9" {
		t.Fatalf("task_id not preserved: %v", details)
	}
}

func TestSpaceDelete_WaitFinishes(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			spaceUserHandler(w)
		case "/rest/api/space/ENG":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"task-9","links":{"status":"/x"}}`))
		case "/rest/api/longtask/task-9":
			_, _ = w.Write([]byte(`{"id":"task-9","percentageComplete":100,"finished":true,"successful":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	clockSleep = func(time.Duration) {}
	token := dryRunConfirmToken(t, "--dangerous", "space", "delete", "ENG")
	stdout, _ := runRootOK(t, "--dangerous", "--confirm", token, "space", "delete", "ENG", "--wait", "--timeout", "5")
	var data struct {
		Status     string `json:"status"`
		Successful bool   `json:"successful"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.Status != "finished" || !data.Successful {
		t.Fatalf("data=%+v", data)
	}
}
