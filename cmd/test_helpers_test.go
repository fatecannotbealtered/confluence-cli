package cmd

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/pflag"
	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	// Allow httptest.NewTLSServer URLs (auth login requires https:// URLs).
	http.DefaultTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test-only
	}
	// Tests must never touch the real OS keyring.
	keyring.MockInit()
	// No 429/5xx retry sleeps, no audit writes into the real home directory.
	_ = os.Setenv("CONFLUENCE_CLI_RETRY_BASE_MS", "0")
	_ = os.Setenv("CONFLUENCE_CLI_NO_AUDIT", "1")
	os.Exit(m.Run())
}

// mockConfluenceServer starts an httptest TLS server and points the CLI at it
// via CONFLUENCE_CLI_URL/CONFLUENCE_CLI_TOKEN, with HOME redirected to a temp
// dir so no real config is read or written.
func mockConfluenceServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	t.Setenv("CONFLUENCE_CLI_URL", ts.URL)
	t.Setenv("CONFLUENCE_CLI_TOKEN", "test-pat-token")
	setTempHome(t)
	return ts
}

func setTempHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	return tmpDir
}

func clearConfluenceEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CONFLUENCE_CLI_URL", "")
	t.Setenv("CONFLUENCE_CLI_TOKEN", "")
}

// currentUserHandler responds to GET /rest/api/user/current; other paths 404.
func currentUserHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/user/current":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"type":"known","username":"jdoe","displayName":"John Doe"}`))
		case "/rest/api/settings/systemInfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"8.5.4","buildNumber":"9012"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"statusCode":404,"message":"not found"}`))
		}
	}
}

// resetCmdState restores global flag state that cobra keeps between runs.
func resetCmdState(t *testing.T) {
	t.Helper()
	outputFormat = outputFormatJSON
	jsonMode = true
	compactMode = false
	fieldsList = nil
	quietMode = false
	dryRun = false
	confirmToken = ""
	dangerousMode = false
	forceMode = false
	loginURLFlag = ""
	loginTokenFlag = ""
	spaceListType = ""
	spaceListLimit = 0
	spaceListStart = 0
	spaceCreateKey = ""
	spaceCreateName = ""
	spaceCreateDesc = ""
	spaceUpdateName = ""
	spaceUpdateDesc = ""
	spaceDeleteWait = false
	spaceDeleteTO = 60
	searchSpaces = nil
	searchType = ""
	searchTitle = ""
	searchText = ""
	searchLabels = nil
	searchCreator = ""
	searchContributor = ""
	searchAncestor = ""
	searchCreatedSince = ""
	searchCreatedUntil = ""
	searchModifiedSince = ""
	searchModifiedUntil = ""
	searchSort = ""
	searchDesc = false
	searchAsc = false
	searchCountOnly = false
	searchAll = false
	searchLimit = 0
	searchStart = 0
	userSearchLimit = 0
	userSearchStart = 0
	pageGetBodyFormat = "markdown"
	pageGetSpace = ""
	pageGetTitle = ""
	pageListSpace = ""
	pageListLimit = 0
	pageListStart = 0
	pageCreateSpace = ""
	pageCreateTitle = ""
	pageCreateBody = ""
	pageCreateBodyFile = ""
	pageCreateFormat = "markdown"
	pageCreateParent = ""
	pageCreateType = "page"
	pageUpdateTitle = ""
	pageUpdateBody = ""
	pageUpdateBodyFile = ""
	pageUpdateFormat = "markdown"
	pageDeletePurge = false
	pageMoveParent = ""
	pageChildrenLimit = 0
	pageChildrenStart = 0
	pageDescLimit = 0
	pageDescStart = 0
	pageRestoreVersion = 0
	commentListLocation = "all"
	commentListLimit = 0
	commentListStart = 0
	commentAddBody = ""
	commentAddReplyTo = ""
	attachListLimit = 0
	attachListStart = 0
	attachUploadFiles = nil
	attachUploadOver = false
	attachDownloadOut = ""
	labelListLimit = 0
	labelListStart = 0
	labelAddNames = nil
	labelRmNames = nil
	clockNow = time.Now
	clockSleep = time.Sleep
	pollInterval = 2 * time.Second
	output.CommandNotices = nil
	output.CompactJSON = false
	output.ErrorJSON = false
	output.EnvelopeJSON = true
	output.Quiet = false
	resetFlagValue(rootCmd.PersistentFlags(), "format", outputFormatJSON)
	resetFlagValue(rootCmd.PersistentFlags(), "compact", "false")
	resetFlagValue(rootCmd.PersistentFlags(), "fields", "")
	resetFlagValue(rootCmd.PersistentFlags(), "quiet", "false")
	resetFlagValue(rootCmd.PersistentFlags(), "dry-run", "false")
	resetFlagValue(rootCmd.PersistentFlags(), "confirm", "")
	resetFlagValue(rootCmd.PersistentFlags(), "dangerous", "false")
	resetFlagValue(rootCmd.PersistentFlags(), "force", "false")
	resetFlagValue(updateCmd.Flags(), "check", "false")
	resetFlagValue(updateCmd.Flags(), "target-version", "")
	resetFlagValue(updateCmd.Flags(), "version", "")
	resetFlagValue(authLoginCmd.Flags(), "url", "")
	resetFlagValue(authLoginCmd.Flags(), "token", "")
	resetFlagValue(changelogCmd.Flags(), "since", "")
}

