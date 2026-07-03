package cmd

import (
	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	labelListLimit int
	labelListStart int
	labelAddNames  []string
	labelRmNames   []string
)

var pageLabelCmd = &cobra.Command{
	Use:   "label",
	Short: "Manage page labels",
}

var pageLabelListCmd = &cobra.Command{
	Use:   "list <PAGE_ID>",
	Short: "List labels on a page",
	Args:  cobra.ExactArgs(1),
	RunE:  runLabelList,
}

var pageLabelAddCmd = &cobra.Command{
	Use:   "add <PAGE_ID>",
	Short: "Add labels to a page",
	Args:  cobra.ExactArgs(1),
	RunE:  runLabelAdd,
}

var pageLabelRemoveCmd = &cobra.Command{
	Use:   "remove <PAGE_ID>",
	Short: "Remove labels from a page",
	Args:  cobra.ExactArgs(1),
	RunE:  runLabelRemove,
}

func init() {
	pageLabelListCmd.Flags().IntVar(&labelListLimit, "limit", 0, "Maximum number of labels to return")
	pageLabelListCmd.Flags().IntVar(&labelListStart, "start-at", 0, "Zero-based offset of the first result")

	pageLabelAddCmd.Flags().StringSliceVar(&labelAddNames, "labels", nil, "Labels to add (comma-separated, required)")
	markWrite(pageLabelAddCmd)

	pageLabelRemoveCmd.Flags().StringSliceVar(&labelRmNames, "labels", nil, "Labels to remove (comma-separated, required)")
	markWrite(pageLabelRemoveCmd)

	pageLabelCmd.AddCommand(pageLabelListCmd, pageLabelAddCmd, pageLabelRemoveCmd)
}

func labelMap(l *api.Label) map[string]any {
	return map[string]any{
		"name":   l.Name,
		"prefix": l.Prefix,
		"id":     l.ID,
	}
}

func runLabelList(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	page, err := client.Labels.ListLabels(args[0], labelListStart, labelListLimit)
	if err != nil {
		return emitAPIError(err)
	}
	items := make([]map[string]any, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, labelMap(&page.Items[i]))
	}
	output.PrintJSON(map[string]any{
		"labels":        items,
		"start_at":      page.Start,
		"limit":         page.Limit,
		"size":          page.Size,
		"has_more":      page.HasMore,
		"next_start_at": page.NextStartAt,
	})
	return nil
}

func runLabelAdd(_ *cobra.Command, args []string) error {
	if len(labelAddNames) == 0 {
		emitError(output.ErrValidation, "--labels is required", nil)
		return SilentErr(ExitBadArgs)
	}
	detail := map[string]any{"page_id": args[0], "labels": labelAddNames}
	if dryRunOutput("page label add", detail) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	labels, err := client.Labels.AddLabels(args[0], labelAddNames)
	if err != nil {
		return emitAPIError(err)
	}
	items := make([]map[string]any, 0, len(labels))
	for i := range labels {
		items = append(items, labelMap(&labels[i]))
	}
	output.PrintJSON(map[string]any{"labels": items, "count": len(items)})
	return nil
}

func runLabelRemove(_ *cobra.Command, args []string) error {
	if len(labelRmNames) == 0 {
		emitError(output.ErrValidation, "--labels is required", nil)
		return SilentErr(ExitBadArgs)
	}
	detail := map[string]any{"page_id": args[0], "labels": labelRmNames}
	if dryRunOutput("page label remove", detail) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	// Remove each label; aggregate per-item results so partial failures are
	// fully reported with ok=false but every attempt visible.
	items := make([]map[string]any, 0, len(labelRmNames))
	allOK := true
	for _, name := range labelRmNames {
		entry := map[string]any{"name": name}
		if err := client.Labels.RemoveLabel(args[0], name); err != nil {
			allOK = false
			entry["ok"] = false
			entry["error"] = err.Error()
		} else {
			entry["ok"] = true
		}
		items = append(items, entry)
	}
	output.PrintJSON(map[string]any{
		"page_id": args[0],
		"ok":      allOK,
		"items":   items,
		"summary": map[string]any{"total": len(labelRmNames), "removed": countOK(items)},
	})
	if !allOK {
		return SilentErr(ExitGeneric)
	}
	return nil
}

func countOK(items []map[string]any) int {
	n := 0
	for _, it := range items {
		if it["ok"] == true {
			n++
		}
	}
	return n
}
