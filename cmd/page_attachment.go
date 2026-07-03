package cmd

import (
	"io"
	"os"
	"path/filepath"

	"github.com/fatecannotbealtered/confluence-cli/internal/api"
	"github.com/fatecannotbealtered/confluence-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	attachListLimit   int
	attachListStart   int
	attachUploadFiles []string
	attachUploadOver  bool
	attachDownloadOut string
)

var pageAttachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Manage page attachments",
}

var pageAttachmentListCmd = &cobra.Command{
	Use:   "list <PAGE_ID>",
	Short: "List attachments on a page",
	Args:  cobra.ExactArgs(1),
	RunE:  runAttachmentList,
}

var pageAttachmentUploadCmd = &cobra.Command{
	Use:   "upload <PAGE_ID>",
	Short: "Upload one or more attachments",
	Args:  cobra.ExactArgs(1),
	RunE:  runAttachmentUpload,
}

var pageAttachmentDownloadCmd = &cobra.Command{
	Use:   "download <ATTACHMENT_ID>",
	Short: "Download an attachment",
	Args:  cobra.ExactArgs(1),
	RunE:  runAttachmentDownload,
}

var pageAttachmentDeleteCmd = &cobra.Command{
	Use:   "delete <ATTACHMENT_ID>",
	Short: "Delete an attachment (write-dangerous)",
	Args:  cobra.ExactArgs(1),
	RunE:  runAttachmentDelete,
}

func init() {
	pageAttachmentListCmd.Flags().IntVar(&attachListLimit, "limit", 0, "Maximum number of attachments to return")
	pageAttachmentListCmd.Flags().IntVar(&attachListStart, "start-at", 0, "Zero-based offset of the first result")

	pageAttachmentUploadCmd.Flags().StringArrayVar(&attachUploadFiles, "file", nil, "File to upload (repeatable, required)")
	pageAttachmentUploadCmd.Flags().BoolVar(&attachUploadOver, "overwrite", false, "Overwrite same-named attachments as a new version")
	markWrite(pageAttachmentUploadCmd)

	pageAttachmentDownloadCmd.Flags().StringVar(&attachDownloadOut, "out", "", "Output path (defaults to the original filename in the current directory)")
	if pageAttachmentDownloadCmd.Annotations == nil {
		pageAttachmentDownloadCmd.Annotations = map[string]string{}
	}
	pageAttachmentDownloadCmd.Annotations["format.raw"] = "true"

	markWrite(pageAttachmentDeleteCmd)

	pageAttachmentCmd.AddCommand(pageAttachmentListCmd, pageAttachmentUploadCmd, pageAttachmentDownloadCmd, pageAttachmentDeleteCmd)
}

// attachmentMap projects an attachment Content into the output shape. The
// filename (title) is external content and tagged untrusted.
func attachmentMap(c *api.Content) map[string]any {
	m := map[string]any{
		"id":         c.ID,
		"filename":   c.Title,
		"_untrusted": []string{"filename"},
	}
	if c.Extensions != nil {
		if c.Extensions.MediaType != "" {
			m["media_type"] = c.Extensions.MediaType
		}
		if c.Extensions.FileSize != 0 {
			m["file_size"] = c.Extensions.FileSize
		}
	}
	if c.Version != nil {
		m["version"] = c.Version.Number
	}
	return m
}

func runAttachmentList(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	page, err := client.Attachments.ListAttachments(args[0], attachListStart, attachListLimit)
	if err != nil {
		return emitAPIError(err)
	}
	items := make([]map[string]any, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, attachmentMap(&page.Items[i]))
	}
	output.PrintJSON(map[string]any{
		"attachments":   items,
		"start_at":      page.Start,
		"limit":         page.Limit,
		"size":          page.Size,
		"has_more":      page.HasMore,
		"next_start_at": page.NextStartAt,
	})
	return nil
}

