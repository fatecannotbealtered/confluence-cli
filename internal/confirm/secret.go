// Package confirm implements the HMAC confirm-token gate for write commands
// (CLI-SPEC §7/§9): a --dry-run issues a token bound to the operation content
// and a machine-local secret; the confirm step validates it and consumes it
// exactly once. Token format is aligned with the fleet canon (jira-cli):
// ct_<unixExpiry>_<hmacDigest[:32]>.
package confirm

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fatecannotbealtered/confluence-cli/internal/config"
)

var (
	secretMu     sync.Mutex
	secretLoaded bool
	secretValue  []byte
)

// secret returns the machine-local key that signs confirm tokens, so a token
// cannot be fabricated by recomputing a public hash: it must come from a real
// --dry-run on this machine. Created on first use at
// ~/.confluence-cli/confirm.secret with 0600 permissions.
func secret() []byte {
	secretMu.Lock()
	defer secretMu.Unlock()
	if !secretLoaded {
		secretValue = loadOrCreateSecret()
		secretLoaded = true
	}
	return secretValue
}

// resetSecretCache clears the cached secret (test seam for home-dir overrides).
func resetSecretCache() {
	secretMu.Lock()
	defer secretMu.Unlock()
	secretLoaded = false
	secretValue = nil
}

func secretPath() string {
	return filepath.Join(config.Dir(), "confirm.secret")
}

func loadOrCreateSecret() []byte {
	path := secretPath()
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return data
	}
	sec := make([]byte, 32)
	if _, err := rand.Read(sec); err != nil {
		warnSecretFallback("cannot generate random key")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		warnSecretFallback("cannot create config directory")
		return nil
	}
	if err := os.WriteFile(path, sec, 0o600); err != nil {
		warnSecretFallback("cannot persist key file")
		return nil
	}
	return sec
}

func warnSecretFallback(reason string) {
	fmt.Fprintf(os.Stderr, "warning: %s; confirm tokens fall back to unkeyed hashing\n", reason)
}

// digest32 is a drop-in replacement for sha256.Sum256 on token seeds, keyed
// with the machine-local secret when available.
func digest32(data []byte) [32]byte {
	sec := secret()
	if len(sec) == 0 {
		return sha256.Sum256(data)
	}
	mac := hmac.New(sha256.New, sec)
	mac.Write(data)
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}
