package cmd

import (
	"net/http"
	"strings"
	"testing"
)

// ─── root / global flags ────────────────────────────────────────────────────

func TestRootHelp_DocumentsGlobalFlags(t *testing.T) {
	help, _, err := runRoot(t, "--help")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	for _, flag := range []string{"--format", "--compact", "--fields", "--dry-run", "--confirm", "--dangerous", "--quiet"} {
		if !strings.Contains(help, flag) {
			t.Errorf("help should document %s", flag)
		}
	}
}

func TestRoot_InvalidFormatRejected(t *testing.T) {
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "--format", "xml", "context")
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_VALIDATION" {
		t.Fatalf("error=%v", errPayload)
	}
}

func TestRoot_CompactRequiresJSON(t *testing.T) {
	_, stderr := runRootExpectSilent(t, ExitBadArgs, "--format", "text", "--compact", "context")
	if !containsAny(stderr, "--compact can only be used with --format json") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestRoot_FieldsRequiresJSON(t *testing.T) {
	_, stderr := runRootExpectSilent(t, ExitBadArgs, "--format", "text", "--fields", "url", "context")
	if !containsAny(stderr, "--fields can only be used with --format json") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestRoot_RawNotSupportedByContext(t *testing.T) {
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "--format", "raw", "context")
	errPayload := decodeEnvelopeError(t, stdout)
	if !strings.Contains(errPayload["message"].(string), "does not support --format raw") {
		t.Fatalf("error=%v", errPayload)
	}
}

func TestRoot_UnknownCommandJSONError(t *testing.T) {
	stdout, _ := runRootExpectSilent(t, ExitBadArgs, "no-such-command")
	errPayload := decodeEnvelopeError(t, stdout)
	if errPayload["code"] != "E_VALIDATION" {
		t.Fatalf("error=%v", errPayload)
	}
}

// ─── context ────────────────────────────────────────────────────────────────

func TestContext_NotConfiguredJSON(t *testing.T) {
	setTempHome(t)
	clearConfluenceEnv(t)
	stdout, _ := runRootOK(t, "context")
	var doc contextDocument
	decodeEnvelopeData(t, stdout, &doc)
	if doc.Tool != "confluence-cli" || doc.Version == "" {
		t.Fatalf("doc=%+v", doc)
	}
	if doc.Credentials.Status != "not_configured" || doc.Config.Configured {
		t.Fatalf("credentials=%+v", doc.Credentials)
	}
	if doc.Env["CONFLUENCE_CLI_URL"] != "unset" || doc.Env["CONFLUENCE_CLI_TOKEN"] != "unset" {
		t.Fatalf("env=%v", doc.Env)
	}
}

func TestContext_ValidCredentialsEnvSource(t *testing.T) {
	mockConfluenceServer(t, currentUserHandler(t))
	stdout, _ := runRootOK(t, "context")
	var doc contextDocument
	decodeEnvelopeData(t, stdout, &doc)
	if doc.Credentials.Status != "valid" || doc.Credentials.Source != "env" {
		t.Fatalf("credentials=%+v", doc.Credentials)
	}
	if doc.Account == nil || doc.Account.Username != "jdoe" {
		t.Fatalf("account=%+v", doc.Account)
	}
	if doc.Env["CONFLUENCE_CLI_URL"] != "set" || doc.Env["CONFLUENCE_CLI_TOKEN"] != "set" {
		t.Fatalf("env=%v", doc.Env)
	}
	if strings.Contains(stdout, "test-pat-token") {
		t.Fatalf("context leaked the plaintext token:\n%s", stdout)
	}
}

func TestContext_InvalidCredentials(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"statusCode":401,"message":"expired"}`))
	})
	stdout, _ := runRootOK(t, "context")
	var doc contextDocument
	decodeEnvelopeData(t, stdout, &doc)
	if doc.Credentials.Status != "invalid" || doc.Credentials.Error == "" {
		t.Fatalf("credentials=%+v", doc.Credentials)
	}
}

func TestContext_Text(t *testing.T) {
	mockConfluenceServer(t, currentUserHandler(t))
	stdout, _ := runRootOK(t, "--format", "text", "context")
	if !containsAny(stdout, "confluence-cli context", "Auth: valid") {
		t.Fatalf("stdout=%q", stdout)
	}
}

// ─── doctor ─────────────────────────────────────────────────────────────────

func TestDoctor_NotConfigured(t *testing.T) {
	setTempHome(t)
	clearConfluenceEnv(t)
	stdout, _ := runRootExpectSilent(t, ExitAuth, "doctor")
	var result doctorResult
	decodeEnvelopeData(t, stdout, &result)
	if len(result.Checks) < 2 {
		t.Fatalf("checks=%v", result.Checks)
	}
	if result.Checks[0].Check != "config" || result.Checks[0].Status != "fail" || result.Checks[0].Fix == "" {
		t.Fatalf("config check should fail with a hint: %+v", result.Checks[0])
	}
	last := result.Checks[len(result.Checks)-1]
	if last.Check != "release_readiness" || last.Status != "pass" {
		t.Fatalf("release_readiness should be declared and pass while stable: %+v", last)
	}
}

func TestDoctor_AuthFailure(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"statusCode":401,"message":"expired"}`))
	})
	stdout, _ := runRootExpectSilent(t, ExitAuth, "doctor")
	var result doctorResult
	decodeEnvelopeData(t, stdout, &result)
	byName := doctorChecksByName(result)
	if byName["config"] != "pass" || byName["auth"] != "fail" || byName["network"] != "fail" {
		t.Fatalf("checks=%v", result.Checks)
	}
}

