package cmd

import (
	"strings"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/fatecannotbealtered/confluence-cli/internal/render"
	"github.com/spf13/cobra"
)

var (
	commentListLocation string
	commentListLimit    int
	commentListStart    int
	commentAddBody      string
	commentAddReplyTo   string
)

var pageCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Manage page comments",
}

var pageCommentListCmd = &cobra.Command{
	Use:   "list <PAGE_ID>",
	Short: "List comments on a page",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommentList,
}

var pageCommentGetCmd = &cobra.Command{
	Use:   "get <COMMENT_ID>",
	Short: "Get a single comment",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommentGet,
}

var pageCommentAddCmd = &cobra.Command{
	Use:   "add <PAGE_ID>",
	Short: "Add a comment to a page",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommentAdd,
}

var pageCommentDeleteCmd = &cobra.Command{
	Use:   "delete <COMMENT_ID>",
	Short: "Delete a comment (write-dangerous)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommentDelete,
}

func init() {
	pageCommentListCmd.Flags().StringVar(&commentListLocation, "location", "all", "Filter by location: inline, footer, or all")
	pageCommentListCmd.Flags().IntVar(&commentListLimit, "limit", 0, "Maximum number of comments to return")
	pageCommentListCmd.Flags().IntVar(&commentListStart, "start-at", 0, "Zero-based offset of the first result")

	pageCommentAddCmd.Flags().StringVar(&commentAddBody, "body", "", "Comment body in Markdown (required)")
	pageCommentAddCmd.Flags().StringVar(&commentAddReplyTo, "reply-to", "", "Parent comment ID for a threaded reply")
	markWrite(pageCommentAddCmd)

	markWrite(pageCommentDeleteCmd)

	pageCommentCmd.AddCommand(pageCommentListCmd, pageCommentGetCmd, pageCommentAddCmd, pageCommentDeleteCmd)
}

// commentMap projects a comment Content into the output shape, converting the
// storage body to Markdown and surfacing inline resolution status.
func commentMap(c *api.Content) map[string]any {
	body := ""
	if c.Body != nil && c.Body.Storage != nil {
		if res, err := render.StorageToMarkdown(c.Body.Storage.Value); err == nil {
			body = res.Markdown
		} else {
			body = c.Body.Storage.Value
		}
	}
	m := map[string]any{
		"id":         c.ID,
		"title":      c.Title,
		"body":       body,
		"_untrusted": []string{"title", "body"},
	}
	if c.Extensions != nil {
		if c.Extensions.Location != "" {
			m["location"] = c.Extensions.Location
		}
		if c.Extensions.Resolution != nil {
			m["resolution"] = c.Extensions.Resolution.Status
		}
	}
	return m
}

func runCommentList(_ *cobra.Command, args []string) error {
	location := commentListLocation
	switch location {
	case "all", "":
		location = ""
	case "inline", "footer":
	default:
		emitError(output.ErrValidation, "--location must be inline, footer, or all", nil)
		return SilentErr(ExitBadArgs)
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	page, err := client.Comments.ListComments(args[0], location, commentListStart, commentListLimit)
	if err != nil {
		return emitAPIError(err)
	}
	items := make([]map[string]any, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, commentMap(&page.Items[i]))
	}
	output.PrintJSON(output.PagedMap(items, len(items), page.Start, page.NextStartAt, page.HasMore))
	return nil
}

func runCommentGet(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	c, err := client.Content.GetContent(args[0], api.GetContentOptions{
		Expand: []string{"body.storage", "extensions.resolution"},
	})
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(commentMap(c))
	return nil
}

func runCommentAdd(_ *cobra.Command, args []string) error {
	if strings.TrimSpace(commentAddBody) == "" {
		emitError(output.ErrValidation, "--body is required", nil)
		return SilentErr(ExitBadArgs)
	}
	storage, err := render.MarkdownToStorage(commentAddBody)
	if err != nil {
		emitError(output.ErrValidation, err.Error(), nil)
		return SilentErr(ExitBadArgs)
	}
	detail := map[string]any{
		"page_id":      args[0],
		"body_preview": truncate(storage, 500),
		// Bind the full body into the confirm token, not just the preview.
		"body_sha256": tokenFingerprint(storage),
	}
	if commentAddReplyTo != "" {
		detail["reply_to"] = commentAddReplyTo
	}
	if dryRunOutput("page comment add", detail) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	c, err := client.Comments.CreateComment(args[0], storage, commentAddReplyTo)
	if err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(commentMap(c))
	return nil
}

func runCommentDelete(_ *cobra.Command, args []string) error {
	detail := map[string]any{"comment_id": args[0]}
	if dryRunOutput("page comment delete", detail) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	if err := client.Comments.DeleteComment(args[0]); err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(map[string]any{"comment_id": args[0], "status": "deleted"})
	return nil
}
