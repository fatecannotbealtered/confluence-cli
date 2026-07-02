package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestListAttachments(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123/child/attachment" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{
			"results":[{
				"id":"att1","type":"attachment","title":"report.pdf",
				"extensions":{"mediaType":"application/pdf","fileSize":2048,"comment":"v1"},
				"_links":{"download":"/download/attachments/123/report.pdf"}
			}],
			"start":0,"limit":25,"size":1,"_links":{}
		}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	page, err := c.Attachments.ListAttachments("123", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	att := page.Items[0]
	if att.ID != "att1" || att.Title != "report.pdf" {
		t.Errorf("attachment = %+v", att)
	}
	if att.Extensions == nil || att.Extensions.MediaType != "application/pdf" || att.Extensions.FileSize != 2048 {
		t.Errorf("extensions = %+v", att.Extensions)
	}
	if att.Links.Download != "/download/attachments/123/report.pdf" {
		t.Errorf("download link = %q", att.Links.Download)
	}
}

func TestUploadAttachment_Success(t *testing.T) {
	file := writeTempFile(t, "hello.txt", "hello world")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/content/123/child/attachment" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Atlassian-Token"); got != "no-check" {
			t.Errorf("X-Atlassian-Token = %q, want no-check", got)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		fhs := r.MultipartForm.File["file"]
		if len(fhs) != 1 || fhs[0].Filename != "hello.txt" {
			t.Errorf("files = %+v", fhs)
		}
		f, _ := fhs[0].Open()
		body, _ := io.ReadAll(f)
		_ = f.Close()
		if string(body) != "hello world" {
			t.Errorf("file body = %q", body)
		}
		if got := r.FormValue("comment"); got != "initial upload" {
			t.Errorf("comment = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"id":"att9","type":"attachment","title":"hello.txt","_links":{}}],"start":0,"limit":25,"size":1,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	atts, err := c.Attachments.UploadAttachment("123", []string{file}, "initial upload")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(atts) != 1 || atts[0].ID != "att9" {
		t.Errorf("attachments = %+v", atts)
	}
}

func TestUploadAttachment_MultipleFiles(t *testing.T) {
	f1 := writeTempFile(t, "a.txt", "aaa")
	f2 := writeTempFile(t, "b.txt", "bbb")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := len(r.MultipartForm.File["file"]); got != 2 {
			t.Errorf("file parts = %d, want 2", got)
		}
		_, _ = fmt.Fprint(w, `{"results":[{"id":"1","_links":{}},{"id":"2","_links":{}}],"start":0,"limit":25,"size":2,"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	atts, err := c.Attachments.UploadAttachment("123", []string{f1, f2}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(atts) != 2 {
		t.Errorf("attachments = %d", len(atts))
	}
}

func TestUploadAttachment_FileMissing(t *testing.T) {
	c := newTestClient("https://confluence.example.com")
	_, err := c.Attachments.UploadAttachment("123", []string{"/no/such/file.bin"}, "")
	if err == nil || !strings.Contains(err.Error(), "opening file") {
		t.Errorf("err = %v", err)
	}
}

func TestUploadAttachment_DuplicateName400(t *testing.T) {
	file := writeTempFile(t, "dup.txt", "x")
	ts := statusServer(400, `{"statusCode":400,"message":"Cannot add a new attachment with same file name as an existing attachment: dup.txt"}`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Attachments.UploadAttachment("123", []string{file}, "")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_USAGE" {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "same file name") {
		t.Errorf("Message = %q", apiErr.Message)
	}
}

func TestUpdateAttachmentData(t *testing.T) {
	file := writeTempFile(t, "report.pdf", "v2 bytes")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/123/child/attachment/att1/data" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Atlassian-Token"); got != "no-check" {
			t.Errorf("X-Atlassian-Token = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"id":"att1","type":"attachment","title":"report.pdf","version":{"number":2},"_links":{}}`)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	atts, err := c.Attachments.UpdateAttachmentData("123", "att1", file, "new version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(atts) != 1 || atts[0].Version.Number != 2 {
		t.Errorf("attachments = %+v", atts)
	}
}

func TestDownloadAttachment_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/attachments/123/report.pdf" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-pat-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte("binary-bytes"))
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	rc, err := c.Attachments.DownloadAttachment("/download/attachments/123/report.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading stream: %v", err)
	}
	if string(body) != "binary-bytes" {
		t.Errorf("body = %q", body)
	}
}

func TestDownloadAttachment_EmptyLink(t *testing.T) {
	c := newTestClient("https://confluence.example.com")
	_, err := c.Attachments.DownloadAttachment("")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_NOT_FOUND" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestDownloadAttachment_404(t *testing.T) {
	ts := statusServer(404, `not found`)
	defer ts.Close()

	c := newTestClient(ts.URL)
	_, err := c.Attachments.DownloadAttachment("/download/attachments/gone.bin")
	apiErr := asAPIError(t, err)
	if apiErr.Code != "E_NOT_FOUND" {
		t.Errorf("Code = %q", apiErr.Code)
	}
}

func TestDeleteAttachment(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/rest/api/content/att1" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL)
	if err := c.Attachments.DeleteAttachment("att1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
