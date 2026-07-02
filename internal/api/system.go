package api

import (
	"encoding/json"
	"fmt"
)

// SystemInfo fetches Confluence system information (used by doctor).
// GET /rest/api/settings/systemInfo
func (a *SystemAPI) SystemInfo() (*SystemInfo, error) {
	data, err := a.client.get(restPath("/settings/systemInfo"))
	if err != nil {
		return nil, err
	}
	var si SystemInfo
	if err := json.Unmarshal(data, &si); err != nil {
		return nil, fmt.Errorf("parsing system info: %w", err)
	}
	return &si, nil
}
