package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"foldmark/source/internal/content"
	"foldmark/source/internal/storage"
	"github.com/egoist/mygo"
)

var StateChanged = mygo.NewEvent[ViewState]("state:changed")
var CloseRequested = mygo.NewEvent[bool]("close:requested")
var ImageDropped = mygo.NewEvent[string]("image:dropped")

// ViewState is the serializable desktop state. Editing revisions protect newer
// frontend text when a slower save or file-open operation completes.
type ViewState struct {
	Documents    []document       `json:"documents"`
	Recent       []storage.File   `json:"recent"`
	Files        []storage.File   `json:"files"`
	Folder       string           `json:"folder"`
	ActiveID     string           `json:"activeId"`
	Settings     storage.Settings `json:"settings"`
	Busy         bool             `json:"busy"`
	Message      string           `json:"message"`
	MessageError bool             `json:"messageError"`
}

type EditRequest struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Revision int    `json:"revision"`
}

// Desktop exposes a small typed bridge; application state stays on MyGO's
// main thread, while native dialogs and disk operations use the existing worker.
type Desktop struct {
	app   *application
	ready bool
}

func (a *application) viewState() ViewState {
	s := ViewState{Recent: append([]storage.File{}, a.Recent...), Files: append([]storage.File{}, a.Files...), Folder: a.Folder, Settings: a.snapshot().Settings, Busy: a.Busy, Message: a.Message, MessageError: a.MessageError}
	for _, d := range a.Documents {
		s.Documents = append(s.Documents, *d)
	}
	if d := a.active(); d != nil {
		s.ActiveID = d.ID
	}
	return s
}

func (d *Desktop) onMain(ctx context.Context, fn func() error) (ViewState, error) {
	type reply struct {
		state ViewState
		err   error
	}
	result := make(chan reply, 1)
	d.app.Dispatch(func() { err := fn(); result <- reply{d.app.viewState(), err} })
	select {
	case r := <-result:
		return r.state, r.err
	case <-ctx.Done():
		return ViewState{}, ctx.Err()
	}
}

func (d *Desktop) State(ctx context.Context) (ViewState, error) {
	return d.onMain(ctx, func() error { d.ready = true; return nil })
}

func (d *Desktop) Edit(ctx context.Context, edits []EditRequest) (ViewState, error) {
	return d.onMain(ctx, func() error {
		for _, edit := range edits {
			if len(edit.Content) > storage.MaxBytes || !utf8.ValidString(edit.Content) || strings.ContainsRune(edit.Content, 0) {
				return errors.New("文档必须是不超过 8 MB 的 UTF-8 文本")
			}
		}
		for _, edit := range edits {
			for _, doc := range d.app.Documents {
				if doc.ID == edit.ID && edit.Revision > doc.Revision {
					doc.Content, doc.Revision = edit.Content, edit.Revision
				}
			}
		}
		d.app.persist()
		return nil
	})
}

func (d *Desktop) Command(ctx context.Context, name, id, argument string) (ViewState, error) {
	return d.onMain(ctx, func() error {
		a := d.app
		if a.Busy {
			return errors.New("正在处理文件，请稍候")
		}
		index := a.Active
		if id != "" {
			index = -1
			for i, doc := range a.Documents {
				if doc.ID == id {
					index = i
					break
				}
			}
			if index == -1 {
				return errors.New("文档已关闭")
			}
		}
		switch name {
		case "new":
			a.create()
		case "open":
			a.openDialog(false)
		case "folder":
			a.openDialog(true)
		case "open-path":
			a.openPaths([]string{argument})
		case "open-relative":
			base := a.Documents[index].Path
			if base == "" {
				return errors.New("请先保存文档，再打开相对链接")
			}
			uri, err := url.Parse(argument)
			if err != nil || uri.IsAbs() || uri.Host != "" || filepath.IsAbs(uri.Path) {
				return errors.New("文档链接无效")
			}
			target := filepath.Join(filepath.Dir(base), filepath.FromSlash(uri.Path))
			if !storage.IsMarkdown(target) {
				return errors.New("链接不是 Markdown 文档")
			}
			a.openPaths([]string{target})
		case "activate":
			a.Active = index
			a.persist()
		case "save", "save-as":
			a.Active = index
			a.save(name == "save-as")
		case "close":
			a.closeDocument(index)
		case "close-window":
			a.closeWindow()
		case "dismiss":
			a.Message = ""
		case "export":
			a.Active = index
			a.exportHTML()
		default:
			return errors.New("不支持的操作")
		}
		return nil
	})
}

func (d *Desktop) Preferences(ctx context.Context, mode int, fontSize float32, sidebar bool) (ViewState, error) {
	return d.onMain(ctx, func() error {
		if mode < 0 || mode > 2 || fontSize < 12 || fontSize > 22 {
			return errors.New("设置值无效")
		}
		d.app.Mode, d.app.FontSize, d.app.Sidebar = mode, fontSize, sidebar
		d.app.persist()
		return nil
	})
}

func (d *Desktop) Reorder(ctx context.Context, ids []string) (ViewState, error) {
	return d.onMain(ctx, func() error {
		a := d.app
		if a.Busy || len(ids) != len(a.Documents) {
			return errors.New("文档列表已变化，请重试")
		}
		byID := map[string]*document{}
		for _, doc := range a.Documents {
			byID[doc.ID] = doc
		}
		ordered := make([]*document, 0, len(ids))
		activeID := ""
		if current := a.active(); current != nil {
			activeID = current.ID
		}
		for _, id := range ids {
			doc, ok := byID[id]
			if !ok {
				return errors.New("文档顺序无效")
			}
			ordered = append(ordered, doc)
			delete(byID, id)
		}
		a.Documents = ordered
		for i, doc := range ordered {
			if doc.ID == activeID {
				a.Active = i
			}
		}
		a.persist()
		return nil
	})
}

func (d *Desktop) OpenLink(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto") {
		return errors.New("仅支持网页和邮件链接")
	}
	return mygo.Shell.OpenExternal(raw)
}

func (d *Desktop) serveImage(w http.ResponseWriter, r *http.Request) {
	id, raw := r.URL.Query().Get("document"), r.URL.Query().Get("src")
	var documentPath string
	_, err := d.onMain(r.Context(), func() error {
		for _, doc := range d.app.Documents {
			if doc.ID == id {
				documentPath = doc.Path
				break
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, "Document unavailable", 404)
		return
	}
	file, err := content.ImagePath(documentPath, raw)
	if err != nil {
		http.Error(w, "Image unavailable", 404)
		return
	}
	bytes, err := os.ReadFile(file)
	if err != nil {
		http.Error(w, "Image unavailable", 404)
		return
	}
	w.Header().Set("Content-Type", map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp"}[strings.ToLower(filepath.Ext(file))])
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(bytes)
}
