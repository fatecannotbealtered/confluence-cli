package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// CurrentUser fetches the authenticated user.
// GET /rest/api/user/current
func (a *UserAPI) CurrentUser() (*User, error) {
	data, err := a.client.get(restPath("/user/current"))
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("parsing current user: %w", err)
	}
	return &u, nil
}

// GetUser fetches a user by username.
// GET /rest/api/user?username=...
func (a *UserAPI) GetUser(username string) (*User, error) {
	q := url.Values{}
	q.Set("username", username)
	data, err := a.client.get(restPath("/user") + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("parsing user: %w", err)
	}
	return &u, nil
}
