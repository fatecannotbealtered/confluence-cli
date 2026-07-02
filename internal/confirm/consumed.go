package confirm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/config"
)

// Single-use confirm tokens: once a confirm token has been accepted to execute
// a write, its fingerprint is recorded so the SAME token cannot drive a second
// write. This gives agents safe-retry semantics — a confirmed write that times
// out cannot be blindly replayed; the retry is rejected with E_CONFLICT and the
// agent must re-run --dry-run (which reveals the now-current state). The store
// lives at ~/.confluence-cli/confirm-consumed.json (0600) and is pruned of
// expired entries on every access.
var consumedMu sync.Mutex

func consumedPath() string {
	return filepath.Join(config.Dir(), "confirm-consumed.json")
}

// fingerprint is a short, non-reversible id for a confirm token.
func fingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

func loadConsumed(path string, now time.Time) map[string]int64 {
	out := map[string]int64{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var stored map[string]int64
	if json.Unmarshal(data, &stored) != nil {
		return out
	}
	// Drop expired entries so the file cannot grow without bound.
	for fp, exp := range stored {
		if exp > now.Unix() {
			out[fp] = exp
		}
	}
	return out
}

// isConsumed reports whether this token has already been used.
func isConsumed(token string, now time.Time) bool {
	consumedMu.Lock()
	defer consumedMu.Unlock()
	tokens := loadConsumed(consumedPath(), now)
	_, ok := tokens[fingerprint(token)]
	return ok
}

// markConsumed records the token as used until its expiry. Best effort: a
// storage failure does not block the write (single-use simply cannot be
// guaranteed on that host).
func markConsumed(token string, expiresUnix int64, now time.Time) {
	path := consumedPath()
	consumedMu.Lock()
	defer consumedMu.Unlock()
	tokens := loadConsumed(path, now)
	tokens[fingerprint(token)] = expiresUnix
	data, err := json.Marshal(tokens)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
