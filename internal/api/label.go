package api

import (
	"net/url"
)

// ListLabels lists labels on a content entity.
// GET /rest/api/content/{id}/label
func (a *LabelAPI) ListLabels(contentID string, start, limit int) (*Page[Label], error) {
	return getPage[Label](a.client, restPath("/content/"+url.PathEscape(contentID)+"/label"), nil, start, limit)
}

// AddLabels adds labels (prefix "global") to a content entity.
// POST /rest/api/content/{id}/label
func (a *LabelAPI) AddLabels(contentID string, names []string) ([]Label, error) {
	body := make([]Label, 0, len(names))
	for _, n := range names {
		body = append(body, Label{Prefix: "global", Name: n})
	}
	data, err := a.client.post(restPath("/content/"+url.PathEscape(contentID)+"/label"), body)
	if err != nil {
		return nil, err
	}
	page, err := parsePage[Label](data)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// RemoveLabel removes one label by name.
// DELETE /rest/api/content/{id}/label?name=...
func (a *LabelAPI) RemoveLabel(contentID, name string) error {
	q := url.Values{}
	q.Set("name", name)
	_, err := a.client.del(restPath("/content/"+url.PathEscape(contentID)+"/label") + "?" + q.Encode())
	return err
}
