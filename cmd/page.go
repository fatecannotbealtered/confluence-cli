package cmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/fatecannotbealtered/confluence-cli/internal/render"
	"github.com/spf13/cobra"
)

var (
	pageGetBodyFormat  string
	pageGetSpace       string
	pageGetTitle       string
	pageListSpace      string
	pageListLimit      int
	pageListStart      int
	pageCreateSpace    string
	pageCreateTitle    string
	pageCreateBody     string
	pageCreateBodyFile string
	pageCreateFormat   string
	pageCreateParent   string
	pageCreateType     string
	pageUpdateTitle    string
	pageUpdateBody     string
	pageUpdateBodyFile string
	pageUpdateFormat   string
	pageDeletePurge    bool
	pageMoveParent     string
	pageChildrenLimit  int
	pageChildrenStart  int
	pageDescLimit      int
	pageDescStart      int
	pageRestoreVersion int
)

var pageCmd = &cobra.Command{
	Use:   "page",
	Short: "Manage Confluence pages, comments, attachments, and labels",
}

var pageGetCmd = &cobra.Command{
	Use:   "get [ID]",
	Short: "Get a page by ID, or by --space + --title",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPageGet,
}

var pageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pages in a space",
	Args:  cobra.NoArgs,
	RunE:  runPageList,
}

var pageCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a page or blogpost",
	Args:  cobra.NoArgs,
	RunE:  runPageCreate,
}

var pageUpdateCmd = &cobra.Command{
	Use:   "update <ID>",
	Short: "Update a page (title and/or body) with optimistic locking",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageUpdate,
}

var pageDeleteCmd = &cobra.Command{
	Use:   "delete <ID>",
	Short: "Delete a page (write-dangerous; --purge for permanent removal)",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageDelete,
}

var pageMoveCmd = &cobra.Command{
	Use:   "move <ID>",
	Short: "Move a page under a new parent",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageMove,
}

var pageChildrenCmd = &cobra.Command{
	Use:   "children <ID>",
	Short: "List direct child pages",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageChildren,
}

var pageDescendantsCmd = &cobra.Command{
	Use:   "descendants <ID>",
	Short: "List all descendant pages",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageDescendants,
}

var pageAncestorsCmd = &cobra.Command{
	Use:   "ancestors <ID>",
	Short: "Show the ancestor breadcrumb chain",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageAncestors,
}

var pageHistoryCmd = &cobra.Command{
	Use:   "history <ID>",
	Short: "Show a page's version history",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageHistory,
}

var pageRestoreCmd = &cobra.Command{
	Use:   "restore <ID>",
	Short: "Restore a page to a previous version",
	Args:  cobra.ExactArgs(1),
	RunE:  runPageRestore,
}

