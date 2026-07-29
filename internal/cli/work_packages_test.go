package cli

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

// TestBuildAttachmentBody checks the wire format the OpenProject attachments
// endpoint requires: exactly two parts named "metadata" and "file", with the
// metadata part sent as application/json.
func TestBuildAttachmentBody(t *testing.T) {
	body, contentType, err := buildAttachmentBody(
		strings.NewReader("hello world"), "report.txt", "a description")
	if err != nil {
		t.Fatalf("buildAttachmentBody: %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse content type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Errorf("media type = %q, want multipart/form-data", mediaType)
	}

	mr := multipart.NewReader(body, params["boundary"])

	metaPart, err := mr.NextPart()
	if err != nil {
		t.Fatalf("read metadata part: %v", err)
	}
	if got := metaPart.FormName(); got != "metadata" {
		t.Errorf("first part name = %q, want metadata", got)
	}
	if got := metaPart.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("metadata content type = %q, want application/json", got)
	}

	metaRaw, err := io.ReadAll(metaPart)
	if err != nil {
		t.Fatalf("read metadata body: %v", err)
	}
	var meta struct {
		FileName    string `json:"fileName"`
		Description *struct {
			Raw string `json:"raw"`
		} `json:"description"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v", err)
	}
	if meta.FileName != "report.txt" {
		t.Errorf("fileName = %q, want report.txt", meta.FileName)
	}
	if meta.Description == nil || meta.Description.Raw != "a description" {
		t.Errorf("description = %+v, want raw %q", meta.Description, "a description")
	}

	filePart, err := mr.NextPart()
	if err != nil {
		t.Fatalf("read file part: %v", err)
	}
	if got := filePart.FormName(); got != "file" {
		t.Errorf("second part name = %q, want file", got)
	}
	if got := filePart.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("file content type = %q, want text/plain; charset=utf-8", got)
	}
	content, err := io.ReadAll(filePart)
	if err != nil {
		t.Fatalf("read file body: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("file content = %q, want %q", content, "hello world")
	}

	if _, err := mr.NextPart(); err != io.EOF {
		t.Errorf("expected exactly two parts, got a third (err=%v)", err)
	}
}

// TestBuildAttachmentBodyOmitsEmptyDescription verifies no description key is
// sent when the flag is unset, rather than an empty one.
func TestBuildAttachmentBodyOmitsEmptyDescription(t *testing.T) {
	body, contentType, err := buildAttachmentBody(strings.NewReader("x"), "a.bin", "")
	if err != nil {
		t.Fatalf("buildAttachmentBody: %v", err)
	}
	_, params, _ := mime.ParseMediaType(contentType)
	mr := multipart.NewReader(body, params["boundary"])

	metaPart, err := mr.NextPart()
	if err != nil {
		t.Fatalf("read metadata part: %v", err)
	}
	metaRaw, _ := io.ReadAll(metaPart)

	var meta map[string]interface{}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v", err)
	}
	if _, ok := meta["description"]; ok {
		t.Errorf("description present in metadata %s, want it omitted", metaRaw)
	}
}

func TestContentTypeFor(t *testing.T) {
	tests := []struct {
		fileName string
		want     string
	}{
		{"notes.txt", "text/plain; charset=utf-8"},
		{"diagram.png", "image/png"},
		{"noextension", "application/octet-stream"},
		{"archive.unknownext", "application/octet-stream"},
	}
	for _, tt := range tests {
		if got := contentTypeFor(tt.fileName); got != tt.want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", tt.fileName, got, tt.want)
		}
	}
}
