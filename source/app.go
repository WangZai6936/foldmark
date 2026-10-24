package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"foldmark/source/internal/content"
	"foldmark/source/internal/storage"
	"github.com/egoist/mygo"
)

type document struct {
	storage.Document
	Revision int `json:"revision"`
}

func newDocument(data storage.Document) *document {
	return &document{Document: data}
}

type platform interface {
	Open(bool) ([]string, error)
	Save(string, bool) (string, error)
	Confirm(string) (int, error)
	Conflict(string) (int, error)
}

type desktopPlatform struct{ window *mygo.Window }

var markdownFilters = []mygo.FileFilter{{Name: "Markdown", Extensions: []string{"md", "markdown", "mdown"}}}

func (p desktopPlatform) Open(folder bool) ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: p.window, Title: "打开 Markdown", Directory: folder, Multiple: !folder, Filters: markdownFilters})
}
func (p desktopPlatform) Save(path string, html bool) (string, error) {
	filters := markdownFilters
	if html {
		filters = []mygo.FileFilter{{Name: "HTML", Extensions: []string{"html"}}}
	}
	return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: p.window, Title: "保存文档", DefaultPath: path, Filters: filters})
}
func (p desktopPlatform) Confirm(name string) (int, error) {
	r, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: p.window, Type: mygo.MessageQuestion, Title: "保存更改", Message: fmt.Sprintf("要保存“%s”的更改吗？", name), Detail: "未保存的修改将被丢弃。", Buttons: []string{"保存", "不保存", "取消"}, DefaultButton: 0, CancelButton: 2})
	return r.Button, e
}
func (p desktopPlatform) Conflict(name string) (int, error) {
	r, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: p.window, Type: mygo.MessageWarning, Title: "文件已在外部修改", Message: fmt.Sprintf("“%s”已被其他程序修改。", name), Detail: "另存副本可保留两个版本。", Buttons: []string{"另存副本", "取消", "覆盖磁盘文件"}, DefaultButton: 0, CancelButton: 1})
	return r.Button, e
}

type application struct {
	Window                 *mygo.Window
	Platform               platform
	Store                  *storage.Store
	Documents              []*document
	Recent                 []storage.File
	Files                  []storage.File
	Folder                 string
	Active                 int
	Mode                   int
	FontSize               float32
	Sidebar                bool
	SideTab                int
	Search                 string
	Focus                  bool
	PreviousMode           int
	SettingsOpen, HelpOpen bool
	Busy                   bool
	Message                string
	MessageError           bool
	Pending                []string
	Dispatch               func(func())
	Async                  bool
	SessionQueue           chan storage.Session
	FlushQueue             chan flushRequest
	OnDestroy              func()
	OnChange               func()
}

type flushRequest struct {
	Data   storage.Session
	Result chan error
}

func newApplication(store *storage.Store) *application {
	a := &application{Store: store, Mode: 1, FontSize: 15, Sidebar: true, Async: true, Dispatch: func(fn func()) { fn() }}
	return a
}

func (a *application) active() *document {
	if a.Active < 0 || a.Active >= len(a.Documents) {
		return nil
	}
	return a.Documents[a.Active]
}
func (a *application) run(fn func()) {
	if a.Async {
		go fn()
	} else {
		fn()
	}
}
func (a *application) update(fn func()) {
	a.Dispatch(func() { fn(); a.notify() })
}
func (a *application) notify() {
	if a.OnChange != nil {
		a.OnChange()
	}
}
func (a *application) say(message string, err bool) { a.Message, a.MessageError = message, err }

func (a *application) restore(s storage.Session) {
	a.Mode, a.FontSize, a.Sidebar, a.Folder, a.Recent = s.Settings.Mode, s.Settings.FontSize, s.Settings.Sidebar, s.Folder, s.Recent
	for _, saved := range s.Documents {
		d := newDocument(saved)
		a.Documents = append(a.Documents, d)
		if d.ID == s.Settings.ActiveID {
			a.Active = len(a.Documents) - 1
		}
		if d.Dirty() {
			a.say("已恢复上次未保存的草稿。", false)
		}
	}
	if len(a.Documents) == 0 {
		a.Documents = []*document{newDocument(storage.New("欢迎使用.md", welcome))}
	}
}

func (a *application) snapshot() storage.Session {
	s := storage.Session{Recent: append([]storage.File(nil), a.Recent...), Folder: a.Folder, Settings: storage.Settings{Mode: a.Mode, FontSize: a.FontSize, Sidebar: a.Sidebar}}
	for _, d := range a.Documents {
		s.Documents = append(s.Documents, d.Document)
	}
	if d := a.active(); d != nil {
		s.Settings.ActiveID = d.ID
	}
	return s
}

