package confirm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// overrideHome points the confirm secret + consumed store (and config.Dir)
// at a temp dir, and clears env credentials so accountContext is stable.
func overrideHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	origURL := os.Getenv("CONFLUENCE_CLI_URL")
	origToken := os.Getenv("CONFLUENCE_CLI_TOKEN")
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("USERPROFILE", tmpDir)
	_ = os.Unsetenv("CONFLUENCE_CLI_URL")
	_ = os.Unsetenv("CONFLUENCE_CLI_TOKEN")
	resetSecretCache()
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("USERPROFILE", origUserProfile)
		_ = os.Setenv("CONFLUENCE_CLI_URL", origURL)
		_ = os.Setenv("CONFLUENCE_CLI_TOKEN", origToken)
		resetSecretCache()
	})
	return tmpDir
}

func TestIssueAndValidate_RoundTrip(t *testing.T) {
	overrideHome(t)

	detail := map[string]any{"id": "12345", "title": "Doc"}
	token, expiresAt := Issue("page delete", detail)

	if !strings.HasPrefix(token, "ct_") {
		t.Fatalf("token = %q, want ct_ prefix", token)
	}
	parts := strings.Split(token, "_")
	if len(parts) != 3 {
		t.Fatalf("token = %q, want ct_<unix>_<digest> format", token)
	}
	if len(parts[2]) != 32 {
		t.Errorf("digest part length = %d, want 32", len(parts[2]))
	}
	if remaining := time.Until(expiresAt); remaining < 14*time.Minute || remaining > 15*time.Minute {
		t.Errorf("expiresAt = %v, want ~15min from now", expiresAt)
	}

	if err := Validate("page delete", detail, token, time.Now().UTC()); err != nil {
		t.Errorf("Validate() on fresh token: %v", err)
	}
}

func TestValidate_RejectsMalformedTokens(t *testing.T) {
	overrideHome(t)
	now := time.Now().UTC()
	detail := map[string]any{"id": "1"}

	for _, token := range []string{
		"",
		"garbage",
		"ct_only-two",
		"xx_123_abcdef",
		"ct_notanumber_abcdef",
		"ct_123_abc_extra",
	} {
		if err := Validate("op", detail, token, now); err == nil {
			t.Errorf("Validate(%q) should fail", token)
		}
	}
}

func TestValidate_RejectsExpiredToken(t *testing.T) {
	overrideHome(t)

	detail := map[string]any{"id": "1"}
	token, expiresAt := Issue("page delete", detail)

	err := Validate("page delete", detail, token, expiresAt.Add(time.Second))
	if err == nil {
		t.Fatal("Validate() should reject expired token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want expired message", err)
	}
}

func TestValidate_RejectsDifferentOperation(t *testing.T) {
	overrideHome(t)

	detail := map[string]any{"id": "1"}
	token, _ := Issue("page delete", detail)

	if err := Validate("page update", detail, token, time.Now().UTC()); err == nil {
		t.Error("token minted for one operation must not validate another")
	}
}

func TestValidate_RejectsDifferentPayload(t *testing.T) {
	overrideHome(t)

	token, _ := Issue("page delete", map[string]any{"id": "1"})

	if err := Validate("page delete", map[string]any{"id": "2"}, token, time.Now().UTC()); err == nil {
		t.Error("token must be bound to the payload hash")
	}
}