func init() {
	pageGetCmd.Flags().StringVar(&pageGetBodyFormat, "body-format", "markdown", "Body format: markdown, storage, or view")
	pageGetCmd.Flags().StringVar(&pageGetSpace, "space", "", "Locate by space key (with --title, mutually exclusive with ID)")
	pageGetCmd.Flags().StringVar(&pageGetTitle, "title", "", "Locate by exact title (with --space)")

	pageListCmd.Flags().StringVar(&pageListSpace, "space", "", "Space key (required)")
	pageListCmd.Flags().IntVar(&pageListLimit, "limit", 0, "Maximum number of pages to return")
	pageListCmd.Flags().IntVar(&pageListStart, "start-at", 0, "Zero-based offset of the first result")

	pageCreateCmd.Flags().StringVar(&pageCreateSpace, "space", "", "Space key (required)")
	pageCreateCmd.Flags().StringVar(&pageCreateTitle, "title", "", "Page title (required)")
	pageCreateCmd.Flags().StringVar(&pageCreateBody, "body", "", "Body content (mutually exclusive with --body-file)")
	pageCreateCmd.Flags().StringVar(&pageCreateBodyFile, "body-file", "", "Read body from file (mutually exclusive with --body)")
	pageCreateCmd.Flags().StringVar(&pageCreateFormat, "body-format", "markdown", "Body format: markdown or storage")
	pageCreateCmd.Flags().StringVar(&pageCreateParent, "parent", "", "Parent page ID")
	pageCreateCmd.Flags().StringVar(&pageCreateType, "type", "page", "Content type: page or blogpost")
	markWrite(pageCreateCmd)

	pageUpdateCmd.Flags().StringVar(&pageUpdateTitle, "title", "", "New title")
	pageUpdateCmd.Flags().StringVar(&pageUpdateBody, "body", "", "New body content (mutually exclusive with --body-file)")
	pageUpdateCmd.Flags().StringVar(&pageUpdateBodyFile, "body-file", "", "Read new body from file (mutually exclusive with --body)")
	pageUpdateCmd.Flags().StringVar(&pageUpdateFormat, "body-format", "markdown", "Body format: markdown or storage")
	markWrite(pageUpdateCmd)

	pageDeleteCmd.Flags().BoolVar(&pageDeletePurge, "purge", false, "Permanently delete (irreversible second-stage removal)")
	markWrite(pageDeleteCmd)

	pageMoveCmd.Flags().StringVar(&pageMoveParent, "parent", "", "New parent page ID (required)")
	markWrite(pageMoveCmd)

	pageChildrenCmd.Flags().IntVar(&pageChildrenLimit, "limit", 0, "Maximum number of pages to return")
	pageChildrenCmd.Flags().IntVar(&pageChildrenStart, "start-at", 0, "Zero-based offset of the first result")

	pageDescendantsCmd.Flags().IntVar(&pageDescLimit, "limit", 0, "Maximum number of pages to return")
	pageDescendantsCmd.Flags().IntVar(&pageDescStart, "start-at", 0, "Zero-based offset of the first result")

	pageRestoreCmd.Flags().IntVar(&pageRestoreVersion, "version", 0, "Version number to restore (required)")
	markWrite(pageRestoreCmd)

	pageCmd.AddCommand(
		pageGetCmd, pageListCmd, pageCreateCmd, pageUpdateCmd, pageDeleteCmd,
		pageMoveCmd, pageChildrenCmd, pageDescendantsCmd, pageAncestorsCmd,
		pageHistoryCmd, pageRestoreCmd,
	)
	pageCmd.AddCommand(pageCommentCmd, pageAttachmentCmd, pageLabelCmd)
	rootCmd.AddCommand(pageCmd)

	dangerousCommandPaths["confluence-cli page delete"] = true
	dangerousCommandPaths["confluence-cli page comment delete"] = true
	dangerousCommandPaths["confluence-cli page attachment delete"] = true
}

// pageWebURL assembles a full clickable deep link from the client base URL and
// the content's _links.webui path.
func pageWebURL(client *api.Client, c *api.Content) string {
	if c.Links.WebUI == "" {
		return ""
	}
	return client.BaseURL() + c.Links.WebUI
}

// flatPageFromContent projects an api.Content onto the token-efficient FlatPage
// shape, tagging title as untrusted.
func flatPageFromContent(client *api.Client, c *api.Content) output.FlatPage {
	fp := output.FlatPage{
		ID:        c.ID,
		Title:     c.Title,
		Status:    c.Status,
		Type:      c.Type,
		URL:       pageWebURL(client, c),
		Untrusted: []string{"title"},
	}
	if c.Space != nil {
		fp.SpaceKey = c.Space.Key
	}
	if c.Version != nil {
		fp.Version = strconv.Itoa(c.Version.Number)
		if c.Version.When != "" {
			fp.Updated = c.Version.When
		}
		if c.Version.By != nil {
			fp.Author = c.Version.By.DisplayName
		}
	}
	if len(c.Ancestors) > 0 {
		fp.ParentID = c.Ancestors[len(c.Ancestors)-1].ID
	}
	return fp
}

// readBodyInput resolves body text from an inline value and a file path,
// enforcing mutual exclusivity. Returns ("", false) when neither is set.
func readBodyInput(body, bodyFile string) (string, bool, error) {
	if body != "" && bodyFile != "" {
		return "", false, errBodyConflict
	}
	if bodyFile != "" {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return "", false, err
		}
		return string(data), true, nil
	}
	if body != "" {
		return body, true, nil
	}
	return "", false, nil
}