func (a *application) startWriter() {
	a.SessionQueue = make(chan storage.Session, 1)
	a.FlushQueue = make(chan flushRequest)
	go func() {
		var latest storage.Session
		var timer *time.Timer
		var tick <-chan time.Time
		for {
			select {
			case latest = <-a.SessionQueue:
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(600 * time.Millisecond)
				tick = timer.C
			case <-tick:
				tick = nil
				if err := a.Store.Save(latest); err != nil {
					a.update(func() { a.say("草稿恢复文件写入失败："+err.Error(), true) })
				}
			case request := <-a.FlushQueue:
				if timer != nil {
					timer.Stop()
				}
				tick = nil
				select {
				case <-a.SessionQueue:
				default:
				}
				request.Result <- a.Store.Save(request.Data)
			}
		}
	}()
}

func (a *application) persist() {
	if a.SessionQueue == nil {
		return
	}
	s := a.snapshot()
	select {
	case a.SessionQueue <- s:
	default:
		select {
		case <-a.SessionQueue:
		default:
		}
		a.SessionQueue <- s
	}
}
func (a *application) flush(s storage.Session) error {
	if a.FlushQueue == nil {
		return a.Store.Save(s)
	}
	request := flushRequest{s, make(chan error, 1)}
	a.FlushQueue <- request
	return <-request.Result
}

func (a *application) changed(d *document) {
	a.persist()
}

func (a *application) create() {
	if a.Busy {
		return
	}
	if len(a.Documents) >= 30 {
		a.say("最多同时打开 30 份文档，请先关闭部分标签。", true)
		return
	}
	d := newDocument(storage.New(fmt.Sprintf("未命名%d.md", len(a.Documents)+1), ""))
	a.Documents = append(a.Documents, d)
	a.Active = len(a.Documents) - 1
	if a.Mode == 2 {
		a.Mode = 0
	}
	a.persist()
}

func (a *application) remember(path string) {
	recent := []storage.File{{Path: path, Name: filepath.Base(path)}}
	for _, f := range a.Recent {
		if !strings.EqualFold(f.Path, path) && len(recent) < 15 {
			recent = append(recent, f)
		}
	}
	a.Recent = recent
}

func (a *application) finish() {
	a.Busy = false
	if len(a.Pending) > 0 {
		paths := a.Pending
		a.Pending = nil
		a.openPaths(paths)
	}
}

func (a *application) openPaths(paths []string) {
	if a.Busy {
		a.Pending = append(a.Pending, paths...)
		return
	}
	a.Busy = true
	a.run(func() {
		var opened []storage.Document
		var lastErr error
		for _, path := range paths {
			if !storage.IsMarkdown(path) {
				continue
			}
			d, err := storage.Read(path)
			if err != nil {
				lastErr = err
			} else {
				opened = append(opened, d)
			}
		}
		a.update(func() {
			for _, data := range opened {
				found := -1
				for i, d := range a.Documents {
					if strings.EqualFold(d.Path, data.Path) {
						found = i
						break
					}
				}
				if found >= 0 {
					a.Active = found
				} else if len(a.Documents) < 30 {
					a.Documents = append(a.Documents, newDocument(data))
					a.Active = len(a.Documents) - 1
				} else {
					a.say("已达到 30 个标签的上限。", true)
					break
				}
				a.remember(data.Path)
			}
			if lastErr != nil {
				a.say(lastErr.Error(), true)
			}
			a.persist()
			a.finish()
		})
	})
}

func (a *application) openDialog(folder bool) {
	if a.Busy || a.Platform == nil {
		return
	}
	a.Busy = true
	a.run(func() {
		paths, err := a.Platform.Open(folder)
		var files []storage.File
		truncated := false
		if err == nil && folder && len(paths) > 0 {
			files, truncated, err = storage.Scan(paths[0])
		}
		a.update(func() {
			a.Busy = false
			if err != nil {
				a.say(err.Error(), true)
			} else if len(paths) > 0 {
				if folder {
					a.Files = files
					a.Folder = paths[0]
					a.SideTab = 0
					a.Search = ""
					if truncated {
						a.say("文件夹较大，已显示前 1500 份文档（最多扫描 8 层）。", false)
					}
					a.persist()
				} else {
					a.openPaths(paths)
				}
			}
		})
	})
}

// saveData runs off the UI thread against a stable snapshot of the document.
func (a *application) saveData(doc storage.Document, saveAs bool, paths map[string]string) (storage.Document, bool, error) {
	destination := doc.Path
	if destination == "" || saveAs {
		defaultPath := destination
		if defaultPath == "" {
			defaultPath = doc.Name
		}
		if saveAs {
			ext := filepath.Ext(defaultPath)
			defaultPath = strings.TrimSuffix(defaultPath, ext) + " - 副本" + ext
		}
		chosen, err := a.Platform.Save(defaultPath, false)
		if err != nil || chosen == "" {
			return doc, false, err
		}
		if !storage.IsMarkdown(chosen) {
			chosen += ".md"
		}
		destination = chosen
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return doc, false, err
	}
	if id := paths[strings.ToLower(absolute)]; id != "" && id != doc.ID {
		return doc, false, errors.New("该文件已在其他标签打开，请切换到那个标签编辑")
	}
	saved, err := storage.Save(doc, destination, false)
	if errors.Is(err, storage.ErrConflict) {
		choice, dialogErr := a.Platform.Conflict(doc.Name)
		if dialogErr != nil {
			return doc, false, dialogErr
		}
		switch choice {
		case 0:
			return a.saveData(doc, true, paths)
		case 1:
			return doc, false, nil
		case 2:
			saved, err = storage.Save(doc, destination, true)
		default:
			return doc, false, nil
		}
	}
	return saved, err == nil, err
}

