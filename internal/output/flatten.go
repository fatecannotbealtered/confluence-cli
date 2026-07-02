package output

import "strings"

// FlatPage is a flattened representation of a Confluence page for
// token-efficient output. Title and excerpt are external content and are
// tagged in _untrusted by the command layer.
type FlatPage struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	SpaceKey  string   `json:"spaceKey"`
	Status    string   `json:"status"`
	Type      string   `json:"type"`
	Version   string   `json:"version,omitempty"`
	Author    string   `json:"author,omitempty"`
	Created   string   `json:"created,omitempty"`
	Updated   string   `json:"updated,omitempty"`
	ParentID  string   `json:"parentId,omitempty"`
	Excerpt   string   `json:"excerpt,omitempty"`
	URL       string   `json:"url,omitempty"`
	Untrusted []string `json:"_untrusted,omitempty"`
}

// FlatSpace is a flattened representation of a Confluence space.
type FlatSpace struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Status      string   `json:"status,omitempty"`
	Description string   `json:"description,omitempty"`
	Untrusted   []string `json:"_untrusted,omitempty"`
}

// PageToMap converts a FlatPage to a map for field filtering.
func PageToMap(p FlatPage) map[string]any {
	m := map[string]any{
		"id":       p.ID,
		"title":    p.Title,
		"spaceKey": p.SpaceKey,
		"status":   p.Status,
		"type":     p.Type,
	}
	if p.Version != "" {
		m["version"] = p.Version
	}
	if p.Author != "" {
		m["author"] = p.Author
	}
	if p.Created != "" {
		m["created"] = p.Created
	}
	if p.Updated != "" {
		m["updated"] = p.Updated
	}
	if p.ParentID != "" {
		m["parentId"] = p.ParentID
	}
	if p.Excerpt != "" {
		m["excerpt"] = p.Excerpt
	}
	if p.URL != "" {
		m["url"] = p.URL
	}
	if len(p.Untrusted) > 0 {
		m["_untrusted"] = p.Untrusted
	}
	return m
}

// pageCanonicalKey maps a normalized (lowercase) page field name to its
// canonical output key (camelCase where applicable, matching FlatPage JSON tags).
func pageCanonicalKey(normalized string) string {
	switch normalized {
	case "spacekey":
		return "spaceKey"
	case "parentid":
		return "parentId"
	default:
		return normalized
	}
}

// FilterPageFields filters a FlatPage to only the specified field names.
// Field names are case-insensitive; output keys use canonical camelCase.
// The _untrusted marker is preserved for any included untrusted field.
func FilterPageFields(p FlatPage, fieldNames []string) map[string]any {
	m := PageToMap(p)
	if len(fieldNames) == 0 {
		return m
	}
	result := make(map[string]any, len(fieldNames))
	included := map[string]bool{}
	for _, name := range fieldNames {
		normalized := strings.TrimSpace(strings.ToLower(name))
		if normalized == "" {
			continue
		}
		key := pageCanonicalKey(normalized)
		if v, ok := m[key]; ok {
			result[key] = v
			included[key] = true
		}
	}
	var untrusted []string
	for _, name := range p.Untrusted {
		key := pageCanonicalKey(strings.TrimSpace(strings.ToLower(name)))
		if included[key] {
			untrusted = append(untrusted, key)
		}
	}
	if len(untrusted) > 0 {
		result["_untrusted"] = untrusted
	}
	return result
}

// SpaceToMap converts a FlatSpace to a map for field filtering.
func SpaceToMap(s FlatSpace) map[string]any {
	m := map[string]any{
		"key":  s.Key,
		"name": s.Name,
		"type": s.Type,
	}
	if s.Status != "" {
		m["status"] = s.Status
	}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if len(s.Untrusted) > 0 {
		m["_untrusted"] = s.Untrusted
	}
	return m
}

// FilterSpaceFields filters a FlatSpace to only the specified field names.
// Field names are case-insensitive. The _untrusted marker is preserved for
// any included untrusted field.
func FilterSpaceFields(s FlatSpace, fieldNames []string) map[string]any {
	m := SpaceToMap(s)
	if len(fieldNames) == 0 {
		return m
	}
	result := make(map[string]any, len(fieldNames))
	included := map[string]bool{}
	for _, name := range fieldNames {
		key := strings.TrimSpace(strings.ToLower(name))
		if key == "" {
			continue
		}
		if v, ok := m[key]; ok {
			result[key] = v
			included[key] = true
		}
	}
	var untrusted []string
	for _, name := range s.Untrusted {
		key := strings.TrimSpace(strings.ToLower(name))
		if included[key] {
			untrusted = append(untrusted, key)
		}
	}
	if len(untrusted) > 0 {
		result["_untrusted"] = untrusted
	}
	return result
}
