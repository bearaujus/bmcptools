package user

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeImageDataURLAcceptsImageDataURL(t *testing.T) {
	mimeType, raw, err := decodeImageDataURL("", "data:image/png;BASE64,aGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	if mimeType != "image/png" {
		t.Fatalf("expected image/png MIME, got %q", mimeType)
	}
	if string(raw) != "hello" {
		t.Fatalf("expected decoded bytes, got %q", string(raw))
	}
}

func TestDecodeImageDataURLRejectsNonImages(t *testing.T) {
	if _, _, err := decodeImageDataURL("text/plain", "aGVsbG8="); err == nil {
		t.Fatal("expected non-image MIME to be rejected")
	}
}

func TestSaveDialogAttachmentsSanitizesAndDeDuplicatesNames(t *testing.T) {
	files, err := saveDialogAttachments([]dialogAttachmentPayload{
		{
			Name: filepath.Join("..", `same"name.png`),
			MIME: "image/png",
			Data: "data:image/png;base64,aGVsbG8=",
		},
		{
			Name: filepath.Join("..", `same"name.png`),
			MIME: "image/png",
			Data: "data:image/png;base64,d29ybGQ=",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two saved files, got %d", len(files))
	}
	defer os.RemoveAll(filepath.Dir(files[0].Path))

	if files[0].Name != "same_name.png" {
		t.Fatalf("expected sanitized first filename, got %q", files[0].Name)
	}
	if files[1].Name != "same_name-2.png" {
		t.Fatalf("expected de-duplicated second filename, got %q", files[1].Name)
	}
	if filepath.Dir(files[0].Path) != filepath.Dir(files[1].Path) {
		t.Fatal("expected attachments from one reply to share a temp directory")
	}
	for _, f := range files {
		if strings.ContainsAny(f.Name, `<>:"/\|?*`) {
			t.Fatalf("filename still contains unsafe characters: %q", f.Name)
		}
		if !strings.HasPrefix(f.Path, filepath.Dir(files[0].Path)+string(filepath.Separator)) {
			t.Fatalf("saved path escaped attachment directory: %q", f.Path)
		}
	}

	first, err := os.ReadFile(files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(files[1].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "hello" || string(second) != "world" {
		t.Fatalf("unexpected saved bytes: %q / %q", string(first), string(second))
	}
}

func TestSaveDialogAttachmentsSkipsDuplicateImageData(t *testing.T) {
	files, err := saveDialogAttachments([]dialogAttachmentPayload{
		{
			Name: "first.png",
			MIME: "image/png",
			Data: "data:image/png;base64,aGVsbG8=",
		},
		{
			Name: "same-bytes.png",
			MIME: "image/png",
			Data: "data:image/png;base64,aGVsbG8=",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected duplicate image data to be saved once, got %d files", len(files))
	}
	defer os.RemoveAll(filepath.Dir(files[0].Path))
	if files[0].Name != "first.png" {
		t.Fatalf("expected first attachment name to win, got %q", files[0].Name)
	}
}

func TestFormatDialogAnswerIncludesAttachmentPaths(t *testing.T) {
	answer := formatDialogAnswer("Use this", "With the screenshot.", []dialogAttachmentFile{{
		Name: "screenshot.png",
		MIME: "image/png",
		Size: 123,
		Path: filepath.Join("C:", "Temp", "screenshot.png"),
	}})
	for _, want := range []string{
		"Use this",
		"With the screenshot.",
		"Attached images (1):",
		"screenshot.png (image/png, 123 bytes)",
		"Local path:",
	} {
		if !strings.Contains(answer, want) {
			t.Fatalf("expected %q in formatted answer: %q", want, answer)
		}
	}
}
