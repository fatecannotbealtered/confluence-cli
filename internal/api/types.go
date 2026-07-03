package api

// ===== Common =====

// Links mirrors the DC "_links" object attached to most entities.
type Links struct {
	Base     string `json:"base,omitempty"`
	Context  string `json:"context,omitempty"`
	Self     string `json:"self,omitempty"`
	WebUI    string `json:"webui,omitempty"`
	Download string `json:"download,omitempty"`
	Next     string `json:"next,omitempty"`
}

// ===== Content =====

// Content is a page, blogpost, comment, or attachment entity.
type Content struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Status     string             `json:"status"`
	Title      string             `json:"title"`
	Space      *Space             `json:"space,omitempty"`
	Version    *Version           `json:"version,omitempty"`
	Ancestors  []Content          `json:"ancestors,omitempty"`
	Body       *Body              `json:"body,omitempty"`
	History    *ContentHistory    `json:"history,omitempty"`
	Extensions *ContentExtensions `json:"extensions,omitempty"`
	Links      Links              `json:"_links"`
}

// Version is the content version block (optimistic-lock anchor).
type Version struct {
	By        *User  `json:"by,omitempty"`
	When      string `json:"when,omitempty"`
	Number    int    `json:"number"`
	Message   string `json:"message,omitempty"`
	MinorEdit bool   `json:"minorEdit,omitempty"`
}

// Body carries the content body representations.
type Body struct {
	Storage *BodyRepresentation `json:"storage,omitempty"`
	View    *BodyRepresentation `json:"view,omitempty"`
}

// BodyRepresentation is one body format, e.g. {value, representation:"storage"}.
type BodyRepresentation struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
}

// ContentHistory mirrors GET /content/{id}/history.
type ContentHistory struct {
	Latest      bool     `json:"latest"`
	CreatedBy   *User    `json:"createdBy,omitempty"`
	CreatedDate string   `json:"createdDate,omitempty"`
	LastUpdated *Version `json:"lastUpdated,omitempty"`
}

// ContentExtensions carries type-specific extension fields (attachment media
// type/size, comment location/resolution).
type ContentExtensions struct {
	// Attachment fields
	MediaType string `json:"mediaType,omitempty"`
	FileSize  int64  `json:"fileSize,omitempty"`
	Comment   string `json:"comment,omitempty"`
	// Comment fields
	Location   string      `json:"location,omitempty"`
	Resolution *Resolution `json:"resolution,omitempty"`
}

// Resolution is the inline-comment resolution status.
type Resolution struct {
	Status string `json:"status"`
}

// ContentRef references another content entity by ID (ancestors, reply-to).
type ContentRef struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
}

// SpaceKeyRef references a space by key in request bodies.
type SpaceKeyRef struct {
	Key string `json:"key"`
}

// VersionRef carries the version number for optimistic-lock updates.
type VersionRef struct {
	Number int `json:"number"`
}

// RequestBody is the body block for create/update requests.
type RequestBody struct {
	Storage BodyRepresentation `json:"storage"`
}

// CreateContentRequest creates a page or blogpost.
type CreateContentRequest struct {
	Type      string       `json:"type"` // "page" | "blogpost"
	Title     string       `json:"title"`
	Space     SpaceKeyRef  `json:"space"`
	Body      RequestBody  `json:"body"`
	Ancestors []ContentRef `json:"ancestors,omitempty"`
}

// UpdateContentRequest updates content with version.number optimistic lock.
// Ancestors, when set, reparents the content (used by page move).
type UpdateContentRequest struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Version   VersionRef   `json:"version"`
	Body      *RequestBody `json:"body,omitempty"`
	Ancestors []ContentRef `json:"ancestors,omitempty"`
}

// createCommentRequest creates a footer comment (optionally as a reply).
type createCommentRequest struct {
	Type      string       `json:"type"` // "comment"
	Container ContentRef   `json:"container"`
	Ancestors []ContentRef `json:"ancestors,omitempty"`
	Body      RequestBody  `json:"body"`
}

