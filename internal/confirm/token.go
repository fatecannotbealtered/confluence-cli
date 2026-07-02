package confirm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/config"
	"github.com/fatecannotbealtered/confluence-cli/internal/contract"
)

// TTL is how long an issued confirm token stays valid.
const TTL = 15 * time.Minute

// tokenPayload binds everything a confirm token authorizes: schema version,
// the operation name, the operation payload, the calling account context, and
// the expiry. Changing any of these invalidates the token (E_CONFLICT).
func tokenPayload(operation string, detail map[string]any, expiresAt time.Time) map[string]any {
	if detail == nil {
		detail = map[string]any{}
	}
	return map[string]any{
		"schema_version": contract.SchemaVersion,
		"operation":      operation,
		"details":        detail,
		"context":        accountContext(),
		"expires_at":     expiresAt.UTC().Format(time.RFC3339),
	}
}

// accountContext binds the token to the configured instance and (redacted)
// credential, so a token minted for one account cannot confirm a write as
// another.
func accountContext() map[string]any {
	cfg, err := config.Load()
	if err != nil {
		return map[string]any{"config": "unavailable"}
	}
	ctx := map[string]any{
		"url": cfg.URL,
	}
	if strings.TrimSpace(cfg.Token) != "" {
		sum := sha256.Sum256([]byte(strings.TrimSpace(cfg.Token)))
		ctx["token_sha256"] = hex.EncodeToString(sum[:])[:16]
	}
	return ctx
}

func digest(operation string, detail map[string]any, expiresAt time.Time) (string, error) {
	data, err := json.Marshal(tokenPayload(operation, detail, expiresAt))
	if err != nil {
		return "", err
	}
	sum := digest32(data)
	return hex.EncodeToString(sum[:]), nil
}

// Issue mints a confirm token for the operation, valid for TTL from now.
// Token format: ct_<unixExpiry>_<hmacDigest[:32]>.
func Issue(operation string, detail map[string]any) (token string, expiresAt time.Time) {
	expiresAt = time.Now().UTC().Add(TTL)
	d, err := digest(operation, detail, expiresAt)
	if err != nil {
		return "ct_invalid", expiresAt
	}
	return fmt.Sprintf("ct_%d_%s", expiresAt.Unix(), d[:32]), expiresAt
}

// Validate checks a token against the operation without consuming it.
// Any error maps to E_CONFLICT at the command layer.
func Validate(operation string, detail map[string]any, token string, now time.Time) error {
	parts := strings.Split(token, "_")
	if len(parts) != 3 || parts[0] != "ct" {
		return fmt.Errorf("invalid confirm token")
	}
	expiresUnix, err := parseExpiry(parts[1])
	if err != nil {
		return fmt.Errorf("invalid confirm token")
	}
	expiresAt := time.Unix(expiresUnix, 0).UTC()
	if now.After(expiresAt) {
		return fmt.Errorf("confirm token expired")
	}
	d, err := digest(operation, detail, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to validate confirm token: %w", err)
	}
	if parts[2] != d[:32] {
		return fmt.Errorf("confirm token does not match this operation")
	}
	return nil
}

// Consume validates the token and marks it used, enforcing single consumption:
// a replay is rejected so a confirmed write that timed out cannot be blindly
// re-sent (the agent must re-run --dry-run). The token is marked consumed
// BEFORE the caller executes the write, so a crash mid-write conservatively
// blocks the replay rather than risking a duplicate. Any error maps to
// E_CONFLICT at the command layer.
func Consume(operation string, detail map[string]any, token string, now time.Time) error {
	if err := Validate(operation, detail, token, now); err != nil {
		return err
	}
	if isConsumed(token, now) {
		return fmt.Errorf("confirm token already used; the operation may have completed — re-run --dry-run to see current state")
	}
	markConsumed(token, expiryUnix(token), now)
	return nil
}

// expiryUnix extracts the expiry unix seconds from a ct_<unix>_<digest> token,
// or 0 if it cannot be parsed (the consumed-token entry then prunes on the
// next access).
func expiryUnix(token string) int64 {
	parts := strings.Split(token, "_")
	if len(parts) != 3 {
		return 0
	}
	n, err := parseExpiry(parts[1])
	if err != nil {
		return 0
	}
	return n
}

func parseExpiry(s string) (int64, error) {
	var n int64
	if s == "" {
		return 0, fmt.Errorf("empty expiry")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid expiry")
		}
		n = n*10 + int64(ch-'0')
	}
	return n, nil
}