var errBodyConflict = &usageError{"--body and --body-file are mutually exclusive"}

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// toStorage converts a body from the given format to Confluence storage format.
func toStorage(body, format string) (string, error) {
	switch format {
	case "markdown", "":
		return render.MarkdownToStorage(body)
	case "storage":
		return body, nil
	default:
		return "", &usageError{"--body-format must be markdown or storage"}
	}
}

func runPageGet(_ *cobra.Command, args []string) error {
	byLocator := pageGetSpace != "" || pageGetTitle != ""
	if len(args) == 1 && byLocator {
		emitError(output.ErrValidation, "provide either an ID or --space/--title, not both", nil)
		return SilentErr(ExitBadArgs)
	}
	if len(args) == 0 && !byLocator {
		emitError(output.ErrValidation, "provide a page ID or both --space and --title", nil)
		return SilentErr(ExitBadArgs)
	}
	switch pageGetBodyFormat {
	case "markdown", "storage", "view":
	default:
		emitError(output.ErrValidation, "--body-format must be markdown, storage, or view", nil)
		return SilentErr(ExitBadArgs)
	}

	client, err := newClient()
	if err != nil {
		return err
	}

	expand := []string{"space", "version", "ancestors"}
	switch pageGetBodyFormat {
	case "view":
		expand = append(expand, "body.view")
	default:
		expand = append(expand, "body.storage")
	}

	var c *api.Content
	if byLocator {
		if pageGetSpace == "" || pageGetTitle == "" {
			emitError(output.ErrValidation, "--space and --title must be provided together", nil)
			return SilentErr(ExitBadArgs)
		}
		c, err = client.Content.GetContentBySpaceTitle(pageGetSpace, pageGetTitle, expand)
	} else {
		c, err = client.Content.GetContent(args[0], api.GetContentOptions{Expand: expand})
	}
	if err != nil {
		return emitAPIError(err)
	}

	fp := flatPageFromContent(client, c)
	result := map[string]any{
		"id":         fp.ID,
		"title":      fp.Title,
		"space_key":  fp.SpaceKey,
		"status":     fp.Status,
		"type":       fp.Type,
		"version":    versionNumber(c),
		"url":        fp.URL,
		"_untrusted": []string{"title", "body"},
	}
	if fp.ParentID != "" {
		result["parent_id"] = fp.ParentID
	}

	if err := attachBody(result, c); err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(result)
	return nil
}

// attachBody renders the requested body representation into result and records
// conversion metadata (fidelity + unsupported macros) for markdown.
func attachBody(result map[string]any, c *api.Content) error {
	body := ""
	if c.Body != nil {
		switch pageGetBodyFormat {
		case "view":
			if c.Body.View != nil {
				body = c.Body.View.Value
			}
		default:
			if c.Body.Storage != nil {
				body = c.Body.Storage.Value
			}
		}
	}
	switch pageGetBodyFormat {
	case "storage", "view":
		result["body"] = body
		result["body_format"] = pageGetBodyFormat
	default:
		res, err := render.StorageToMarkdown(body)
		if err != nil {
			return err
		}
		result["body"] = res.Markdown
		result["body_format"] = "markdown"
		result["meta"] = map[string]any{"fidelity": res.Fidelity}
		if len(res.UnsupportedMacros) > 0 {
			result["unsupported_macros"] = res.UnsupportedMacros
		}
	}
	return nil
}

func versionNumber(c *api.Content) int {
	if c.Version != nil {
		return c.Version.Number
	}
	return 0
}