func runAttachmentUpload(_ *cobra.Command, args []string) error {
	pageID := args[0]
	if len(attachUploadFiles) == 0 {
		emitError(output.ErrValidation, "--file is required", nil)
		return SilentErr(ExitBadArgs)
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	// Pre-check same-named attachments so the preview and confirm agree.
	existing, err := existingAttachmentNames(client, pageID)
	if err != nil {
		return emitAPIError(err)
	}
	var conflicts []string
	for _, f := range attachUploadFiles {
		if existing[filepath.Base(f)] != "" {
			conflicts = append(conflicts, filepath.Base(f))
		}
	}
	detail := map[string]any{"page_id": pageID, "files": basenames(attachUploadFiles)}
	if len(conflicts) > 0 {
		if attachUploadOver {
			detail["will_overwrite"] = conflicts
		} else {
			detail["conflict"] = conflicts
		}
	}
	if dryRunOutput("page attachment upload", detail) {
		return nil
	}
	// Enforce the conflict policy at execution time.
	if len(conflicts) > 0 && !attachUploadOver {
		emitError(output.ErrConflict, "same-named attachment(s) already exist; use --overwrite", map[string]any{"conflict": conflicts})
		return SilentErr(ExitConflict)
	}

	uploaded := make([]map[string]any, 0, len(attachUploadFiles))
	for _, f := range attachUploadFiles {
		base := filepath.Base(f)
		if attachUploadOver && existing[base] != "" {
			res, uerr := client.Attachments.UpdateAttachmentData(pageID, existing[base], f, "")
			if uerr != nil {
				return emitAPIError(uerr)
			}
			for i := range res {
				uploaded = append(uploaded, attachmentMap(&res[i]))
			}
			continue
		}
		res, uerr := client.Attachments.UploadAttachment(pageID, []string{f}, "")
		if uerr != nil {
			return emitAPIError(uerr)
		}
		for i := range res {
			uploaded = append(uploaded, attachmentMap(&res[i]))
		}
	}
	output.PrintJSON(map[string]any{"attachments": uploaded, "count": len(uploaded)})
	return nil
}

// existingAttachmentNames maps attachment filename -> id for the page.
func existingAttachmentNames(client *api.Client, pageID string) (map[string]string, error) {
	names := map[string]string{}
	start := 0
	for {
		page, err := client.Attachments.ListAttachments(pageID, start, 0)
		if err != nil {
			return nil, err
		}
		for i := range page.Items {
			names[page.Items[i].Title] = page.Items[i].ID
		}
		if !page.HasMore || len(page.Items) == 0 {
			return names, nil
		}
		start = page.NextStartAt
	}
}

func basenames(files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, filepath.Base(f))
	}
	return out
}

func runAttachmentDownload(_ *cobra.Command, args []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	meta, err := client.Content.GetContent(args[0], api.GetContentOptions{})
	if err != nil {
		return emitAPIError(err)
	}
	rc, err := client.Attachments.DownloadAttachment(meta.Links.Download)
	if err != nil {
		return emitAPIError(err)
	}
	defer func() { _ = rc.Close() }()

	if outputFormat == outputFormatRaw {
		if _, cerr := io.Copy(os.Stdout, rc); cerr != nil {
			return emitAPIError(cerr)
		}
		return nil
	}

	outPath := attachDownloadOut
	if outPath == "" {
		outPath = meta.Title
		if outPath == "" {
			outPath = args[0]
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		emitError(output.ErrValidation, err.Error(), nil)
		return SilentErr(ExitGeneric)
	}
	size, cerr := io.Copy(f, rc)
	closeErr := f.Close()
	if cerr != nil {
		return emitAPIError(cerr)
	}
	if closeErr != nil {
		emitError(output.ErrValidation, closeErr.Error(), nil)
		return SilentErr(ExitGeneric)
	}
	output.PrintJSON(map[string]any{"path": outPath, "size_bytes": size})
	return nil
}

func runAttachmentDelete(_ *cobra.Command, args []string) error {
	detail := map[string]any{"attachment_id": args[0]}
	if dryRunOutput("page attachment delete", detail) {
		return nil
	}
	client, err := newClient()
	if err != nil {
		return err
	}
	if err := client.Attachments.DeleteAttachment(args[0]); err != nil {
		return emitAPIError(err)
	}
	output.PrintJSON(map[string]any{"attachment_id": args[0], "status": "deleted"})
	return nil
}
