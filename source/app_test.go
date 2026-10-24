package main

import (
	"os"
	"path/filepath"
	"testing"

	"context"
	"foldmark/source/internal/storage"
)

type fakePlatform struct {
	path                        string
	open                        []string
	closeChoice, conflictChoice int
}

func (f *fakePlatform) Open(bool) ([]string, error)       { return f.open, nil }
func (f *fakePlatform) Save(string, bool) (string, error) { return f.path, nil }
func (f *fakePlatform) Confirm(string) (int, error)       { return f.closeChoice, nil }
func (f *fakePlatform) Conflict(string) (int, error)      { return f.conflictChoice, nil }

func testApp(t *testing.T) (*application, *fakePlatform) {
	t.Helper()
	a := newApplication(&storage.Store{Directory: t.TempDir()})
	a.Async = false
	f := &fakePlatform{closeChoice: 2, conflictChoice: 1}
	a.Platform = f
	a.restore(storage.Session{Settings: storage.Settings{Mode: 1, FontSize: 15, Sidebar: true}})
	return a, f
}

func TestOpenSaveConflictAndCloseDecisions(t *testing.T) {
	a, f := testApp(t)
	file := filepath.Join(t.TempDir(), "中文 路径.md")
	os.WriteFile(file, []byte("# 文件\n"), 0600)
	a.openPaths([]string{file})
	d := a.active()
	if d.Path != file {
		t.Fatal("failed to open path")
	}
	d.Content = "# 改过\n"
	a.changed(d)
	a.closeDocument(a.Active)
	if a.active() != d {
		t.Fatal("cancel did not retain tab")
	}
	a.save(false)
	b, _ := os.ReadFile(file)
	if string(b) != "# 改过\n" || d.Dirty() {
		t.Fatal("save failed")
	}
	os.WriteFile(file, []byte("external"), 0600)
	d.Content = "local"
	a.changed(d)
	a.save(false)
	b, _ = os.ReadFile(file)
	if string(b) != "external" || !d.Dirty() {
		t.Fatal("cancelled conflict lost data")
	}
	f.conflictChoice = 2
	a.save(false)
	b, _ = os.ReadFile(file)
	if string(b) != "local" || d.Dirty() {
		t.Fatal("explicit overwrite failed")
	}
	d.Content = "discard"
	a.changed(d)
	f.closeChoice = 1
	a.closeDocument(a.Active)
	b, _ = os.ReadFile(file)
	if string(b) != "local" {
		t.Fatal("discard wrote to disk")
	}
}

func TestSaveAsBlocksOtherOpenDocument(t *testing.T) {
	a, f := testApp(t)
	root := t.TempDir()
	one := filepath.Join(root, "1.md")
	two := filepath.Join(root, "2.md")
	os.WriteFile(one, []byte("one"), 0600)
	os.WriteFile(two, []byte("two"), 0600)
	a.openPaths([]string{one, two})
	f.path = one
	a.save(true)
	b, _ := os.ReadFile(one)
	if string(b) != "one" || !a.MessageError {
		t.Fatal("save-as overwrote an open document")
	}
}

func TestCloseWindowCancellationAndRecovery(t *testing.T) {
	a, f := testApp(t)
	a.create()
	d := a.active()
	d.Content = "# 恢复我\n"
	a.changed(d)
	destroyed := false
	a.OnDestroy = func() { destroyed = true }
	a.closeWindow()
	if destroyed {
		t.Fatal("cancel closed window")
	}
	if err := a.flush(a.snapshot()); err != nil {
		t.Fatal(err)
	}
	s, err := a.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	b := newApplication(a.Store)
	b.restore(s)
	if b.active().Content != "# 恢复我\n" || !b.active().Dirty() {
		t.Fatal("draft did not recover")
	}
	f.path = filepath.Join(t.TempDir(), "已恢复.md")
	f.closeChoice = 0
	a.closeWindow()
	if !destroyed {
		t.Fatal("successful save did not close window")
	}
	bytes, _ := os.ReadFile(f.path)
	if string(bytes) != "# 恢复我\n" {
		t.Fatal("save-before-close failed")
	}
}

func TestBridgeEditRevisionsAndDocumentReorder(t *testing.T) {
	a, _ := testApp(t)
	bridge := &Desktop{app: a}
	a.create()
	first := a.Documents[0].ID
	second := a.Documents[1].ID
	if _, err := bridge.Edit(context.Background(), []EditRequest{{ID: second, Content: "# 新稿", Revision: 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Edit(context.Background(), []EditRequest{{ID: second, Content: "旧内容", Revision: 1}}); err != nil {
		t.Fatal(err)
	}
	if a.active().Content != "# 新稿" {
		t.Fatal("stale edit replaced new content")
	}
	result, err := bridge.Reorder(context.Background(), []string{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if result.Documents[0].ID != second || result.ActiveID != second {
		t.Fatal("reorder lost active document")
	}
}
