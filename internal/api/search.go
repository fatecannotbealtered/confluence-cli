package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Search runs a CQL query with highlighted excerpts.
// GET /rest/api/search?cql=...&excerpt=highlight&start&limit
// Each result's WebURL is assembled client-side (base + webui path) so the
// caller gets a full clickable link.
func (a *SearchAPI) Search(cql string, opts SearchOptions) (*SearchPage, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}
	q := url.Values{}
	q.Set("cql", cql)
	q.Set("excerpt", "highlight")
	q.Set("start", strconv.Itoa(opts.Start))
	q.Set("limit", strconv.Itoa(limit))

	data, err := a.client.get(restPath("/search") + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	var page SearchPage
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("parsing search results: %w", err)
	}

	for i := range page.Results {
		page.Results[i].WebURL = a.client.webURL(&page.Results[i])
	}
	page.HasMore = page.Links.Next != ""
	if page.HasMore {
		page.NextStart = page.Start + len(page.Results)
	}
	return &page, nil
}

// webURL assembles the full clickable URL for one search result.
func (c *Client) webURL(r *SearchResult) string {
	path := r.URL
	if r.Content != nil && r.Content.Links.WebUI != "" {
		path = r.Content.Links.WebUI
	}
	if path == "" {
		return ""
	}
	return c.baseURL + path
}

// SearchUser searches users by CQL user query.
// GET /rest/api/search?cql=user.fullname~"query"
func (a *SearchAPI) SearchUser(query string, opts SearchOptions) (*SearchPage, error) {
	return a.Search(fmt.Sprintf(`user.fullname ~ %q`, query), opts)
}
