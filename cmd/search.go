package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	searchSpaces        []string
	searchType          string
	searchTitle         string
	searchText          string
	searchLabels        []string
	searchCreator       string
	searchContributor   string
	searchAncestor      string
	searchCreatedSince  string
	searchCreatedUntil  string
	searchModifiedSince string
	searchModifiedUntil string
	searchSort          string
	searchDesc          bool
	searchAsc           bool
	searchCountOnly     bool
	searchAll           bool
	searchLimit         int
	searchStart         int
)

// searchAllCap bounds --all auto-pagination so a huge result set cannot run away.
const searchAllCap = 1000

var searchCmd = &cobra.Command{
	Use:   "search [CQL...]",
	Short: "Search Confluence with CQL and/or convenience flags",
	Long: `Search Confluence content. Positional arguments are treated as raw CQL and
combined (AND) with the convenience flags. Dates accept relative (-7d, -24h)
or absolute (2026-01-01) forms.`,
	Args: cobra.ArbitraryArgs,
	RunE: runSearch,
}

func init() {
	f := searchCmd.Flags()
	f.StringSliceVar(&searchSpaces, "space", nil, "Restrict to space keys (comma-separated, OR)")
	f.StringVar(&searchType, "type", "", "Content type: page, blogpost, attachment, comment, or space")
	f.StringVar(&searchTitle, "title", "", "Match title (title ~)")
	f.StringVar(&searchText, "text", "", "Full-text match (siteSearch ~)")
	f.StringSliceVar(&searchLabels, "label", nil, "Require labels (comma-separated, AND)")
	f.StringVar(&searchCreator, "creator", "", "Filter by creator username")
	f.StringVar(&searchContributor, "contributor", "", "Filter by contributor username")
	f.StringVar(&searchAncestor, "ancestor", "", "Restrict to descendants of a page id")
	f.StringVar(&searchCreatedSince, "created-since", "", "Created on/after (relative -7d or absolute 2026-01-01)")
	f.StringVar(&searchCreatedUntil, "created-until", "", "Created on/before (relative or absolute)")
	f.StringVar(&searchModifiedSince, "modified-since", "", "Modified on/after (relative or absolute)")
	f.StringVar(&searchModifiedUntil, "modified-until", "", "Modified on/before (relative or absolute)")
	f.StringVar(&searchSort, "sort", "", "Sort by relevance, created, or modified")
	f.BoolVar(&searchDesc, "desc", false, "Sort descending")
	f.BoolVar(&searchAsc, "asc", false, "Sort ascending")
	f.BoolVar(&searchCountOnly, "count-only", false, "Return only the total match count")
	f.BoolVar(&searchAll, "all", false, "Auto-paginate all results (capped at 1000)")
	f.IntVar(&searchLimit, "limit", 0, "Maximum number of results per page")
	f.IntVar(&searchStart, "start-at", 0, "Zero-based offset of the first result")
	rootCmd.AddCommand(searchCmd)
}

// searchFilters holds the raw flag inputs the CQL compiler consumes. Keeping it
// a plain struct lets buildCQL be pure and unit-testable without cobra state.
type searchFilters struct {
	raw           []string
	spaces        []string
	contentType   string
	title         string
	text          string
	labels        []string
	creator       string
	contributor   string
	ancestor      string
	createdSince  string
	createdUntil  string
	modifiedSince string
	modifiedUntil string
	sort          string
	desc          bool
	asc           bool
}

var validSearchTypes = map[string]bool{
	"page": true, "blogpost": true, "attachment": true, "comment": true, "space": true,
}

var relativeDateRe = regexp.MustCompile(`^-\d+[dhmw]$`)
var absoluteDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// compileDate converts a relative (-7d) or absolute (2026-01-01) token into a
// CQL-valid date expression. Relative becomes now("-7d"); absolute stays a
// quoted literal. Anything else is a usage error.
func compileDate(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case relativeDateRe.MatchString(v):
		return fmt.Sprintf(`now(%q)`, v), nil
	case absoluteDateRe.MatchString(v):
		return fmt.Sprintf("%q", v), nil
	default:
		return "", fmt.Errorf("invalid date %q: use relative (-7d, -24h) or absolute (2026-01-01)", v)
	}
}

// cqlQuote quotes a CQL string literal.
func cqlQuote(s string) string { return fmt.Sprintf("%q", s) }