func TestDoctor_AllChecksJSON(t *testing.T) {
	mockConfluenceServer(t, currentUserHandler(t))
	stdout, _ := runRootOK(t, "doctor")
	var result doctorResult
	decodeEnvelopeData(t, stdout, &result)
	byName := doctorChecksByName(result)
	if byName["config"] != "pass" || byName["network"] != "pass" || byName["auth"] != "pass" || byName["server"] != "pass" {
		t.Fatalf("checks=%v", result.Checks)
	}
	if byName["release_readiness"] != "pass" {
		t.Fatalf("release_readiness must pass while declared stable: %v", result.Checks)
	}
	if result.Username != "jdoe" || result.ServerVersion != "8.5.4" {
		t.Fatalf("result=%+v", result)
	}
}

func TestDoctor_SystemInfoUnavailableIsPass(t *testing.T) {
	mockConfluenceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/user/current" {
			_, _ = w.Write([]byte(`{"username":"jdoe","displayName":"John Doe"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"message":"admin only"}`))
	})
	stdout, _ := runRootOK(t, "doctor")
	var result doctorResult
	decodeEnvelopeData(t, stdout, &result)
	// Auth already proved reachability; an unreadable version endpoint (403/404
	// on some DC versions) is informational, so the server check still passes.
	if doctorChecksByName(result)["server"] != "pass" {
		t.Fatalf("unreadable systemInfo should pass (connectivity proven by auth): %v", result.Checks)
	}
}

func TestDoctor_Text(t *testing.T) {
	mockConfluenceServer(t, currentUserHandler(t))
	stdout, _ := runRootOK(t, "--format", "text", "doctor")
	if !containsAny(stdout, "confluence-cli Doctor", "Authenticated as John Doe") {
		t.Fatalf("stdout=%q", stdout)
	}
}

func doctorChecksByName(result doctorResult) map[string]string {
	byName := map[string]string{}
	for _, c := range result.Checks {
		byName[c.Check] = c.Status
	}
	return byName
}

// ─── changelog ──────────────────────────────────────────────────────────────

func TestChangelog_JSON(t *testing.T) {
	stdout, _ := runRootOK(t, "changelog")
	var data changelogResult
	decodeEnvelopeData(t, stdout, &data)
	if len(data.Entries) == 0 {
		t.Fatal("changelog should expose at least one entry (Unreleased)")
	}
	if data.Entries[0].Version != "Unreleased" {
		t.Fatalf("first entry=%+v", data.Entries[0])
	}
}

func TestChangelog_Since(t *testing.T) {
	stdout, _ := runRootOK(t, "changelog", "--since", "999.0.0")
	var data changelogResult
	decodeEnvelopeData(t, stdout, &data)
	if data.Since != "999.0.0" {
		t.Fatalf("since=%q", data.Since)
	}
	for _, e := range data.Entries {
		if e.Version != "Unreleased" {
			t.Fatalf("--since 999.0.0 should only keep Unreleased, got %+v", e)
		}
	}
}