func (a *application) pathIDs() map[string]string {
	result := map[string]string{}
	for _, d := range a.Documents {
		if d.Path != "" {
			result[strings.ToLower(d.Path)] = d.ID
		}
	}
	return result
}

func (a *application) applySaved(saved storage.Document) {
	for _, d := range a.Documents {
		if d.ID == saved.ID {
			text := d.Content
			d.Document = saved
			d.Content = text
			break
		}
	}
	a.remember(saved.Path)
}

func (a *application) save(saveAs bool) {
	d := a.active()
	if d == nil || a.Busy || a.Platform == nil {
		return
	}
	a.changed(d)
	doc, paths := d.Document, a.pathIDs()
	a.Busy = true
	a.run(func() {
		saved, ok, err := a.saveData(doc, saveAs, paths)
		a.update(func() {
			if err != nil {
				a.say(err.Error(), true)
			} else if ok {
				a.applySaved(saved)
				a.say("已保存 "+saved.Name, false)
				a.persist()
			}
			a.finish()
		})
	})
}

func (a *application) closeDocument(index int) {
	if index < 0 || index >= len(a.Documents) || a.Busy || a.Platform == nil {
		return
	}
	d := a.Documents[index]
	a.changed(d)
	doc, paths := d.Document, a.pathIDs()
	a.Busy = true
	a.run(func() {
		ok, wasSaved := true, false
		saved := doc
		var err error
		if doc.Dirty() {
			var choice int
			choice, err = a.Platform.Confirm(doc.Name)
			if err != nil || choice == 2 {
				ok = false
			} else if choice == 0 {
				saved, ok, err = a.saveData(doc, false, paths)
				wasSaved = ok
			}
		}
		a.update(func() {
			if err != nil {
				a.say(err.Error(), true)
			}
			if ok {
				if wasSaved {
					a.remember(saved.Path)
				}
				a.Documents = append(a.Documents[:index], a.Documents[index+1:]...)
				if a.Active > index {
					a.Active--
				} else if a.Active >= len(a.Documents) {
					a.Active = len(a.Documents) - 1
				}
				a.persist()
			}
			a.finish()
		})
	})
}

func (a *application) closeWindow() {
	if a.Busy || a.Platform == nil {
		return
	}
	a.Busy = true
	snapshot, paths := a.snapshot(), a.pathIDs()
	a.run(func() {
		var next []storage.Document
		var savedDocs []storage.Document
		var err error
		cancelled := false
		for _, doc := range snapshot.Documents {
			if doc.Dirty() {
				choice, dialogErr := a.Platform.Confirm(doc.Name)
				if dialogErr != nil {
					err = dialogErr
					break
				}
				if choice == 2 {
					cancelled = true
					break
				}
				if choice == 0 {
					saved, ok, saveErr := a.saveData(doc, false, paths)
					if saveErr != nil {
						err = saveErr
						break
					}
					if !ok {
						cancelled = true
						break
					}
					doc = saved
					savedDocs = append(savedDocs, saved)
					paths[strings.ToLower(saved.Path)] = saved.ID
				} else {
					if doc.Path == "" {
						continue
					}
					doc.Content = doc.Baseline
				}
			}
			next = append(next, doc)
		}
		if err == nil && !cancelled {
			snapshot.Documents = next
			err = a.flush(snapshot)
		}
		a.update(func() {
			for _, saved := range savedDocs {
				a.applySaved(saved)
			}
			if err != nil {
				a.say(err.Error(), true)
			} else if !cancelled {
				if a.OnDestroy != nil {
					a.OnDestroy()
				}
				return
			}
			a.persist()
			a.finish()
		})
	})
}

func (a *application) exportHTML() {
	d := a.active()
	if d == nil || a.Busy || a.Platform == nil {
		return
	}
	a.changed(d)
	doc := d.Document
	a.Busy = true
	a.run(func() {
		text, err := content.Export(doc.Name, doc.Content, doc.Path)
		var target string
		if err == nil {
			target, err = a.Platform.Save(strings.TrimSuffix(doc.Name, filepath.Ext(doc.Name))+".html", true)
		}
		if err == nil && target != "" {
			if !strings.HasSuffix(strings.ToLower(target), ".html") {
				target += ".html"
			}
			err = storage.AtomicWrite(target, []byte(text))
		}
		a.update(func() {
			if err != nil {
				a.say(err.Error(), true)
			} else if target != "" {
				a.say("已导出 HTML 文档。", false)
			}
			a.finish()
		})
	})
}

func defaultStore() (*storage.Store, error) {
	dir := os.Getenv("FOLDMARK_DATA_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(base, "Foldmark")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &storage.Store{Directory: dir}, nil
}