// ===== Search =====

// SearchResult is one CQL search hit from GET /rest/api/search.
type SearchResult struct {
	Content              *Content `json:"content,omitempty"`
	User                 *User    `json:"user,omitempty"`
	Title                string   `json:"title"`
	Excerpt              string   `json:"excerpt"`
	URL                  string   `json:"url"`
	EntityType           string   `json:"entityType"`
	LastModified         string   `json:"lastModified"`
	FriendlyLastModified string   `json:"friendlyLastModified"`
	// WebURL is assembled client-side: base URL + content._links.webui
	// (falls back to the result's url path). Full clickable link.
	WebURL string `json:"-"`
}

// SearchPage is the search response with offset pagination metadata.
type SearchPage struct {
	Results   []SearchResult `json:"results"`
	Start     int            `json:"start"`
	Limit     int            `json:"limit"`
	Size      int            `json:"size"`
	TotalSize int            `json:"totalSize"`
	CQLQuery  string         `json:"cqlQuery"`
	Links     Links          `json:"_links"`
	// NextStart is the start offset for the next page; meaningful only when
	// HasMore. Both are derived client-side.
	NextStart int  `json:"-"`
	HasMore   bool `json:"-"`
}

// SearchOptions tunes a CQL search.
type SearchOptions struct {
	Start int
	Limit int
}

// ===== Space =====

// Space is a Confluence space.
type Space struct {
	ID          int64             `json:"id,omitempty"`
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Type        string            `json:"type,omitempty"` // "global" | "personal"
	Status      string            `json:"status,omitempty"`
	Description *SpaceDescription `json:"description,omitempty"`
	Links       Links             `json:"_links"`
}

// SpaceDescription wraps the plain-text space description.
type SpaceDescription struct {
	Plain BodyRepresentation `json:"plain"`
}

// CreateSpaceRequest creates a space.
type CreateSpaceRequest struct {
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description *SpaceDescription `json:"description,omitempty"`
}

// UpdateSpaceRequest updates a space's name/description.
type UpdateSpaceRequest struct {
	Name        string            `json:"name,omitempty"`
	Description *SpaceDescription `json:"description,omitempty"`
}

// LongTaskLink is the async pointer returned by DELETE /space/{key}.
type LongTaskLink struct {
	ID    string `json:"id"`
	Links struct {
		Status string `json:"status"`
	} `json:"links"`
}

// ===== Label =====

// Label is a content label.
type Label struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	ID     string `json:"id,omitempty"`
	Label  string `json:"label,omitempty"`
}

// ===== User =====

// User is a Confluence DC user.
type User struct {
	Type        string `json:"type,omitempty"`
	Username    string `json:"username"`
	UserKey     string `json:"userKey,omitempty"`
	DisplayName string `json:"displayName"`
	Links       Links  `json:"_links"`
}

// ===== LongTask =====

// LongTask mirrors GET /rest/api/longtask/{id}.
type LongTask struct {
	ID                 string            `json:"id"`
	Name               *LongTaskName     `json:"name,omitempty"`
	ElapsedTime        int64             `json:"elapsedTime"`
	PercentageComplete int               `json:"percentageComplete"`
	Successful         bool              `json:"successful"`
	Finished           bool              `json:"finished"`
	Messages           []LongTaskMessage `json:"messages,omitempty"`
}

// LongTaskName is the i18n key of a long task.
type LongTaskName struct {
	Key string `json:"key"`
}

// LongTaskMessage is one translated progress/result message.
type LongTaskMessage struct {
	Translation string `json:"translation"`
}

// ===== System =====

// SystemInfo mirrors GET /rest/api/settings/systemInfo (fields vary by DC
// version; unknown fields are ignored).
type SystemInfo struct {
	BaseURL     string `json:"baseUrl,omitempty"`
	Version     string `json:"version,omitempty"`
	BuildNumber string `json:"buildNumber,omitempty"`
	CloudID     string `json:"cloudId,omitempty"`
	CommitHash  string `json:"commitHash,omitempty"`
}