// buildCQL compiles the raw CQL fragments and convenience filters into a single
// CQL string, ANDing every clause and appending an ORDER BY when a sort is set.
func buildCQL(fl searchFilters) (string, error) {
	var clauses []string

	for _, r := range fl.raw {
		if s := strings.TrimSpace(r); s != "" {
			clauses = append(clauses, "("+s+")")
		}
	}

	if fl.contentType != "" {
		if !validSearchTypes[fl.contentType] {
			return "", fmt.Errorf("invalid --type %q: use page, blogpost, attachment, comment, or space", fl.contentType)
		}
		clauses = append(clauses, "type = "+cqlQuote(fl.contentType))
	}

	if len(fl.spaces) > 0 {
		quoted := make([]string, 0, len(fl.spaces))
		for _, s := range fl.spaces {
			if s = strings.TrimSpace(s); s != "" {
				quoted = append(quoted, cqlQuote(s))
			}
		}
		if len(quoted) > 0 {
			clauses = append(clauses, "space in ("+strings.Join(quoted, ", ")+")")
		}
	}

	if fl.title != "" {
		clauses = append(clauses, "title ~ "+cqlQuote(fl.title))
	}
	if fl.text != "" {
		clauses = append(clauses, "siteSearch ~ "+cqlQuote(fl.text))
	}
	for _, lbl := range fl.labels {
		if lbl = strings.TrimSpace(lbl); lbl != "" {
			clauses = append(clauses, "label = "+cqlQuote(lbl))
		}
	}
	if fl.creator != "" {
		clauses = append(clauses, "creator = "+cqlQuote(fl.creator))
	}
	if fl.contributor != "" {
		clauses = append(clauses, "contributor = "+cqlQuote(fl.contributor))
	}
	if fl.ancestor != "" {
		clauses = append(clauses, "ancestor = "+cqlQuote(fl.ancestor))
	}

	dateClauses := []struct {
		field, value, op string
	}{
		{"created", fl.createdSince, ">="},
		{"created", fl.createdUntil, "<="},
		{"lastmodified", fl.modifiedSince, ">="},
		{"lastmodified", fl.modifiedUntil, "<="},
	}
	for _, dc := range dateClauses {
		if dc.value == "" {
			continue
		}
		expr, err := compileDate(dc.value)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, fmt.Sprintf("%s %s %s", dc.field, dc.op, expr))
	}

	if len(clauses) == 0 {
		return "", fmt.Errorf("empty query: provide raw CQL or at least one filter flag")
	}

	cql := strings.Join(clauses, " AND ")

	if order, err := buildOrderBy(fl); err != nil {
		return "", err
	} else if order != "" {
		cql += " " + order
	}
	return cql, nil
}

func buildOrderBy(fl searchFilters) (string, error) {
	if fl.sort == "" {
		return "", nil
	}
	if fl.desc && fl.asc {
		return "", fmt.Errorf("--desc and --asc are mutually exclusive")
	}
	var field string
	switch fl.sort {
	case "relevance":
		return "", nil // server default ordering
	case "created":
		field = "created"
	case "modified":
		field = "lastmodified"
	default:
		return "", fmt.Errorf("invalid --sort %q: use relevance, created, or modified", fl.sort)
	}
	dir := "desc"
	if fl.asc {
		dir = "asc"
	}
	return "order by " + field + " " + dir, nil
}

func runSearch(_ *cobra.Command, args []string) error {
	cql, err := buildCQL(searchFilters{
		raw:           args,
		spaces:        searchSpaces,
		contentType:   searchType,
		title:         searchTitle,
		text:          searchText,
		labels:        searchLabels,
		creator:       searchCreator,
		contributor:   searchContributor,
		ancestor:      searchAncestor,
		createdSince:  searchCreatedSince,
		createdUntil:  searchCreatedUntil,
		modifiedSince: searchModifiedSince,
		modifiedUntil: searchModifiedUntil,
		sort:          searchSort,
		desc:          searchDesc,
		asc:           searchAsc,
	})
	if err != nil {
		emitError(output.ErrValidation, err.Error(), nil)
		return SilentErr(ExitBadArgs)
	}

	client, err := newClient()
	if err != nil {
		return err
	}

	if searchCountOnly {
		page, err := client.Search.Search(cql, api.SearchOptions{Start: 0, Limit: 1})
		if err != nil {
			return emitAPIError(err)
		}
		output.PrintJSON(map[string]any{"total_size": page.TotalSize})
		return nil
	}

	if searchAll {
		return runSearchAll(client, cql)
	}

	page, err := client.Search.Search(cql, api.SearchOptions{Start: searchStart, Limit: searchLimit})
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(map[string]any{
		"results":       searchResultMaps(page.Results),
		"start_at":      page.Start,
		"size":          page.Size,
		"total_size":    page.TotalSize,
		"has_more":      page.HasMore,
		"next_start_at": page.NextStart,
	})
	return nil
}

func runSearchAll(client *api.Client, cql string) error {
	var all []api.SearchResult
	start := 0
	capped := false
	for {
		page, err := client.Search.Search(cql, api.SearchOptions{Start: start, Limit: 0})
		if err != nil {
			return emitAPIError(err)
		}
		all = append(all, page.Results...)
		if len(all) >= searchAllCap {
			all = all[:searchAllCap]
			capped = true
			break
		}
		if !page.HasMore || len(page.Results) == 0 {
			break
		}
		start = page.NextStart
	}
	if capped {
		output.AddNotice(fmt.Sprintf("result set truncated at the --all cap of %d", searchAllCap))
	}
	output.PrintJSON(map[string]any{
		"results":    searchResultMaps(all),
		"size":       len(all),
		"total_size": len(all),
		"has_more":   false,
	})
	return nil
}

// searchResultMaps projects search hits to the per-result output shape.
func searchResultMaps(results []api.SearchResult) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for i := range results {
		r := &results[i]
		id, ctype, spaceKey := "", r.EntityType, ""
		if r.Content != nil {
			id = r.Content.ID
			if r.Content.Type != "" {
				ctype = r.Content.Type
			}
			if r.Content.Space != nil {
				spaceKey = r.Content.Space.Key
			}
		}
		out = append(out, map[string]any{
			"id":            id,
			"type":          ctype,
			"title":         r.Title,
			"space_key":     spaceKey,
			"url":           r.WebURL,
			"excerpt":       cleanExcerpt(r.Excerpt),
			"last_modified": r.LastModified,
			"_untrusted":    []string{"title", "excerpt"},
		})
	}
	return out
}

// excerptHighlight strips Confluence's raw match-highlight delimiters
// (@@@hl@@@term@@@endhl@@@) that the server wraps around matched terms in
// highlighted excerpts. Agents want clean prose to judge relevance; the raw
// markers are noise. The matched text itself is preserved.
var excerptHighlight = strings.NewReplacer("@@@hl@@@", "", "@@@endhl@@@", "")

func cleanExcerpt(s string) string {
	return excerptHighlight.Replace(s)
}
