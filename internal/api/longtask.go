package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// GetLongTask fetches the status of an async long-running task (e.g. space
// deletion).
// GET /rest/api/longtask/{id}
func (a *LongTaskAPI) GetLongTask(id string) (*LongTask, error) {
	data, err := a.client.get(restPath("/longtask/" + url.PathEscape(id)))
	if err != nil {
		return nil, err
	}
	var lt LongTask
	if err := json.Unmarshal(data, &lt); err != nil {
		return nil, fmt.Errorf("parsing long task: %w", err)
	}
	return &lt, nil
}