func resetFlagValue(flags *pflag.FlagSet, name, value string) {
	if f := flags.Lookup(name); f != nil {
		_ = f.Value.Set(value)
		f.Changed = false
	}
}

// runRoot executes the CLI with args, capturing stdout/stderr separately.
func runRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetCmdState(t)
	lastExit = 0

	stdoutR, stdoutW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("stdout pipe: %v", pipeErr)
	}
	stderrR, stderrW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("stderr pipe: %v", pipeErr)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutW, stderrW

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stdoutBuf, stdoutR)
		close(stdoutDone)
	}()
	go func() {
		_, _ = io.Copy(&stderrBuf, stderrR)
		close(stderrDone)
	}()

	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	rootCmd.SetArgs(args)
	runErr := Execute()

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout, os.Stderr = origOut, origErr
	<-stdoutDone
	<-stderrDone
	_ = stdoutR.Close()
	_ = stderrR.Close()

	rootCmd.SetOut(origOut)
	rootCmd.SetErr(origErr)
	rootCmd.SetArgs(nil)

	return stdoutBuf.String(), stderrBuf.String(), runErr
}

// runRootWithStdin runs the CLI with the given stdin content.
func runRootWithStdin(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("stdin pipe: %v", pipeErr)
	}
	go func() {
		_, _ = io.WriteString(w, stdin)
		_ = w.Close()
	}()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })
	return runRoot(t, args...)
}

// runRootOK runs the CLI expecting success (nil error, exit 0).
func runRootOK(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := runRoot(t, args...)
	if err != nil {
		t.Fatalf("args=%v: unexpected error %v (exit=%d)\nstdout=%s\nstderr=%s", args, err, LastExitCode(), stdout, stderr)
	}
	if LastExitCode() != ExitOK {
		t.Fatalf("args=%v: exit=%d, want 0", args, LastExitCode())
	}
	return stdout, stderr
}

// runRootExpectSilent runs the CLI and asserts ErrSilent with the expected exit code.
func runRootExpectSilent(t *testing.T, code int, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := runRoot(t, args...)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("args=%v: expected ErrSilent, got %v (exit=%d)", args, err, LastExitCode())
	}
	if LastExitCode() != code {
		t.Fatalf("args=%v: exit=%d, want %d\nstdout=%s\nstderr=%s", args, LastExitCode(), code, stdout, stderr)
	}
	return stdout, stderr
}

// dryRunConfirmToken runs a write command with --dry-run and returns the
// issued confirm token.
func dryRunConfirmToken(t *testing.T, args ...string) string {
	t.Helper()
	stdout, _, err := runRoot(t, append([]string{"--dry-run"}, args...)...)
	if err != nil {
		t.Fatalf("dry-run args=%v: unexpected error %v (exit=%d)", args, err, LastExitCode())
	}
	var data struct {
		ConfirmToken string `json:"confirm_token"`
	}
	decodeEnvelopeData(t, stdout, &data)
	if data.ConfirmToken == "" {
		t.Fatalf("dry-run args=%v: no confirm_token in output:\n%s", args, stdout)
	}
	return data.ConfirmToken
}

// decodeEnvelope decodes the full success envelope and asserts ok=true.
func decodeEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v\n%s", err, out)
	}
	if env["ok"] != true {
		t.Fatalf("envelope ok=%v, want true:\n%s", env["ok"], out)
	}
	if env["schema_version"] != output.SchemaVersion {
		t.Fatalf("envelope schema_version=%v, want %s", env["schema_version"], output.SchemaVersion)
	}
	return env
}

func decodeEnvelopeData(t *testing.T, out string, v any) {
	t.Helper()
	var env struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !env.OK {
		t.Fatalf("envelope ok=false:\n%s", out)
	}
	if err := json.Unmarshal(env.Data, v); err != nil {
		t.Fatalf("invalid envelope data: %v\n%s", err, out)
	}
}

func decodeEnvelopeError(t *testing.T, out string) map[string]any {
	t.Helper()
	var env struct {
		OK    bool           `json:"ok"`
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("invalid error envelope: %v\n%s", err, out)
	}
	if env.OK {
		t.Fatalf("expected ok=false envelope: %s", out)
	}
	if env.Error == nil {
		t.Fatalf("missing error envelope: %s", out)
	}
	return env.Error
}

// containsAny reports whether s contains any of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
