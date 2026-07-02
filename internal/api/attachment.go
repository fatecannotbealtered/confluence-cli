package api

import (
	"fmt"
	"io"
	"net/url"
)

// ListAttachments lists attachments on a content entity.
// GET /rest/api/content/{id}/child/attachment
func (a *AttachmentAPI) ListAttachments(contentID string, start, limit int) (*Page[Content], error) {
	return getPage[Content](a.client, restPath("/content/"+url.PathEscape(contentID)+"/child/attachment"), nil, start, limit)
}

// UploadAttachment uploads one or more files as new attachments (multipart,
// X-Atlassian-Token: no-check). DC returns 400 if a same-named attachment
// already exists — use UpdateAttachmentData for that.
// POST /rest/api/content/{id}/child/attachment
func (a *AttachmentAPI) UploadAttachment(contentID string, files []string, comment string) ([]Content, error) {
	data, err := a.client.uploadMultipart(restPath("/content/"+url.PathEscape(contentID)+"/child/attachment"), files, comment, false)
	if err != nil {
		return nil, err
	}
	page, err := parsePage[Content](data)
	if err != nil {
		return nil, fmt.Errorf("parsing uploaded attachments: %w", err)
	}
	return page.Items, nil
}

// UpdateAttachmentData uploads a new version of an existing attachment
// (same-name upload endpoint bumps the version).
// POST /rest/api/content/{id}/child/attachment/{attachmentId}/data
func (a *AttachmentAPI) UpdateAttachmentData(contentID, attachmentID, file, comment string) ([]Content, error) {
	path := restPath("/content/" + url.PathEscape(contentID) + "/child/attachment/" + url.PathEscape(attachmentID) + "/data")
	data, err := a.client.uploadMultipart(path, []string{file}, comment, false)
	if err != nil {
		return nil, err
	}
	// This endpoint returns a single attachment object, not a paged list.
	c, err := parseContent(data)
	if err != nil {
		return nil, err
	}
	return []Content{*c}, nil
}

// DownloadAttachment streams the attachment binary by following its
// _links.download path. The caller must Close the reader.
func (a *AttachmentAPI) DownloadAttachment(downloadLink string) (io.ReadCloser, error) {
	if downloadLink == "" {
		return nil, notFoundError("attachment has no download link")
	}
	return a.client.download(downloadLink)
}

// DeleteAttachment moves an attachment to trash.
// DELETE /rest/api/content/{attachmentId}
func (a *AttachmentAPI) DeleteAttachment(attachmentID string) error {
	_, err := a.client.del(restPath("/content/" + url.PathEscape(attachmentID)))
	return err
}