func TestChangelog_Text(t *testing.T) {
	stdout, _ := runRootOK(t, "--format", "text", "changelog")
	if !containsAny(stdout, "confluence-cli changelog") {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestParseChangelog_CategoriesAndOrdering(t *testing.T) {
	md := `# Changelog

## [Unreleased]

## [0.2.0] - 2026-02-01

### Added

- feature B

## [0.1.0] - 2026-01-01

### Added

- feature A

### Fixed

- bug X
`
	entries := parseChangelog(md)
	if len(entries) != 3 {
		t.Fatalf("entries=%v", entries)
	}
	if entries[0].Version != "Unreleased" || entries[1].Version != "0.2.0" || entries[2].Version != "0.1.0" {
		t.Fatalf("order=%v", entries)
	}
	if entries[2].Changes["fixed"][0] != "bug X" {
		t.Fatalf("changes=%v", entries[2].Changes)
	}
}

// ─── reference ──────────────────────────────────────────────────────────────

func TestReference_JSONDocument(t *testing.T) {
	stdout, _ := runRootOK(t, "reference")
	var doc struct {
		Tool             string            `json:"tool"`
		Version          string            `json:"version"`
		SchemaVersion    string            `json:"schema_version"`
		RiskTier         string            `json:"risk_tier"`
		ReleaseReadiness releaseReadiness  `json:"release_readiness"`
		Commands         []map[string]any  `json:"commands"`
		ExitCodes        map[string]string `json:"exit_codes"`
		ErrorCodes       map[string]string `json:"error_codes"`
		Schemas          map[string]any    `json:"schemas"`
	}
	decodeEnvelopeData(t, stdout, &doc)
	if doc.Tool != "confluence-cli" || doc.SchemaVersion != "1.0" || doc.RiskTier != "T1" {
		t.Fatalf("doc header=%+v", doc)
	}
	if doc.ReleaseReadiness.Level != "stable" {
		t.Fatalf("release level=%q, want stable", doc.ReleaseReadiness.Level)
	}
	if doc.ExitCodes["130"] == "" || doc.ErrorCodes["E_INTERRUPTED"] == "" {
		t.Fatal("exit/error code tables must include the interrupted mapping")
	}
	paths := map[string]bool{}
	for _, c := range doc.Commands {
		p, _ := c["path"].(string)
		paths[p] = true
	}
	for _, want := range []string{
		"confluence-cli auth login",
		"confluence-cli auth logout",
		"confluence-cli auth status",
		"confluence-cli context",
		"confluence-cli doctor",
		"confluence-cli changelog",
		"confluence-cli reference",
	} {
		if !paths[want] {
			t.Errorf("reference should enumerate %q; got %v", want, paths)
		}
	}
}

func TestReference_LeafMetadataComplete(t *testing.T) {
	// Every enumerated leaf must have a schema label that resolves in the
	// schema catalog, so output_schema can never be a stub.
	stdout, _ := runRootOK(t, "reference")
	var doc struct {
		Commands []struct {
			Path         string `json:"path"`
			OutputSchema string `json:"output_schema"`
			Examples     []any  `json:"examples"`
		} `json:"commands"`
		Schemas map[string]any `json:"schemas"`
	}
	decodeEnvelopeData(t, stdout, &doc)
	if len(doc.Commands) == 0 {
		t.Fatal("no commands enumerated")
	}
	for _, c := range doc.Commands {
		if c.OutputSchema == "" {
			t.Errorf("%s has no output_schema", c.Path)
			continue
		}
		if _, ok := doc.Schemas[c.OutputSchema]; !ok {
			t.Errorf("%s references schema %q not in catalog", c.Path, c.OutputSchema)
		}
		if len(c.Examples) == 0 {
			t.Errorf("%s has no examples", c.Path)
		}
	}
}

func TestReference_WritePermissionTiers(t *testing.T) {
	stdout, _ := runRootOK(t, "reference")
	var doc struct {
		Commands []struct {
			Path           string `json:"path"`
			PermissionTier string `json:"permission_tier"`
			Write          bool   `json:"write"`
		} `json:"commands"`
	}
	decodeEnvelopeData(t, stdout, &doc)
	tiers := map[string]string{}
	for _, c := range doc.Commands {
		tiers[c.Path] = c.PermissionTier
	}
	if tiers["confluence-cli auth login"] != "write" || tiers["confluence-cli auth logout"] != "write" {
		t.Fatalf("auth login/logout should be write tier: %v", tiers)
	}
	if tiers["confluence-cli auth status"] != "read" || tiers["confluence-cli doctor"] != "read" {
		t.Fatalf("status/doctor should be read tier: %v", tiers)
	}
}

func TestReference_Text(t *testing.T) {
	text, _ := runRootOK(t, "--format", "text", "reference")
	if !strings.Contains(text, "# confluence-cli Command Reference") {
		t.Error("text reference should start with header")
	}
	if !strings.Contains(text, "## confluence-cli auth") {
		t.Error("text reference should document auth group")
	}
}

// ─── envelope conformance across self-description leaves ───────────────────

func TestSelfDescription_EnvelopeShape(t *testing.T) {
	setTempHome(t)
	clearConfluenceEnv(t)
	for _, args := range [][]string{
		{"reference"},
		{"changelog"},
	} {
		stdout, _ := runRootOK(t, args...)
		env := decodeEnvelope(t, stdout)
		meta, ok := env["meta"].(map[string]any)
		if !ok {
			t.Fatalf("%v: envelope missing meta: %v", args, env)
		}
		if _, ok := meta["duration_ms"]; !ok {
			t.Fatalf("%v: meta missing duration_ms: %v", args, meta)
		}
	}
}

func TestCompactJSON_SingleLine(t *testing.T) {
	stdout, _ := runRootOK(t, "--compact", "changelog")
	if strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
		t.Fatalf("--compact should emit single-line JSON:\n%s", stdout)
	}
}
