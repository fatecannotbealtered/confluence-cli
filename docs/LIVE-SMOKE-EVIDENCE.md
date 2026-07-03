# Live Smoke Evidence

Recorded live smoke for `release_readiness`, run against a **real Confluence
Data Center** instance.

- **Date:** 2026-07-03
- **Target:** a production Confluence Data Center (REST API v1). Host, token, and
  all returned content are intentionally **not** recorded here — only aggregate
  counts, IDs, and pass/fail. Auth is a Personal Access Token supplied via the
  `CONFLUENCE_CLI_TOKEN` env override; only the token's SHA-256 fingerprint ever
  appears in output.
- **Method:** each command invoked with `--format json`; envelope `ok`/`error`
  asserted. **Read-only** run — no writes, no mutations. All commands returned a
  well-formed JSON envelope with `schema_version: 1.0`.

## 2026-07-03 — v0.1.0 read-path smoke (live)

| Command / behavior | Result | Notes |
|---|---|---|
| `auth status` | PASS | `status: valid`, token shown only as `token_sha256` fingerprint, resolved username + display name |
| `doctor` | PASS (with 1 warn) | config/network/auth all `pass` (network ~225ms); `server` check `warn` — `GET /rest/api/settings/systemInfo` returns 404 on this DC (endpoint admin-only or moved); connectivity otherwise fine |
| `user current` | PASS | resolves authenticated user; `display_name` tagged `_untrusted` |
| `space list --limit 5` | PASS | returns real global spaces; pagination `has_more`/`next_start_at` correct; `name` tagged `_untrusted` |
| `search --type page --text test` | PASS | `total_size` 50,828; results carry clickable `url` deep links; `title`/`excerpt` tagged `_untrusted` |
| `page get <id>` (markdown) | PASS | storage→markdown conversion `fidelity: exact`; `space_key`/`version`/`parent_id`/`url` populated |
| `page get <id> --body-format storage` | PASS | raw storage passthrough |
| `page label list <id>` / `page attachment list <id>` | PASS | empty arrays for a page with none; pagination fields present |

### Issues found during this smoke (and dispositions)

- **`search` results had empty `space_key`** — FIXED same day: `/rest/api/search`
  does not expand the content's space by default; added `expand=content.space`.
  Re-verified live: results now carry `space_key` (`SHSEP`, `TKS`, `~feng28.liu`).
- **`search` excerpts contain raw Confluence highlight markers**
  (`@@@hl@@@...@@@endhl@@@`) and double-escaped HTML entities — cosmetic, from
  the server's `excerpt=highlight` mode; content is `_untrusted`. Left as-is
  pending a decision on excerpt cleanup.
- **`doctor` server check 404** on `settings/systemInfo` — this DC does not serve
  that path to a normal PAT; the check degrades to `warn`, not a failure.

All read paths are live-verified. Writes were not exercised (read-only token
policy for this session).