func runPageList(_ *cobra.Command, _ []string) error {
	if strings.TrimSpace(pageListSpace) == "" {
		emitError(output.ErrValidation, "--space is required", nil)
		return SilentErr(ExitBadArgs)
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	cql := "space = " + cqlQuote(pageListSpace) + " AND type = page"
	page, err := client.Search.Search(cql, api.SearchOptions{Start: pageListStart, Limit: pageListLimit})
	if err != nil {
		return emitAPIError(err)
	}
	items := make([]map[string]any, 0, len(page.Results))
	for i := range page.Results {
		r := &page.Results[i]
		fp := output.FlatPage{
			ID:        "",
			Title:     r.Title,
			SpaceKey:  pageListSpace,
			Untrusted: []string{"title"},
		}
		if r.Content != nil {
			fp.ID = r.Content.ID
			fp.Type = r.Content.Type
			fp.URL = client.BaseURL() + r.Content.Links.WebUI
		}
		items = append(items, output.FilterPageFields(fp, fieldsList))
	}
	output.PrintJSON(map[string]any{
		"pages":         items,
		"start_at":      page.Start,
		"size":          page.Size,
		"total_size":    page.TotalSize,
		"has_more":      page.HasMore,
		"next_start_at": page.NextStart,
	})
	return nil
}

func runPageCreate(_ *cobra.Command, _ []string) error {
	space := strings.TrimSpace(pageCreateSpace)
	title := strings.TrimSpace(pageCreateTitle)
	if space == "" || title == "" {
		emitError(output.ErrValidation, "--space and --title are required", nil)
		return SilentErr(ExitBadArgs)
	}
	if pageCreateType != "page" && pageCreateType != "blogpost" {
		emitError(output.ErrValidation, "--type must be page or blogpost", nil)
		return SilentErr(ExitBadArgs)
	}
	body, _, err := readBodyInput(pageCreateBody, pageCreateBodyFile)
	if err != nil {
		emitError(output.ErrValidation, err.Error(), nil)
		return SilentErr(ExitBadArgs)
	}
	storage, err := toStorage(body, pageCreateFormat)
	if err != nil {
		emitError(output.ErrValidation, err.Error(), nil)
		return SilentErr(ExitBadArgs)
	}

	preview := map[string]any{
		"space": space,
		"title": title,
		"type":  pageCreateType,
	}
	if pageCreateParent != "" {
		preview["parent"] = pageCreateParent
	}
	preview["storage_preview"] = truncate(storage, 500)
	if dryRunOutput("page create", preview) {
		return nil
	}

	client, err := newClient()
	if err != nil {
		return err
	}
	req := &api.CreateContentRequest{
		Type:  pageCreateType,
		Title: title,
		Space: api.SpaceKeyRef{Key: space},
		Body:  api.RequestBody{Storage: api.BodyRepresentation{Value: storage, Representation: "storage"}},
	}
	if pageCreateParent != "" {
		req.Ancestors = []api.ContentRef{{ID: pageCreateParent}}
	}
	c, err := client.Content.CreateContent(req)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(pageResultMap(client, c))
	return nil
}

func runPageUpdate(_ *cobra.Command, args []string) error {
	id := args[0]
	body, bodySet, err := readBodyInput(pageUpdateBody, pageUpdateBodyFile)
	if err != nil {
		emitError(output.ErrValidation, err.Error(), nil)
		return SilentErr(ExitBadArgs)
	}
	if pageUpdateTitle == "" && !bodySet {
		emitError(output.ErrValidation, "provide --title and/or --body/--body-file", nil)
		return SilentErr(ExitBadArgs)
	}
	var storage string
	if bodySet {
		storage, err = toStorage(body, pageUpdateFormat)
		if err != nil {
			emitError(output.ErrValidation, err.Error(), nil)
			return SilentErr(ExitBadArgs)
		}
	}

	client, err := newClient()
	if err != nil {
		return err
	}
	current, err := client.Content.GetContent(id, api.GetContentOptions{Expand: []string{"version"}})
	if err != nil {
		return emitAPIError(err)
	}
	curVer := versionNumber(current)

	detail := map[string]any{"id": id, "current_version": curVer}
	if pageUpdateTitle != "" {
		detail["new_title"] = pageUpdateTitle
	}
	if dryRunOutput("page update", detail) {
		return nil
	}

	// Optimistic-lock re-check: the version must not have drifted since dry-run.
	fresh, err := client.Content.GetContent(id, api.GetContentOptions{Expand: []string{"version"}})
	if err != nil {
		return emitAPIError(err)
	}
	if versionNumber(fresh) != curVer {
		emitError(output.ErrConflict, "page version changed since dry-run; re-run --dry-run", map[string]any{
			"expected_version": curVer,
			"actual_version":   versionNumber(fresh),
		})
		return SilentErr(ExitConflict)
	}

	title := pageUpdateTitle
	if title == "" {
		title = current.Title
	}
	req := &api.UpdateContentRequest{
		Type:    contentType(current),
		Title:   title,
		Version: api.VersionRef{Number: curVer + 1},
	}
	if bodySet {
		req.Body = &api.RequestBody{Storage: api.BodyRepresentation{Value: storage, Representation: "storage"}}
	}
	c, err := client.Content.UpdateContent(id, req)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(pageResultMap(client, c))
	return nil
}

func runPageDelete(_ *cobra.Command, args []string) error {
	id := args[0]
	client, err := newClient()
	if err != nil {
		return err
	}
	// Preview needs the page title + descendant count before the gate decision.
	c, descendants, gerr := deletePreviewData(client, id)
	if gerr != nil {
		return emitAPIError(gerr)
	}
	detail := map[string]any{
		"id":          id,
		"title":       c.Title,
		"descendants": descendants,
		"purge":       pageDeletePurge,
	}
	if pageDeletePurge {
		detail["irreversible"] = true
	}
	if dryRunOutput("page delete", detail) {
		return nil
	}
	if err := client.Content.DeleteContent(id, pageDeletePurge); err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(map[string]any{"id": id, "status": "deleted", "purged": pageDeletePurge})
	return nil
}

// deletePreviewData fetches the target title and its descendant count for the
// delete preview. On dangerous-gate rejection this still runs (cheap reads).
func deletePreviewData(client *api.Client, id string) (*api.Content, int, error) {
	c, err := client.Content.GetContent(id, api.GetContentOptions{})
	if err != nil {
		return nil, 0, err
	}
	page, err := client.Content.DescendantPages(id, 0, 0)
	if err != nil {
		return nil, 0, err
	}
	return c, page.Size, nil
}

func runPageMove(_ *cobra.Command, args []string) error {
	id := args[0]
	if strings.TrimSpace(pageMoveParent) == "" {
		emitError(output.ErrValidation, "--parent is required", nil)
		return SilentErr(ExitBadArgs)
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	current, err := client.Content.GetContent(id, api.GetContentOptions{Expand: []string{"version"}})
	if err != nil {
		return emitAPIError(err)
	}
	curVer := versionNumber(current)
	detail := map[string]any{"id": id, "new_parent": pageMoveParent, "current_version": curVer}
	if dryRunOutput("page move", detail) {
		return nil
	}
	fresh, err := client.Content.GetContent(id, api.GetContentOptions{Expand: []string{"version"}})
	if err != nil {
		return emitAPIError(err)
	}
	if versionNumber(fresh) != curVer {
		emitError(output.ErrConflict, "page version changed since dry-run; re-run --dry-run", map[string]any{
			"expected_version": curVer,
			"actual_version":   versionNumber(fresh),
		})
		return SilentErr(ExitConflict)
	}
	req := &api.UpdateContentRequest{
		Type:      contentType(current),
		Title:     current.Title,
		Version:   api.VersionRef{Number: curVer + 1},
		Ancestors: []api.ContentRef{{ID: pageMoveParent}},
	}
	c, err := client.Content.UpdateContent(id, req)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(pageResultMap(client, c))
	return nil
}

func runPageChildren(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	page, err := client.Content.ChildPages(args[0], pageChildrenStart, pageChildrenLimit)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(flatPagePage(client, page))
	return nil
}

func runPageDescendants(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	page, err := client.Content.DescendantPages(args[0], pageDescStart, pageDescLimit)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(flatPagePage(client, page))
	return nil
}

func runPageAncestors(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	ancestors, err := client.Content.Ancestors(args[0])
	if err != nil {
		return emitAPIError(err)
	}
	breadcrumb := make([]map[string]any, 0, len(ancestors))
	for i := range ancestors {
		a := &ancestors[i]
		breadcrumb = append(breadcrumb, map[string]any{
			"id":         a.ID,
			"title":      a.Title,
			"url":        pageWebURL(client, a),
			"_untrusted": []string{"title"},
		})
	}
	output.PrintJSON(map[string]any{"ancestors": breadcrumb})
	return nil
}

func runPageHistory(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	h, err := client.Content.ContentHistory(args[0])
	if err != nil {
		return emitAPIError(err)
	}
	result := map[string]any{"id": args[0], "latest": h.Latest}
	if h.LastUpdated != nil {
		result["versions"] = []map[string]any{versionEntry(h.LastUpdated)}
	}
	if h.CreatedBy != nil {
		result["created_by"] = h.CreatedBy.DisplayName
		result["created_date"] = h.CreatedDate
	}
	output.PrintJSON(result)
	return nil
}

func versionEntry(v *api.Version) map[string]any {
	e := map[string]any{"number": v.Number, "when": v.When, "message": v.Message}
	if v.By != nil {
		e["by"] = v.By.DisplayName
	}
	return e
}

func runPageRestore(_ *cobra.Command, args []string) error {
	id := args[0]
	if pageRestoreVersion <= 0 {
		emitError(output.ErrValidation, "--version is required and must be positive", nil)
		return SilentErr(ExitBadArgs)
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	// Read the historical version body to restore.
	old, err := client.Content.GetContent(id, api.GetContentOptions{
		Expand:  []string{"body.storage"},
		Status:  "historical",
		Version: pageRestoreVersion,
	})
	if err != nil {
		return emitAPIError(err)
	}
	current, err := client.Content.GetContent(id, api.GetContentOptions{Expand: []string{"version"}})
	if err != nil {
		return emitAPIError(err)
	}
	curVer := versionNumber(current)
	detail := map[string]any{
		"id":              id,
		"restore_version": pageRestoreVersion,
		"current_version": curVer,
		"title":           old.Title,
	}
	if dryRunOutput("page restore", detail) {
		return nil
	}
	fresh, err := client.Content.GetContent(id, api.GetContentOptions{Expand: []string{"version"}})
	if err != nil {
		return emitAPIError(err)
	}
	if versionNumber(fresh) != curVer {
		emitError(output.ErrConflict, "page version changed since dry-run; re-run --dry-run", map[string]any{
			"expected_version": curVer,
			"actual_version":   versionNumber(fresh),
		})
		return SilentErr(ExitConflict)
	}
	oldBody := ""
	if old.Body != nil && old.Body.Storage != nil {
		oldBody = old.Body.Storage.Value
	}
	req := &api.UpdateContentRequest{
		Type:    contentType(current),
		Title:   old.Title,
		Version: api.VersionRef{Number: curVer + 1},
		Body:    &api.RequestBody{Storage: api.BodyRepresentation{Value: oldBody, Representation: "storage"}},
	}
	c, err := client.Content.UpdateContent(id, req)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(pageResultMap(client, c))
	return nil
}

// pageResultMap is the standard success shape for create/update/move/restore.
func pageResultMap(client *api.Client, c *api.Content) map[string]any {
	m := map[string]any{
		"id":         c.ID,
		"title":      c.Title,
		"type":       c.Type,
		"status":     c.Status,
		"version":    versionNumber(c),
		"url":        pageWebURL(client, c),
		"_untrusted": []string{"title"},
	}
	if c.Space != nil {
		m["space_key"] = c.Space.Key
	}
	return m
}

// flatPagePage projects a paged Content list onto the list output shape.
func flatPagePage(client *api.Client, page *api.Page[api.Content]) map[string]any {
	items := make([]map[string]any, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, output.FilterPageFields(flatPageFromContent(client, &page.Items[i]), fieldsList))
	}
	return map[string]any{
		"pages":         items,
		"start_at":      page.Start,
		"limit":         page.Limit,
		"size":          page.Size,
		"has_more":      page.HasMore,
		"next_start_at": page.NextStartAt,
	}
}

func contentType(c *api.Content) string {
	if c.Type != "" {
		return c.Type
	}
	return "page"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
