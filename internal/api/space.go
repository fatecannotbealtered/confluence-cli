package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// ListSpaces lists spaces, optionally filtered by type ("global"|"personal").
// GET /rest/api/space?type=...
func (a *SpaceAPI) ListSpaces(spaceType string, start, limit int) (*Page[Space], error) {
	params := url.Values{}
	if spaceType != "" {
		params.Set("type", spaceType)
	}
	return getPage[Space](a.client, restPath("/space"), params, start, limit)
}

// GetSpace fetches one space by key.
// GET /rest/api/space/{key}?expand=description.plain
func (a *SpaceAPI) GetSpace(key string) (*Space, error) {
	data, err := a.client.get(restPath("/space/"+url.PathEscape(key)) + "?expand=description.plain")
	if err != nil {
		return nil, err
	}
	var s Space
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing space: %w", err)
	}
	return &s, nil
}

// CreateSpace creates a space.
// POST /rest/api/space
func (a *SpaceAPI) CreateSpace(req *CreateSpaceRequest) (*Space, error) {
	data, err := a.client.post(restPath("/space"), req)
	if err != nil {
		return nil, err
	}
	var s Space
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing created space: %w", err)
	}
	return &s, nil
}

// UpdateSpace updates a space's name/description.
// PUT /rest/api/space/{key}
func (a *SpaceAPI) UpdateSpace(key string, req *UpdateSpaceRequest) (*Space, error) {
	data, err := a.client.put(restPath("/space/"+url.PathEscape(key)), req)
	if err != nil {
		return nil, err
	}
	var s Space
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing updated space: %w", err)
	}
	return &s, nil
}

// DeleteSpace deletes a space asynchronously; the response is a long task
// pointer ({id, links.status}) the caller polls via LongTasks.GetLongTask.
// DELETE /rest/api/space/{key}
func (a *SpaceAPI) DeleteSpace(key string) (*LongTaskLink, error) {
	data, err := a.client.del(restPath("/space/" + url.PathEscape(key)))
	if err != nil {
		return nil, err
	}
	var lt LongTaskLink
	if err := json.Unmarshal(data, &lt); err != nil {
		return nil, fmt.Errorf("parsing space delete long task: %w", err)
	}
	return &lt, nil
}
