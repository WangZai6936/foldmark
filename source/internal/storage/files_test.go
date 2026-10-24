package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePreservesBOMAndCRLF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "中文 文档.md")
	if err := os.WriteFile(path, []byte("\ufeff# 折页\r\n第二行\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Content != "# 折页\n第二行\n" || !d.BOM || d.EOL != "CRLF" {
		t.Fatalf("wrong decoding: %+v", d)
	}
	d.Content = "# 已经编辑\n第三行\n"
	saved, err := Save(d, path, false)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "\ufeff# 已经编辑\r\n第三行\r\n" || saved.Dirty() {
		t.Fatal("encoding or saved baseline changed")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("temporary files were left behind")
	}
}

func TestExternalChangeAndMissingFileConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.md")
	os.WriteFile(path, []byte("old"), 0600)
	d, _ := Read(path)
	os.WriteFile(path, []byte("external"), 0600)
	d.Content = "mine"
	if _, err := Save(d, path, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "external" {
		t.Fatal("external content was overwritten")
	}
	saved, err := Save(d, path, true)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	if _, err := Save(saved, path, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("want deleted-file conflict: %v", err)
	}
}

func TestSaveAsAndInputLimits(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "a.md")
	copy := filepath.Join(root, "b.markdown")
	os.WriteFile(source, []byte("original"), 0600)
	d, _ := Read(source)
	d.Content = "copy"
	if _, err := Save(d, copy, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(source)
	if string(b) != "original" {
		t.Fatal("save-as changed original")
	}
	os.WriteFile(source, []byte{0xff, 0xfe, 0x41}, 0600)
	if _, err := Read(source); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	d.Content = strings.Repeat("a", MaxBytes+1)
	if _, err := Save(d, copy, false); err == nil {
		t.Fatal("oversized document accepted")
	}
}

func TestFolderAndSessionRecovery(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "章节"), 0700)
	os.Mkdir(filepath.Join(root, "node_modules"), 0700)
	path := filepath.Join(root, "章节", "一.MD")
	os.WriteFile(path, []byte("disk"), 0600)
	os.WriteFile(filepath.Join(root, "node_modules", "ignore.md"), []byte("skip"), 0600)
	files, truncated, err := Scan(root)
	if err != nil || truncated || len(files) != 1 {
		t.Fatalf("scan: %v %v %v", files, truncated, err)
	}
	d, _ := Read(path)
	d.Content = "unsaved"
	store := Store{Directory: filepath.Join(root, "state")}
	session := Session{Documents: []Document{d}, Settings: Settings{Mode: 1, FontSize: 16, Sidebar: true}}
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("external"), 0600)
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 1 || got.Documents[0].Content != "unsaved" || got.Documents[0].Fingerprint != d.Fingerprint {
		t.Fatal("recovery lost draft or conflict baseline")
	}
	if _, err := Save(got.Documents[0], path, false); !errors.Is(err, ErrConflict) {
		t.Fatal("recovered draft bypassed conflict detection")
	}
}
