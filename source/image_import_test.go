package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"foldmark/source/internal/content"
	"foldmark/source/internal/storage"
)

func TestImportImageKeepsMarkdownRelativeAndDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	documentPath := filepath.Join(root, "笔记.md")
	if err := os.WriteFile(documentPath, []byte("# 笔记"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "图标.png")
	image, err := os.ReadFile(filepath.Join("assets", "icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, image, 0600); err != nil {
		t.Fatal(err)
	}
	first, err := importImagePath(documentPath, outside)
	if err != nil {
		t.Fatal(err)
	}
	second, err := importImagePath(documentPath, outside)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "assets/") {
		t.Fatalf("expected distinct relative assets: %q, %q", first, second)
	}
	if _, err := content.ImagePath(documentPath, first); err != nil {
		t.Fatal(err)
	}
	reused, err := importImagePath(documentPath, filepath.Join(root, filepath.FromSlash(first)))
	if err != nil || reused != first {
		t.Fatalf("existing local image should be referenced directly: %q, %v", reused, err)
	}
	bad := append([]byte(nil), image...)
	bad[0] = 0
	if _, err := saveImage(documentPath, "fake.png", bad); err == nil {
		t.Fatal("accepted a non-image with a PNG extension")
	}
}

func TestClipboardImageImport(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "文档.md")
	if err := os.WriteFile(path, []byte("# 文档"), 0600); err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(filepath.Join("assets", "icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	app := newApplication(&storage.Store{Directory: filepath.Join(root, "state")})
	doc := newDocument(storage.New("文档.md", ""))
	doc.Path = path
	app.Documents = []*document{doc}
	service := &Desktop{app: app}
	relative, err := service.ImportImage(context.Background(), doc.ID, "截图.png", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(image))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := content.ImagePath(path, relative); err != nil {
		t.Fatal(err)
	}
}
