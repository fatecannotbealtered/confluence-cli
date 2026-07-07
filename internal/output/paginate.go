package output

// PagedMap builds a list command's canonical offset-style pagination body:
// {items, count, offset, next_offset, has_more}. These key names are the fleet
// contract (contract.json pagination.offset_style); every list command routes
// through here so the keys can never drift per-command. Callers attach extras
// (total_size when the server reports a grand total, truncated:true when a
// client-side cap hides results) on the returned map.
func PagedMap(items any, count, offset, nextOffset int, hasMore bool) map[string]any {
	return map[string]any{
		"items":       items,
		"count":       count,
		"offset":      offset,
		"next_offset": nextOffset,
		"has_more":    hasMore,
	}
}
