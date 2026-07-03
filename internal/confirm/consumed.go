package confirm

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatecannotbealtered/confluence-cli/internal/config"
)

// Single-use confirm tokens: once a confirm token has been accepted to execute
// a write, its fingerprint is recorded so the SAME token cannot drive a second
// write. This gives agents safe-retry semantics — a confirmed write that times
// out cannot be blindly replayed; the retry is rejected with E_CONFLICT and the
// agent must re-run --dry-run (which reveals the now-current state).
//
// The store is a directory of marker files, one per consumed token, at
// ~/.confluence-cli/confirm-consumed/<fingerprint> (dir 0700, files 0600). Each
// marker's content is its expiry (unix seconds) so expired markers can be
// pruned. A per-token marker created with O_CREATE|O_EXCL is the single-use
// primitive: claiming is an ATOMIC file create, so two concurrent processes
// replaying the same token cannot both win — the loser gets os.IsExist and is
// rejected. This closes the check-then-mark TOCTOU that a shared JSON map (read
// -modify-write) guarded only by a process-local mutex left open across
// processes.
var consumedMu sync.Mutex

func consumedDir() string {
	return filepath.Join(config.Dir(), "confirm-consumed")
}

// fingerprint is a short, non-reversible id for a confirm token.
func fingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

func markerPath(token string) string {
	return filepath.Join(consumedDir(), fingerprint(token))
}

// pruneExpired removes marker files whose recorded expiry is at or before now.
// Best effort: unreadable or malformed markers are left untouched.
func pruneExpired(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if exp, ok := readExpiry(p); ok && exp <= now.Unix() {
			_ = os.Remove(p)
		}
	}
}

func readExpiry(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	exp, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false
	}
	return exp, true
}

// isConsumed reports whether this token has already been used and not expired.
func isConsumed(token string, now time.Time) bool {
	consumedMu.Lock()
	defer consumedMu.Unlock()
	exp, ok := readExpiry(markerPath(token))
	return ok && exp > now.Unix()
}

// markConsumed records the token as used until its expiry. Best effort: a
// storage failure does not block the write (single-use simply cannot be
// guaranteed on that host).
func markConsumed(token string, expiresUnix int64, now time.Time) {
	consumedMu.Lock()
	defer consumedMu.Unlock()
	dir := consumedDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	pruneExpired(dir, now)
	_ = os.WriteFile(markerPath(token), []byte(strconv.FormatInt(expiresUnix, 10)), 0o600)
}

// claimConsumed atomically claims single-use ownership of a token. It returns
// true if THIS caller won the claim (the token had not been consumed and may
// now proceed), or false if the token was already consumed (replay → reject).
// The claim is an atomic O_CREATE|O_EXCL marker create, so it is race-free
// across concurrent processes, not just goroutines.
//
// Storage failures other than "already exists" do not block the write: as with
// the previous best-effort marker, single-use cannot be guaranteed on a host
// where the store is unwritable, and blocking would be worse than a rare replay.
func claimConsumed(token string, expiresUnix int64, now time.Time) bool {
	consumedMu.Lock()
	defer consumedMu.Unlock()

	dir := consumedDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return true // unwritable store: fall back to allowing the write
	}
	pruneExpired(dir, now)

	path := markerPath(token)
	content := []byte(strconv.FormatInt(expiresUnix, 10))
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.Write(content)
			_ = f.Close()
			return true // we created the marker → first and only claimant
		}
		if !os.IsExist(err) {
			return true // unexpected error: fall back to allowing the write
		}
		// Marker exists. If it is an expired remnant that pruning raced past,
		// remove it and retry the atomic create exactly once; otherwise it is a
		// genuine prior consumption → reject.
		if exp, ok := readExpiry(path); ok && exp <= now.Unix() {
			_ = os.Remove(path)
			continue
		}
		return false
	}
	return false
}
