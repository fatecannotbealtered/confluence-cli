package cmd

import (
	"net/http"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/confluence-cli/internal/config"
)

// ─── auth login ─────────────────────────────────────────────────────────────

func TestAuthLogin_NonInteractiveInvalidURL(t *testing.T) {
	setTempHome(t)
	_, stderr := runRootExpectSilent(t, ExitBadArgs, "--format", "text", "auth", "login", "--url", "http://confluence.example.com", "--token", "pat")
	if !containsAny(stderr, "url must start with https://") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestAuthLogin_NonInteractiveEmptyToken(t *testing.T) {
	setTempHome(t)
	_, stderr := runRootExpectSilent(t, ExitBadArgs, "--format", "text", "auth", "login", "--url", "https://confluence.example.com", "--token", "   ")
	if !containsAny(stderr, "token cannot be empty") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestAuthLogin_NonInteractiveInvalidCredentials(t *testing.T) {
	ts := mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"statusCode":401,"message":"bad token"}`))
	})
	clearConfluenceEnv(t)
	_, stderr := runRootExpectSilent(t, ExitAuth, "--format", "text", "auth", "login", "--url", ts.URL, "--token", "bad-token")
	if !containsAny(stderr, "invalid credentials") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestAuthLogin_ValidatesBeforeSaving(t *testing.T) {
	// A failed probe must leave no config on disk.
	ts := mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"statusCode":401,"message":"bad token"}`))
	})
	clearConfluenceEnv(t)
	runRootExpectSilent(t, ExitAuth, "--format", "text", "auth", "login", "--url", ts.URL, "--token", "bad-token")
	if config.IsConfigured() {
		t.Fatal("failed login must not persist credentials")
	}
}

func TestAuthLogin_NonInteractiveSuccessJSON(t *testing.T) {
	ts := mockConfluenceServer(t, currentUserHandler(t))
	clearConfluenceEnv(t)
	token := dryRunConfirmToken(t, "auth", "login", "--url", ts.URL, "--token", "good-token")
	stdout, _ := runRootOK(t, "--confirm", token, "auth", "login", "--url", ts.URL, "--token", "good-token")
	var result map[string]string
	decodeEnvelopeData(t, stdout, &result)
	if result["status"] != "ok" || result["username"] != "jdoe" || result["display_name"] != "John Doe" {
		t.Fatalf("result=%v", result)
	}
	cfg, err := config.Load()
	if err != nil || cfg.URL != ts.URL || cfg.Token != "good-token" {
		t.Fatalf("config=%+v err=%v", cfg, err)
	}
}

func TestAuthLogin_NonInteractiveSuccessText(t *testing.T) {
	ts := mockConfluenceServer(t, currentUserHandler(t))
	clearConfluenceEnv(t)
	stdout, _ := runRootOK(t, "--format", "text", "auth", "login", "--url", ts.URL, "--token", "good-token")
	if !containsAny(stdout, "Logged in as John Doe") {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestAuthLogin_JSONWithoutFlagsRejected(t *testing.T) {
	setTempHome(t)
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "auth", "login")
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_VALIDATION" {
		t.Fatalf("error=%v", errPayload)
	}
}

func TestAuthLogin_InteractiveSuccess(t *testing.T) {
	ts := mockConfluenceServer(t, currentUserHandler(t))
	clearConfluenceEnv(t)
	stdout, _, err := runRootWithStdin(t, ts.URL+"\ngood-token\n", "--format", "text", "auth", "login")
	if err != nil {
		t.Fatalf("unexpected error %v (exit=%d)", err, LastExitCode())
	}
	if !containsAny(stdout, "Logged in as John Doe") {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestAuthLogin_JSONRequiresConfirmToken(t *testing.T) {
	// Fleet semantics (jira-cli login): login IS a write command and routes
	// through the confirm gate in JSON mode — no exemption.
	ts := mockConfluenceServer(t, currentUserHandler(t))
	clearConfluenceEnv(t)
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "auth", "login", "--url", ts.URL, "--token", "good-token")
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatalf("error=%v", errPayload)
	}
}