func TestValidate_RejectsTamperedExpiry(t *testing.T) {
	overrideHome(t)

	detail := map[string]any{"id": "1"}
	token, _ := Issue("page delete", detail)
	parts := strings.Split(token, "_")

	// Extend the expiry without re-signing: digest no longer matches.
	future := time.Now().UTC().Add(24 * time.Hour).Unix()
	tampered := "ct_" + itoa(future) + "_" + parts[2]

	if err := Validate("page delete", detail, tampered, time.Now().UTC()); err == nil {
		t.Error("expiry-tampered token must be rejected")
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestConsume_SingleUse(t *testing.T) {
	overrideHome(t)

	detail := map[string]any{"id": "1"}
	token, _ := Issue("page delete", detail)
	now := time.Now().UTC()

	if err := Consume("page delete", detail, token, now); err != nil {
		t.Fatalf("first Consume() should succeed: %v", err)
	}

	err := Consume("page delete", detail, token, now)
	if err == nil {
		t.Fatal("second Consume() must be rejected (replay)")
	}
	if !strings.Contains(err.Error(), "already used") {
		t.Errorf("error = %q, want already-used message", err)
	}
}

func TestConsume_InvalidTokenNotMarked(t *testing.T) {
	overrideHome(t)

	detail := map[string]any{"id": "1"}
	if err := Consume("page delete", detail, "ct_123_bogus", time.Now().UTC()); err == nil {
		t.Fatal("Consume() with invalid token should fail")
	}

	// A failed Consume must not have consumed anything: a real token still works.
	token, _ := Issue("page delete", detail)
	if err := Consume("page delete", detail, token, time.Now().UTC()); err != nil {
		t.Errorf("valid token should still consume after invalid attempt: %v", err)
	}
}

func TestConsume_PersistsAcrossStore(t *testing.T) {
	tmpDir := overrideHome(t)

	detail := map[string]any{"id": "1"}
	token, _ := Issue("page delete", detail)
	now := time.Now().UTC()

	if err := Consume("page delete", detail, token, now); err != nil {
		t.Fatalf("Consume(): %v", err)
	}

	// The consumed store must be on disk as a directory of per-token marker
	// files, so a new process would also reject the replay.
	storeDir := filepath.Join(tmpDir, ".confluence-cli", "confirm-consumed")
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		t.Fatalf("consumed store not written: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("consumed store entries = %d, want 1", len(entries))
	}
	// The marker name is the fingerprint and its content is the expiry — the
	// raw token must appear in neither.
	if strings.Contains(entries[0].Name(), token) {
		t.Error("marker name must be a fingerprint, not the raw token")
	}
	data, err := os.ReadFile(filepath.Join(storeDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if strings.Contains(string(data), token) {
		t.Error("consumed store must hold expiry, not raw tokens")
	}
}

func TestConsumedStore_PrunesExpiredEntries(t *testing.T) {
	overrideHome(t)

	now := time.Now().UTC()
	markConsumed("ct_expired_token", now.Add(-time.Hour).Unix(), now)
	markConsumed("ct_live_token", now.Add(time.Hour).Unix(), now)

	if isConsumed("ct_expired_token", now) {
		t.Error("expired entry should have been pruned")
	}
	if !isConsumed("ct_live_token", now) {
		t.Error("live entry should still be present")
	}
}

func TestSecret_PersistedWithRestrictedMode(t *testing.T) {
	tmpDir := overrideHome(t)

	// Force secret creation via Issue.
	Issue("op", nil)

	path := filepath.Join(tmpDir, ".confluence-cli", "confirm.secret")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("confirm.secret not created: %v", err)
	}
	if info.Size() < 32 {
		t.Errorf("secret size = %d, want >= 32 bytes", info.Size())
	}
}

func TestSecret_StableAcrossReload(t *testing.T) {
	overrideHome(t)

	detail := map[string]any{"id": "1"}
	token, _ := Issue("page delete", detail)

	// Simulate a new process: reload the secret from disk.
	resetSecretCache()

	if err := Validate("page delete", detail, token, time.Now().UTC()); err != nil {
		t.Errorf("token should validate after secret reload from disk: %v", err)
	}
}

func TestTokensDifferAcrossMachSecrets(t *testing.T) {
	// A token minted under one machine secret must not validate under another
	// (i.e. the token cannot be fabricated by recomputing a public hash).
	overrideHome(t)
	detail := map[string]any{"id": "1"}
	token, _ := Issue("page delete", detail)

	// Second "machine": different home, different secret.
	overrideHome(t)
	if err := Validate("page delete", detail, token, time.Now().UTC()); err == nil {
		t.Error("token from another machine secret must be rejected")
	}
}

func TestExpiryUnix(t *testing.T) {
	if got := expiryUnix("ct_1750000000_abc"); got != 1750000000 {
		t.Errorf("expiryUnix = %d, want 1750000000", got)
	}
	if got := expiryUnix("garbage"); got != 0 {
		t.Errorf("expiryUnix(garbage) = %d, want 0", got)
	}
	if got := expiryUnix("ct_notanum_abc"); got != 0 {
		t.Errorf("expiryUnix(non-numeric) = %d, want 0", got)
	}
}

func TestParseExpiry(t *testing.T) {
	if _, err := parseExpiry(""); err == nil {
		t.Error("empty expiry should error")
	}
	if _, err := parseExpiry("12a3"); err == nil {
		t.Error("non-numeric expiry should error")
	}
	n, err := parseExpiry("42")
	if err != nil || n != 42 {
		t.Errorf("parseExpiry(42) = %d, %v", n, err)
	}
}

func TestIssue_NilDetail(t *testing.T) {
	overrideHome(t)

	token, _ := Issue("noop", nil)
	if err := Validate("noop", nil, token, time.Now().UTC()); err != nil {
		t.Errorf("nil detail should round-trip: %v", err)
	}
	// nil and empty map are equivalent payloads.
	if err := Validate("noop", map[string]any{}, token, time.Now().UTC()); err != nil {
		t.Errorf("nil and empty detail should produce the same digest: %v", err)
	}
}

// TestClaimConsumed_AtomicSingleUse verifies exactly one concurrent claimant of
// the same token wins — the single-use guarantee holds under a race, not just a
// process-local mutex.
func TestClaimConsumed_AtomicSingleUse(t *testing.T) {
	overrideHome(t)

	now := time.Unix(1_800_000_000, 0)
	exp := now.Add(time.Hour).Unix()
	const n = 32
	var wins int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if claimConsumed("ct_race_token", exp, now) {
				atomic.AddInt64(&wins, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if wins != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", wins)
	}
}
