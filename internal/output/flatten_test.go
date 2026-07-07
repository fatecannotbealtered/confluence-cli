package output

import (
	"reflect"
	"testing"
)

func TestPageToMap_AllFields(t *testing.T) {
	p := FlatPage{
		ID:        "12345",
		Title:     "Design doc",
		SpaceKey:  "ENG",
		Status:    "current",
		Type:      "page",
		Version:   "7",
		Author:    "alice",
		Created:   "2026-01-01T00:00:00Z",
		Updated:   "2026-01-02T00:00:00Z",
		ParentID:  "100",
		Excerpt:   "intro",
		URL:       "https://confluence.example.com/pages/12345",
		Untrusted: []string{"title", "excerpt"},
	}

	got := PageToMap(p)
	want := map[string]any{
		"id":         "12345",
		"title":      "Design doc",
		"space_key":  "ENG",
		"status":     "current",
		"type":       "page",
		"version":    "7",
		"author":     "alice",
		"created":    "2026-01-01T00:00:00Z",
		"updated":    "2026-01-02T00:00:00Z",
		"parent_id":  "100",
		"excerpt":    "intro",
		"url":        "https://confluence.example.com/pages/12345",
		"_untrusted": []string{"title", "excerpt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PageToMap() = %v, want %v", got, want)
	}
}

func TestPageToMap_RequiredOnly(t *testing.T) {
	p := FlatPage{ID: "1", Title: "T", SpaceKey: "S", Status: "current", Type: "page"}
	got := PageToMap(p)
	if len(got) != 5 {
		t.Fatalf("expected 5 fields, got %d: %v", len(got), got)
	}
}

func TestFilterPageFields(t *testing.T) {
	p := FlatPage{
		ID:       "12345",
		Title:    "Test",
		SpaceKey: "ENG",
		Status:   "current",
		Type:     "page",
		Author:   "alice",
	}

	t.Run("empty field list returns all", func(t *testing.T) {
		got := FilterPageFields(p, nil)
		if len(got) != 6 {
			t.Fatalf("expected all 6 fields, got %d: %v", len(got), got)
		}
	})

	t.Run("filters requested fields case-insensitively", func(t *testing.T) {
		got := FilterPageFields(p, []string{" ID ", "Title", "missing", "AUTHOR"})
		if len(got) != 3 {
			t.Fatalf("expected 3 fields, got %d: %v", len(got), got)
		}
		if got["id"] != "12345" || got["title"] != "Test" || got["author"] != "alice" {
			t.Errorf("unexpected filtered map: %v", got)
		}
	})

	t.Run("snake_case keys resolve case-insensitively", func(t *testing.T) {
		p2 := p
		p2.ParentID = "100"
		for _, fields := range [][]string{
			{"spacekey", "parentid"},
			{"SPACEKEY", "PARENTID"},
			{"spaceKey", "parentId"},
		} {
			got := FilterPageFields(p2, fields)
			if got["space_key"] != "ENG" || got["parent_id"] != "100" {
				t.Errorf("fields %v: got %v, want canonical snake_case keys", fields, got)
			}
		}
	})

	t.Run("skips empty names", func(t *testing.T) {
		got := FilterPageFields(p, []string{"", "  ", "title"})
		if len(got) != 1 || got["title"] != "Test" {
			t.Errorf("expected only title, got %v", got)
		}
	})
}

func TestFilterPageFields_UntrustedPropagation(t *testing.T) {
	p := FlatPage{
		ID:        "1",
		Title:     "Injected <script>",
		SpaceKey:  "ENG",
		Status:    "current",
		Type:      "page",
		Excerpt:   "sneaky",
		Untrusted: []string{"title", "excerpt"},
	}

	t.Run("kept for included untrusted fields", func(t *testing.T) {
		got := FilterPageFields(p, []string{"id", "title"})
		untrusted, ok := got["_untrusted"].([]string)
		if !ok || len(untrusted) != 1 || untrusted[0] != "title" {
			t.Errorf("_untrusted = %v, want [title]", got["_untrusted"])
		}
	})

	t.Run("dropped when no untrusted field included", func(t *testing.T) {
		got := FilterPageFields(p, []string{"id", "status"})
		if _, ok := got["_untrusted"]; ok {
			t.Errorf("_untrusted should be absent, got %v", got["_untrusted"])
		}
	})
}

func TestSpaceToMap_AllFields(t *testing.T) {
	s := FlatSpace{
		Key:         "ENG",
		Name:        "Engineering",
		Type:        "global",
		Status:      "current",
		Description: "Team space",
		Untrusted:   []string{"name", "description"},
	}
	got := SpaceToMap(s)
	if len(got) != 6 {
		t.Fatalf("expected 6 fields, got %d: %v", len(got), got)
	}
}

func TestSpaceToMap_RequiredOnly(t *testing.T) {
	s := FlatSpace{Key: "K", Name: "N", Type: "global"}
	got := SpaceToMap(s)
	if len(got) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(got), got)
	}
}

func TestFilterSpaceFields(t *testing.T) {
	s := FlatSpace{
		Key:         "ENG",
		Name:        "Engineering",
		Type:        "global",
		Status:      "current",
		Description: "Team space",
	}

	t.Run("empty field list returns all", func(t *testing.T) {
		got := FilterSpaceFields(s, nil)
		if len(got) != 5 {
			t.Fatalf("expected 5 fields, got %d: %v", len(got), got)
		}
	})

	t.Run("filters case-insensitively", func(t *testing.T) {
		got := FilterSpaceFields(s, []string{"KEY", " Name ", "missing"})
		if len(got) != 2 {
			t.Fatalf("expected 2 fields, got %d: %v", len(got), got)
		}
		if got["key"] != "ENG" || got["name"] != "Engineering" {
			t.Errorf("unexpected filtered map: %v", got)
		}
	})

	t.Run("skips empty and unknown", func(t *testing.T) {
		got := FilterSpaceFields(s, []string{"", "  ", "type", "nope"})
		if len(got) != 1 || got["type"] != "global" {
			t.Errorf("expected only type, got %v", got)
		}
	})
}

func TestFilterSpaceFields_UntrustedPropagation(t *testing.T) {
	s := FlatSpace{
		Key:       "ENG",
		Name:      "Injected name",
		Type:      "global",
		Untrusted: []string{"name"},
	}

	got := FilterSpaceFields(s, []string{"key", "name"})
	untrusted, ok := got["_untrusted"].([]string)
	if !ok || len(untrusted) != 1 || untrusted[0] != "name" {
		t.Errorf("_untrusted = %v, want [name]", got["_untrusted"])
	}

	got = FilterSpaceFields(s, []string{"key"})
	if _, ok := got["_untrusted"]; ok {
		t.Errorf("_untrusted should be absent, got %v", got["_untrusted"])
	}
}

func TestPageCanonicalKey(t *testing.T) {
	if pageCanonicalKey("spacekey") != "space_key" {
		t.Error("spacekey should map to space_key")
	}
	if pageCanonicalKey("parentid") != "parent_id" {
		t.Error("parentid should map to parent_id")
	}
	if pageCanonicalKey("title") != "title" {
		t.Error("title should pass through unchanged")
	}
}
