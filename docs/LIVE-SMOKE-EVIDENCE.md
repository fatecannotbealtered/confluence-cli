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

## 2026-07-03 — v0.1.0 write-path smoke (live, personal space, self-cleaned)

Run against the same production Confluence DC, **scoped entirely to the caller's
personal space** (`~<user>`). Every resource was created under one throwaway
root page and **purged at the end**; the instance was left clean (all test IDs
verified `E_NOT_FOUND` afterwards). All writes used the `--dry-run` →
`--confirm <token>` two-step; dangerous commands required `--dangerous` on both.

| Command / behavior | Result | Notes |
|---|---|---|
| `page create` (root + child + parent2) | PASS | markdown→storage body; child created under `--parent` |
| `page update` | PASS | title+body change; version 1→2; read-back confirmed |
| `page move` | PASS | reparented child; `page ancestors` reflected the new parent |
| `page restore --version 1` | PASS | body/title reverted to v1 |
| `page comment add` / `page comment delete` | PASS | comment created then deleted (dangerous) |
| `page attachment upload` / `page attachment delete` | PASS | file uploaded then deleted (dangerous) |
| `page label add` / `page label remove` | PASS | `smoke,cc-test` added; `cc-test` removed; list confirmed |
| `page delete --purge` (×3 cleanup) | PASS | dangerous two-step; all test pages permanently removed |
| `space create` | PASS (E_FORBIDDEN) | normal PAT lacks space-admin; server-side 403 → `E_FORBIDDEN` as designed. `space update`/`space delete` not exercised (require an admin-owned throwaway space) |

### Bug found and fixed during this smoke

- **`page delete` was fully blocked on this DC.** The dry-run preview computed a
  descendant count via `/content/{id}/descendant/page`, which this Confluence
  version does not implement (HTTP **501** "Page children is currently only
  supported for direct children"). An informational count must never block a
  delete. FIXED: the count is now best-effort — it falls back to the
  direct-children count (scope `direct_children_only`) and, if that also fails,
  reports `-1`/`unknown`; the delete proceeds regardless. Re-verified live: the
  three cleanup purges succeeded after the fix.
- **`page descendants` returns `E_SERVER` (501) on this DC** — the recursive
  endpoint is genuinely absent on this server version. Left as an honest failure
  (the server message is passed through); `page children` covers direct children.
