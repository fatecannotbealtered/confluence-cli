# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

### Fixed
- **`search` results now carry `space_key`.** `/rest/api/search` does not expand the content's space by default, so every search result returned an empty `space_key`; the request now sends `expand=content.space`. Verified live against a production Confluence DC.

### Deprecated

### Removed

### Security
- **PAT no longer leaks across a cross-host redirect.** The HTTP client's `CheckRedirect` re-applied `Authorization: Bearer <PAT>` on every hop, defeating Go's cross-origin credential stripping; a redirect to a foreign host received the raw token. It is now re-applied only when the redirect target is the same origin as the configured base host, and stripped otherwise.
- **Attachment download closed to SSRF via redirect.** The SSRF guard only checked the initial URL; a same-host download link could still `302` to an internal address (carrying the PAT). Downloads now refuse any redirect that leaves the configured origin, on every hop.
- **Rendered links can no longer carry dangerous URL schemes.** `MarkdownToStorage` wrote link/autolink hrefs with only HTML-entity escaping, so `javascript:`/`data:`/`vbscript:` survived into stored XHTML (stored XSS on click). Hrefs are now scheme-allowlisted (http/https/mailto); other schemes become `#`.
- **Storage->Markdown can no longer inject Markdown.** Untrusted `href`/`src`/`ri:value` were concatenated raw into Markdown `(...)`, letting a crafted URL break out and inject links/images/headings. Destinations are now percent-encoded (parens/spaces/angle brackets/control bytes) and dangerous schemes neutralized.
- **Confirm tokens are single-use across concurrent processes.** The check-then-mark used a process-local mutex over a shared JSON file, so two processes replaying one token could both pass. Consumption is now an atomic `O_CREATE|O_EXCL` per-token marker create.
- **`page history` tags external content `_untrusted`.** `created_by`, `versions[].by` and `versions[].message` were returned untagged; they are now marked `_untrusted`.

<!--
Copy the block below for each release. Newest version first.
Keep the link references at the bottom of the file in sync.

## [0.1.0] - YYYY-MM-DD

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