func TestAuthLogin_DryRunRedactsToken(t *testing.T) {
	ts := mockConfluenceServer(t, currentUserHandler(t))
	clearConfluenceEnv(t)
	stdout, _, err := runRoot(t, "--dry-run", "auth", "login", "--url", ts.URL, "--token", "super-secret-pat")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if strings.Contains(stdout, "super-secret-pat") {
		t.Fatalf("dry-run preview leaked the plaintext token:\n%s", stdout)
	}
	if !strings.Contains(stdout, "token_sha256") {
		t.Fatalf("dry-run preview should carry the token fingerprint:\n%s", stdout)
	}
}

// ─── auth logout ────────────────────────────────────────────────────────────

func TestAuthLogout_JSON(t *testing.T) {
	ts := mockConfluenceServer(t, currentUserHandler(t))
	clearConfluenceEnv(t)
	runRootOK(t, "--format", "text", "auth", "login", "--url", ts.URL, "--token", "good-token")

	token := dryRunConfirmToken(t, "auth", "logout")
	stdout, _ := runRootOK(t, "--confirm", token, "auth", "logout")
	var result map[string]string
	decodeEnvelopeData(t, stdout, &result)
	if result["status"] != "loggedOut" {
		t.Fatalf("result=%v", result)
	}
	if config.IsConfigured() {
		t.Fatal("logout should remove credentials")
	}
}

func TestAuthLogout_RequiresConfirmToken(t *testing.T) {
	setTempHome(t)
	stdout, _ := runRootExpectSilent(t, ExitConfirmRequired, "auth", "logout")
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_CONFIRMATION_REQUIRED" {
		t.Fatalf("error=%v", errPayload)
	}
}

func TestAuthLogout_Text(t *testing.T) {
	setTempHome(t)
	stdout, _ := runRootOK(t, "--format", "text", "auth", "logout")
	if !containsAny(stdout, "Logged out") {
		t.Fatalf("stdout=%q", stdout)
	}
}

// ─── auth status ────────────────────────────────────────────────────────────

func TestAuthStatus_NotConfigured(t *testing.T) {
	setTempHome(t)
	clearConfluenceEnv(t)
	stdout, _ := runRootOK(t, "auth", "status")
	var doc authStatusDocument
	decodeEnvelopeData(t, stdout, &doc)
	if doc.Status != "not_configured" || doc.TokenPresent {
		t.Fatalf("doc=%+v", doc)
	}
}

func TestAuthStatus_ValidRedacted(t *testing.T) {
	mockConfluenceServer(t, currentUserHandler(t))
	stdout, _ := runRootOK(t, "auth", "status")
	var doc authStatusDocument
	decodeEnvelopeData(t, stdout, &doc)
	if doc.Status != "valid" || doc.Username != "jdoe" || !doc.TokenPresent {
		t.Fatalf("doc=%+v", doc)
	}
	if doc.TokenSHA256 == "" || len(doc.TokenSHA256) != 16 {
		t.Fatalf("token fingerprint should be a 16-char sha256 prefix, got %q", doc.TokenSHA256)
	}
	if strings.Contains(stdout, "test-pat-token") {
		t.Fatalf("auth status leaked the plaintext token:\n%s", stdout)
	}
}

func TestAuthStatus_InvalidToken(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"statusCode":401,"message":"expired"}`))
	})
	stdout, _ := runRootOK(t, "auth", "status")
	var doc authStatusDocument
	decodeEnvelopeData(t, stdout, &doc)
	if doc.Status != "invalid" || doc.Error == "" {
		t.Fatalf("doc=%+v", doc)
	}
}

func TestAuthStatus_Text(t *testing.T) {
	mockConfluenceServer(t, currentUserHandler(t))
	stdout, _ := runRootOK(t, "--format", "text", "auth", "status")
	if !containsAny(stdout, "Status: valid") || strings.Contains(stdout, "test-pat-token") {
		t.Fatalf("stdout=%q", stdout)
	}
}
