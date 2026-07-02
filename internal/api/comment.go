package api

import (
	"net/url"
)

// ListComments lists comments on a content entity. location filters
// "inline" or "footer" ("" = all). Expands resolution + storage body so
// inline-comment status is visible.
// GET /rest/api/content/{id}/child/comment
func (a *CommentAPI) ListComments(contentID, location string, start, limit int) (*Page[Content], error) {
	params := url.Values{}
	params.Set("expand", "extensions.resolution,body.storage")
	if location != "" {
		params.Set("location", location)
	}
	return getPage[Content](a.client, restPath("/content/"+url.PathEscape(contentID)+"/child/comment"), params, start, limit)
}

// CreateComment creates a footer comment on contentID; replyTo (optional) is
// the parent comment ID for a threaded reply.
// POST /rest/api/content
func (a *CommentAPI) CreateComment(contentID, bodyStorage, replyTo string) (*Content, error) {
	req := createCommentRequest{
		Type:      "comment",
		Container: ContentRef{ID: contentID, Type: "page"},
		Body: RequestBody{
			Storage: BodyRepresentation{Value: bodyStorage, Representation: "storage"},
		},
	}
	if replyTo != "" {
		req.Ancestors = []ContentRef{{ID: replyTo}}
	}
	data, err := a.client.post(restPath("/content"), &req)
	if err != nil {
		return nil, err
	}
	return parseContent(data)
}

// DeleteComment deletes a comment.
// DELETE /rest/api/content/{commentId}
func (a *CommentAPI) DeleteComment(commentID string) error {
	_, err := a.client.del(restPath("/content/" + url.PathEscape(commentID)))
	return err
}
