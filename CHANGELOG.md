# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.1] - 2026-07-08

### Fixed

- `update` now reports the final post-install state after a successful update: `current_version == target_version` no longer comes with `update_available: true`, and the cached update notice is cleared once the installed version is current.
- Post-swap Skill-sync partial-success details now also report `target_version` and `update_available: false`, so agents can tell the binary is already at the target version even though the Skill still needs syncing.
- Package-manager installs now honor the idempotent no-op path before running `npm`/`go`, so a bare `update` does not reinstall when the running version is already current.
- Windows Go test binaries (`*.test.exe`) no longer write fake release data into the real user update-notice cache during local validation.

## [1.0.0] - 2026-07-03

### Added

- **First stable release.** Finalizes the machine-readable output contract for AI agents. Several output shapes changed since the `0.x` pre-release line — see the **breaking** entries under **Changed** below.

### Changed
- **Breaking: all list commands now use the fleet-standard `offset_style` envelope.** `page list`, `page children`, `page descendants`, `space list`, `search`, `user search`, and `page attachment/comment/label list` return `{items, count, offset, next_offset, has_more}` (plus `total_size` where the upstream reports it) instead of the previous domain-named arrays (`pages`/`spaces`/…) with `start_at`/`size`/`next_start_at`.
- **Breaking: page list/children/descendants record keys are snake_case.** `spaceKey`→`space_key` and `parentId`→`parent_id` in flattened page records.
- **Breaking: `page label remove` returns the contract batch shape.** Results are now `items[].{target, ok, error{code, retryable}}` with a `summary{total, succeeded, failed}` block, replacing the previous ad-hoc count.
- **`risk_tier` raised from `T1` to `T2`.** The tool can delete page trees and spaces (irreversible destruction of shared content), so it declares the higher worst-case tier.
- **`release_readiness` promoted to `stable`.** All 40 leaves were exercised live against a production Confluence DC (read + write + dangerous, self-cleaned); see docs/LIVE-SMOKE-EVIDENCE.md. `doctor`'s server check now reports `pass` when the version endpoint is unavailable (connectivity is already proven by the auth check) instead of a spurious `warn`.

### Fixed
- **`search` excerpts are cleaned of raw highlight markers.** The server wraps matched terms in `@@@hl@@@...@@@endhl@@@`; these delimiters are now stripped so agents get clean prose (the matched text is preserved).
- **`page delete` no longer hard-fails when the descendant endpoint is unimplemented.** The dry-run preview counted descendants via `/content/{id}/descendant/page`, which some Confluence DC versions return `501` for; the delete was fully blocked as a result. The count is now best-effort (falls back to direct children, then to `unknown`) and never blocks the delete. Found and fixed during live smoke.
- **`search` results now carry `space_key`.** `/rest/api/search` does not expand the content's space by default, so every search result returned an empty `space_key`; the request now sends `expand=content.space`. Verified live against a production Confluence DC.
- **`search --all` reports truncation.** When auto-pagination hits the 1000-item cap, the result now sets `truncated: true` so agents know the list is incomplete.
- **`reference` error_codes table lists `E_USAGE`.** The code is emitted (e.g. a CQL syntax error maps HTTP 400 → `E_USAGE`) but was missing from the documented table.
- **`page get` markdown metadata moved off the reserved `meta` key.** The conversion fidelity object was written to `data.meta`, colliding with the envelope's reserved `meta`; it is now `data.body_fidelity`.
- **`doctor` pass checks emit `fix: null`.** Passing checks previously omitted `fix`; every check now carries `check`/`status`/`fix` (`fix` is `null` when the check passes).
- **`context` surfaces update notices under a top-level `notices` key** (present when the update cache has content), matching the self-description contract.

### Deprecated

### Removed

### Security
- **PAT no longer leaks across a cross-host redirect.** The HTTP client's `CheckRedirect` re-applied `Authorization: Bearer <PAT>` on every hop, defeating Go's cross-origin credential stripping; a redirect to a foreign host received the raw token. It is now re-applied only when the redirect target is the same origin as the configured base host, and stripped otherwise.
- **Attachment download closed to SSRF via redirect.** The SSRF guard only checked the initial URL; a same-host download link could still `302` to an internal address (carrying the PAT). Downloads now refuse any redirect that leaves the configured origin, on every hop.
- **Rendered links can no longer carry dangerous URL schemes.** `MarkdownToStorage` wrote link/autolink hrefs with only HTML-entity escaping, so `javascript:`/`data:`/`vbscript:` survived into stored XHTML (stored XSS on click). Hrefs are now scheme-allowlisted (http/https/mailto); other schemes become `#`.
- **Storage->Markdown can no longer inject Markdown.** Untrusted `href`/`src`/`ri:value` were concatenated raw into Markdown `(...)`, letting a crafted URL break out and inject links/images/headings. Destinations are now percent-encoded (parens/spaces/angle brackets/control bytes) and dangerous schemes neutralized.
- **Confirm tokens are single-use across concurrent processes.** The check-then-mark used a process-local mutex over a shared JSON file, so two processes replaying one token could both pass. Consumption is now an atomic `O_CREATE|O_EXCL` per-token marker create.
- **`page history` tags external content `_untrusted`.** `created_by`, `versions[].by` and `versions[].message` were returned untagged; they are now marked `_untrusted`.
- **Label names tagged `_untrusted`.** `page label list` and `page label add` mark the external `name` field `_untrusted`, so agents treat label text as data, not instructions.
- **Confirm tokens now bind the full write payload.** `page create`/`page update`, `space update`, and `page comment add` fold the body/name (and a body SHA-256) into the dry-run confirm token, so a token cannot be replayed against different content (CLI-SPEC §7).

<!--
Copy the block below for each release. Newest version first.
Keep the link references at the bottom of the file in sync.

## [X.Y.Z] - YYYY-MM-DD

### Added

- First public release.

### Changed

### Fixed

### Deprecated

### Removed

### Security

[Unreleased]: https://github.com/fatecannotbealtered/confluence-cli/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/fatecannotbealtered/confluence-cli/releases/tag/v0.1.0
-->
