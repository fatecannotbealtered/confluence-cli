# CQL Search Reference

Quick reference for `confluence-cli search`. Positional arguments are raw CQL; convenience flags compile to CQL and are **ANDed** with the raw fragments. Live flag list: `confluence-cli reference --compact`.

- [Fields](#fields)
- [Operators](#operators)
- [Functions](#functions)
- [Dates](#dates)
- [Convenience flag → CQL mapping](#convenience-flag--cql-mapping)
- [Sorting and paging](#sorting-and-paging)
- [Combining raw CQL with flags](#combining-raw-cql-with-flags)
- [Examples](#examples)

## Fields

| Field | Meaning |
|-------|---------|
| `type` | `page`, `blogpost`, `attachment`, `comment`, or `space` |
| `space` | space key (e.g. `ENG`) |
| `title` | page/content title |
| `text` / `siteSearch` | full-text body search |
| `label` | content label |
| `creator` | original author (username) |
| `contributor` | any contributor (username) |
| `ancestor` | page id — restricts to that page's subtree (descendants) |
| `created` | creation date |
| `lastmodified` | last-modified date |

## Operators

| Operator | Use | Example |
|----------|-----|---------|
| `=` | exact match | `space = "ENG"`, `label = "adr"` |
| `~` | fuzzy / contains (text fields) | `title ~ "roadmap"`, `siteSearch ~ "launch plan"` |
| `in (...)` | any of a set (OR) | `space in ("ENG", "DEV")` |
| `>=` / `<=` | date range bounds | `created >= now("-7d")` |
| `AND` | combine clauses | `type = "page" AND space = "ENG"` |

## Functions

| Function | Meaning |
|----------|---------|
| `currentUser()` | the authenticated account | 
| `now("-7d")` | relative time offset (`-24h`, `-7d`, `-4w`) |
| `startOfDay()` / `startOfWeek()` / `startOfMonth()` | period boundaries |

Example: `creator = currentUser() AND lastmodified >= now("-14d")`.

## Dates

Convenience date flags accept two forms, compiled automatically:

- **Relative**: `-7d`, `-24h`, `-4w`, `-30m` → `now("-7d")`.
- **Absolute**: `2026-01-01` → a quoted date literal.

## Convenience flag → CQL mapping

| Flag | Compiles to |
|------|-------------|
| `--type page` | `type = "page"` |
| `--space ENG,DEV` | `space in ("ENG", "DEV")` |
| `--title roadmap` | `title ~ "roadmap"` |
| `--text "launch plan"` | `siteSearch ~ "launch plan"` |
| `--label adr,design` | `label = "adr" AND label = "design"` (each ANDed) |
| `--creator jdoe` | `creator = "jdoe"` |
| `--contributor jdoe` | `contributor = "jdoe"` |
| `--ancestor 12345` | `ancestor = 12345` (subtree of page 12345) |
| `--created-since -7d` | `created >= now("-7d")` |
| `--created-until 2026-01-01` | `created <= "2026-01-01"` |
| `--modified-since -30d` | `lastmodified >= now("-30d")` |
| `--modified-until 2026-06-01` | `lastmodified <= "2026-06-01"` |

## Sorting and paging

| Flag | Effect |
|------|--------|
| `--sort relevance` | server default relevance order (no ORDER BY emitted) |
| `--sort created` | `order by created` |
| `--sort modified` | `order by lastmodified` |
| `--desc` / `--asc` | sort direction (mutually exclusive) |
| `--limit N` | max results per page |
| `--start-at N` | zero-based offset of the first result |
| `--all` | auto-paginate every result (capped at 1000) |
| `--count-only` | return only `total_size` — cheap magnitude probe |

Each result carries a clickable `url` and an `excerpt` (`_untrusted`). Use the excerpt to judge relevance before fetching full bodies.

## Combining raw CQL with flags

Raw CQL positional args are wrapped in parentheses and ANDed with every flag clause. This lets you express what the flags cannot while keeping the convenience flags:

```bash
confluence-cli search 'label = "adr"' --space ENG --modified-since -30d --compact
# compiles to: (label = "adr") AND space in ("ENG") AND lastmodified >= now("-30d")
```

## Examples

```bash
# Pages in ENG matching a title, newest first.
confluence-cli search --type page --space ENG --title roadmap --sort modified --desc --compact

# My recently-touched content across two spaces.
confluence-cli search --space ENG,DEV --contributor jdoe --modified-since -14d --compact

# Everything under a page subtree that mentions a phrase.
confluence-cli search --ancestor 12345 --text "launch plan" --compact

# How many blogposts in DEV (magnitude probe).
confluence-cli search --type blogpost --space DEV --count-only --compact

# Raw CQL + flags: ADR pages modified in the last month.
confluence-cli search 'label = "adr"' --type page --space ENG --modified-since -30d --compact
```
