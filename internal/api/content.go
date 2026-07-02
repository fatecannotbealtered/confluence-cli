package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func joinComma(parts []string) string { return strings.Join(parts, ",") }

func parseContent(data []byte) (*Content, error) {
	var c Content
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing content: %w", err)
	}
	return &c, nil
}

// GetContentOptions tunes GetContent.
type GetContentOptions struct {
	Expand []string
	// Status + Version read a historical version:
	// status=historical&version=N.
	Status  string
	Version int
}

// GetContent fetches one content entity by ID.
// GET /rest/api/content/{id}?expand=...
func (a *ContentAPI) GetContent(id string, opts GetContentOptions) (*Content, error) {
	q := url.Values{}
	if len(opts.Expand) > 0 {
		q.Set("expand", joinComma(opts.Expand))
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.Version > 0 {
		q.Set("version", strconv.Itoa(opts.Version))
	}
	path := restPath("/content/" + url.PathEscape(id))
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	data, err := a.client.get(path)
	if err != nil {
		return nil, err
	}
	var c Content
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing content: %w", err)
	}
	return &c, nil
}

// GetContentBySpaceTitle resolves a page by exact space key + title via CQL.
// Returns E_NOT_FOUND when no page matches.
func (a *ContentAPI) GetContentBySpaceTitle(space, title string, expand []string) (*Content, error) {
	cql := fmt.Sprintf(`space = %q AND title = %q AND type = page`, space, title)
	q := url.Values{}
	q.Set("cql", cql)
	q.Set("limit", "1")
	if len(expand) > 0 {
		q.Set("expand", joinComma(expand))
	}
	data, err := a.client.get(restPath("/content/search") + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	page, err := parsePage[Content](data)
	if err != nil {
		return nil, err
	}
	if len(page.Items) == 0 {
		return nil, notFoundError(fmt.Sprintf("no page titled %q in space %q", title, space))
	}
	return &page.Items[0], nil
}

// CreateContent creates a page or blogpost.
// POST /rest/api/content
func (a *ContentAPI) CreateContent(req *CreateContentRequest) (*Content, error) {
	data, err := a.client.post(restPath("/content"), req)
	if err != nil {
		return nil, err
	}
	var c Content
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing created content: %w", err)
	}
	return &c, nil
}

// UpdateContent updates content; req.Version.Number is the optimistic lock
// (must be current version + 1, otherwise DC returns 409).
// PUT /rest/api/content/{id}
func (a *ContentAPI) UpdateContent(id string, req *UpdateContentRequest) (*Content, error) {
	data, err := a.client.put(restPath("/content/"+url.PathEscape(id)), req)
	if err != nil {
		return nil, err
	}
	var c Content
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing updated content: %w", err)
	}
	return &c, nil
}

// DeleteContent moves content to trash. With purge, a second DELETE with
// status=trashed permanently removes it.
// DELETE /rest/api/content/{id}[?status=trashed]
func (a *ContentAPI) DeleteContent(id string, purge bool) error {
	base := restPath("/content/" + url.PathEscape(id))
	if _, err := a.client.del(base); err != nil {
		return err
	}
	if purge {
		if _, err := a.client.del(base + "?status=trashed"); err != nil {
			return err
		}
	}
	return nil
}

// ChildPages lists direct child pages.
// GET /rest/api/content/{id}/child/page
func (a *ContentAPI) ChildPages(id string, start, limit int) (*Page[Content], error) {
	return getPage[Content](a.client, restPath("/content/"+url.PathEscape(id)+"/child/page"), nil, start, limit)
}

// DescendantPages lists all descendant pages (every depth).
// GET /rest/api/content/{id}/descendant/page
func (a *ContentAPI) DescendantPages(id string, start, limit int) (*Page[Content], error) {
	return getPage[Content](a.client, restPath("/content/"+url.PathEscape(id)+"/descendant/page"), nil, start, limit)
}

// Ancestors returns the ancestor chain (root first) via expand=ancestors.
func (a *ContentAPI) Ancestors(id string) ([]Content, error) {
	c, err := a.GetContent(id, GetContentOptions{Expand: []string{"ancestors"}})
	if err != nil {
		return nil, err
	}
	return c.Ancestors, nil
}

// ContentHistory fetches the content history block.
// GET /rest/api/content/{id}/history
func (a *ContentAPI) ContentHistory(id string) (*ContentHistory, error) {
	data, err := a.client.get(restPath("/content/" + url.PathEscape(id) + "/history"))
	if err != nil {
		return nil, err
	}
	var h ContentHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("parsing content history: %w", err)
	}
	return &h, nil
}
