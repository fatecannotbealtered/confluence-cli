package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

const defaultPageLimit = 25

// pageMeta mirrors the DC offset-pagination envelope:
// {results, start, limit, size, _links.next}.
type pageMeta struct {
	Start int   `json:"start"`
	Limit int   `json:"limit"`
	Size  int   `json:"size"`
	Links Links `json:"_links"`
}

// Page is the unified pagination result: items plus deterministic paging
// state for the caller (next_start_at + has_more).
type Page[T any] struct {
	Items       []T
	Start       int
	Limit       int
	Size        int
	NextStartAt int // meaningful only when HasMore
	HasMore     bool
}

// pageHasMore decides whether another page exists. DC sets _links.next only
// when a further page exists, so it is the authoritative signal.
func pageHasMore(meta pageMeta) bool {
	return meta.Links.Next != ""
}

// buildPagePath appends start/limit to basePath, preserving existing params.
func buildPagePath(basePath string, params url.Values, start, limit int) string {
	q := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("start", strconv.Itoa(start))
	q.Set("limit", strconv.Itoa(limit))
	return basePath + "?" + q.Encode()
}

// getPage fetches one page of results from a DC array endpoint and returns
// the unified Page shape.
func getPage[T any](c *Client, basePath string, params url.Values, start, limit int) (*Page[T], error) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	data, err := c.get(buildPagePath(basePath, params, start, limit))
	if err != nil {
		return nil, err
	}
	return parsePage[T](data)
}

// parsePage decodes a DC paged body {results,start,limit,size,_links} into a
// Page.
func parsePage[T any](data []byte) (*Page[T], error) {
	var body struct {
		pageMeta
		Results []T `json:"results"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("parsing paged response: %w", err)
	}
	p := &Page[T]{
		Items:   body.Results,
		Start:   body.Start,
		Limit:   body.Limit,
		Size:    body.Size,
		HasMore: pageHasMore(body.pageMeta),
	}
	if p.HasMore {
		p.NextStartAt = body.Start + len(body.Results)
	}
	return p, nil
}

// getAllPages walks every page of a DC array endpoint and returns all items.
func getAllPages[T any](c *Client, basePath string, params url.Values) ([]T, error) {
	var all []T
	start := 0
	for {
		page, err := getPage[T](c, basePath, params, start, defaultPageLimit)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if !page.HasMore || len(page.Items) == 0 {
			return all, nil
		}
		start = page.NextStartAt
	}
}
