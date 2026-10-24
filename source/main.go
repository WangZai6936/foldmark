package main

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"foldmark/source/internal/storage"
	_ "foldmark/source/locales"
	"github.com/egoist/mygo"
)

//go:embed welcome.md
var welcome string

func main() {
	store, err := defaultStore()
	if err != nil {
		log.Fatal(err)
	}
	appName := "Foldmark"
	if os.Getenv("FOLDMARK_CAPTURE_DIR") != "" && os.Getenv("FOLDMARK_DATA_DIR") != "" {
		appName = "Foldmark Capture"
	}
	mygo.App.SetName(appName)
	mygo.App.SetPath(mygo.PathUserData, store.Directory)
	mygo.App.SetLocale("zh-Hans")
	mygo.Theme.SetSource(mygo.ThemeLight)
	a := newApplication(store)
	service := &Desktop{app: a}
	mygo.Bind(service)
	mygo.Protocol.HandleFunc("images", service.serveImage)
	mygo.App.WhenReady(func() {
		if !mygo.App.RequestSingleInstanceLock() {
			mygo.App.Quit()
			return
		}
		saved, loadErr := store.Load()
		a.restore(saved)
		if loadErr != nil {
			a.say("上次会话无法读取："+loadErr.Error(), true)
		}
		a.Window = mygo.NewWindow(mygo.WindowOptions{Title: "折页 Foldmark", URL: "/", Width: 1200, Height: 820, MinWidth: 820, MinHeight: 600, StateKey: "workspace-web", Hidden: true, TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 42})
		a.Dispatch = a.Window.Update
		a.Platform = desktopPlatform{a.Window}
		a.OnDestroy = func() { a.Window.Destroy() }
		a.OnChange = func() { StateChanged.Emit(a.Window, a.viewState()) }
		a.Window.OnClose(func(e *mygo.CloseEvent) {
			e.PreventDefault()
			if service.ready {
				CloseRequested.Emit(a.Window, true)
			} else {
				a.closeWindow()
			}
		})
		a.Window.OnFileDrop(func(e *mygo.FileDropEvent) {
			var markdownPaths []string
			var imagePath string
			for _, path := range e.Paths {
				if storage.IsMarkdown(path) {
					markdownPaths = append(markdownPaths, path)
				} else if imagePath == "" && imageExtension(path) != "" {
					imagePath = path
				}
			}
			if len(markdownPaths) > 0 {
				a.openPaths(markdownPaths)
				a.notify()
			} else if imagePath != "" {
				ImageDropped.Emit(a.Window, imagePath)
			}
		})
		a.Window.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			// Links in Markdown open in the system browser; the app shell stays local.
			if !strings.HasPrefix(e.URL, "http://mygo.localhost/") && !strings.HasPrefix(e.URL, "mygo://localhost/") && !strings.HasPrefix(e.URL, "http://localhost:5173/") {
				e.PreventDefault()
			}
		})
		a.startWriter()
		if a.Folder != "" {
			folder := a.Folder
			go func() {
				files, _, err := storage.Scan(folder)
				a.update(func() {
					if err == nil {
						a.Files = files
					}
				})
			}()
		}
		if len(a.Pending) > 0 {
			paths := a.Pending
			a.Pending = nil
			a.openPaths(paths)
		}
		a.Window.Show()
		if dir := os.Getenv("FOLDMARK_CAPTURE_DIR"); dir != "" {
			nativeCapture(a, dir)
		}
	})
	mygo.App.OnOpenFile(func(path string) {
		if a.Window == nil {
			a.Pending = append(a.Pending, path)
		} else {
			a.openPaths([]string{path})
			a.notify()
		}
	})
	mygo.App.OnSecondInstance(func(_ []string, _ string) {
		if a.Window != nil {
			a.Window.Restore()
			a.Window.Show()
			a.Window.Focus()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// Opt-in verification uses isolated preferences and MyGO's own page capture.
func nativeCapture(a *application, dir string) {
	go func() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Print(err)
			mygo.App.Exit(1)
			return
		}
		for i := 0; i < 30; i++ {
			time.Sleep(300 * time.Millisecond)
			ready, err := a.Window.Page().Eval("document.documentElement.dataset.ready === 'true'")
			if err == nil && ready == true {
				break
			}
			if i == 29 {
				log.Print("frontend did not become ready")
				mygo.App.Exit(1)
				return
			}
		}
		for _, capture := range []struct {
			name          string
			width, height int
		}{{"foldmark-native", 1440, 920}, {"foldmark-compact", 820, 740}} {
			a.Window.SetSize(capture.width, capture.height)
			time.Sleep(650 * time.Millisecond)
			bytes, err := a.Window.CapturePage()
			if err == nil {
				err = os.WriteFile(filepath.Join(dir, capture.name+".png"), bytes, 0600)
			}
			if err != nil {
				log.Print(err)
				mygo.App.Exit(1)
				return
			}
		}
		for i := 0; i < 10; i++ {
			loaded, err := a.Window.Page().Eval(`!document.querySelector('.reading-paper img') || document.querySelector('.reading-paper img').naturalWidth > 0`)
			if err == nil && loaded == true {
				break
			}
			if i == 9 {
				log.Print("local image did not load")
				mygo.App.Exit(1)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		if os.Getenv("FOLDMARK_CAPTURE_READING") == "1" {
			if err := captureReading(a, dir); err != nil {
				log.Print(err)
				mygo.App.Exit(1)
				return
			}
		}
		if os.Getenv("FOLDMARK_CAPTURE_SLASH") == "1" {
			_, _ = a.Window.Page().Eval(`(() => { const tab = [...document.querySelectorAll('.mode-switch button')].find(x => x.textContent?.includes('源码')); tab?.click(); return !!tab })()`)
			time.Sleep(250 * time.Millisecond)
			inserted, err := a.Window.Page().Eval(`(() => { const editor = document.querySelector('.cm-content'); if (!editor) return false; editor.focus(); return document.execCommand('insertText', false, '/'); })()`)
			if err != nil || inserted != true {
				if bytes, captureErr := a.Window.CapturePage(); captureErr == nil {
					_ = os.WriteFile(filepath.Join(dir, "foldmark-slash-fail.png"), bytes, 0600)
				}
				log.Print("slash insertion test could not type in the editor")
				mygo.App.Exit(1)
				return
			}
			for i := 0; i < 15; i++ {
				time.Sleep(100 * time.Millisecond)
				visible, err := a.Window.Page().Eval(`!!document.querySelector('.cm-tooltip-autocomplete li')`)
				if err == nil && visible == true {
					bytes, err := a.Window.CapturePage()
					if err == nil {
						err = os.WriteFile(filepath.Join(dir, "foldmark-slash.png"), bytes, 0600)
					}
					if err != nil {
						log.Print(err)
						mygo.App.Exit(1)
						return
					}
					break
				}
				if i == 14 {
					if bytes, captureErr := a.Window.CapturePage(); captureErr == nil {
						_ = os.WriteFile(filepath.Join(dir, "foldmark-slash-fail.png"), bytes, 0600)
					}
					log.Print("slash menu did not open")
					mygo.App.Exit(1)
					return
				}
			}
		}
		if os.Getenv("FOLDMARK_CAPTURE_TABLE") == "1" {
			if err := captureTable(a, dir); err != nil {
				log.Print(err)
				mygo.App.Exit(1)
				return
			}
		}
		a.Window.Update(func() { a.Window.Destroy() })
	}()
}

func captureTable(a *application, dir string) error {
	page := a.Window.Page()
	if ok, err := page.Eval(`(() => { const tab=[...document.querySelectorAll('.mode-switch button')].find(x=>x.textContent?.trim()==='编辑'); tab?.click(); return !!tab })()`); err != nil || ok != true {
		return fmt.Errorf("visual editor did not open: %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	if ok, err := page.Eval(`(() => { const button=[...document.querySelectorAll('.tool-menu-trigger')].find(x=>x.textContent?.includes('插入')); button?.click(); return !!button })()`); err != nil || ok != true {
		return fmt.Errorf("insert menu did not open: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if ok, err := page.Eval(`(() => { const item=[...document.querySelectorAll('[role=menuitem]')].find(x=>x.textContent?.includes('表格 · 3 × 3')); item?.click(); return !!item })()`); err != nil || ok != true {
		return fmt.Errorf("table menu item missing: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if ok, err := page.Eval(`(() => { const cell=document.querySelector('.visual-content table th'); if (!cell) return false; const range=document.createRange(); range.selectNodeContents(cell); range.collapse(true); const selection=window.getSelection(); selection.removeAllRanges(); selection.addRange(range); document.querySelector('.visual-content')?.focus(); return document.execCommand('insertText',false,'项目') })()`); err != nil || ok != true {
		return fmt.Errorf("visual table cell could not be edited: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if ok, err := page.Eval(`(() => { const action=[...document.querySelectorAll('.table-actions button')].find(x=>x.textContent?.includes('下方插入行')); if (!action) return false; action.click(); return true })()`); err != nil || ok != true {
		return fmt.Errorf("table row action missing: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if ok, err := page.Eval(`document.querySelectorAll('.visual-content table tr').length === 4 && document.querySelector('.visual-content table th')?.textContent?.includes('项目')`); err != nil || ok != true {
		return fmt.Errorf("table row or cell content failed: %v", err)
	}
	if bytes, err := a.Window.CapturePage(); err != nil {
		return err
	} else if err := os.WriteFile(filepath.Join(dir, "foldmark-table.png"), bytes, 0600); err != nil {
		return err
	}
	if ok, err := page.Eval(`(() => { const tab=[...document.querySelectorAll('.mode-switch button')].find(x=>x.textContent?.includes('源码')); tab?.click(); return !!tab })()`); err != nil || ok != true {
		return fmt.Errorf("source view did not open: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if ok, err := page.Eval(`(() => { const source=document.querySelector('.cm-content')?.textContent || ''; return source.includes('项目') && source.includes('---') && source.includes('|') })()`); err != nil || ok != true {
		content, _ := page.Eval(`document.querySelector('.cm-content')?.textContent?.slice(0, 500)`)
		return fmt.Errorf("edited table did not serialize to Markdown: %v, source=%v", err, content)
	}
	if ok, err := page.Eval(`(() => { const tab=[...document.querySelectorAll('.mode-switch button')].find(x=>x.textContent?.trim()==='编辑'); tab?.click(); return !!tab })()`); err != nil || ok != true {
		return fmt.Errorf("visual editor did not reopen: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if ok, err := page.Eval(`document.querySelectorAll('.visual-content table tr').length === 4 && document.querySelector('.visual-content table th')?.textContent?.includes('项目')`); err != nil || ok != true {
		return fmt.Errorf("Markdown table did not roundtrip to visual cells: %v", err)
	}
	if ok, err := page.Eval(`(() => { const button=[...document.querySelectorAll('.document-bar-actions button')].find(x=>x.textContent?.trim()==='保存'); button?.click(); return !!button })()`); err != nil || ok != true {
		return fmt.Errorf("save button did not submit table edits: %v", err)
	}
	for attempt := 0; attempt < 20; attempt++ {
		time.Sleep(150 * time.Millisecond)
		b, err := os.ReadFile(filepath.Join(dir, "第二份 文档.md"))
		if err == nil && strings.Contains(string(b), "项目") && strings.Contains(string(b), "| ---") {
			return nil
		}
	}
	return errors.New("edited visual table was not saved as a Markdown table")
}

func captureReading(a *application, dir string) error {
	page := a.Window.Page()
	if ok, err := page.Eval(`(() => { const tab = [...document.querySelectorAll('.mode-switch button')].find(x => x.textContent?.includes('预览')); if (!tab) return false; tab.click(); return true })()`); err != nil || ok != true {
		return fmt.Errorf("preview mode did not open: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if ok, err := page.Eval(`(() => { const button = document.querySelector('[aria-label="在预览中查找"]'); if (!button) return false; button.click(); return true })()`); err != nil || ok != true {
		return fmt.Errorf("preview search did not open: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if ok, err := page.Eval(`(() => { const input = document.querySelector('.preview-search input'); if (!input) return false; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, '再次'); input.dispatchEvent(new Event('input', {bubbles:true})); return true })()`); err != nil || ok != true {
		return fmt.Errorf("preview search could not type: %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	if result, err := page.Eval(`(() => ({count:document.querySelector('.preview-search-count')?.textContent, highlights:CSS.highlights?.size || 0, image:!!document.querySelector('.preview-image-open')}))()`); err != nil {
		return fmt.Errorf("preview search failed: %w", err)
	} else {
		log.Printf("reading check: %v", result)
	}
	if ok, err := page.Eval(`!!document.querySelector('.preview-search-count')?.textContent?.includes(' / ') && CSS.highlights?.size >= 2`); err != nil || ok != true {
		return fmt.Errorf("preview search did not highlight the result: %v", err)
	}
	if bytes, err := a.Window.CapturePage(); err != nil {
		return err
	} else if err := os.WriteFile(filepath.Join(dir, "foldmark-reading-search.png"), bytes, 0600); err != nil {
		return err
	}
	if ok, err := page.Eval(`(() => { const code=document.querySelector('.preview-code-block'); if (!code?.querySelector('[aria-label="复制代码"]')) return false; code.scrollIntoView({block:'center'}); return true })()`); err != nil || ok != true {
		return fmt.Errorf("code copy action missing: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if bytes, err := a.Window.CapturePage(); err != nil {
		return err
	} else if err := os.WriteFile(filepath.Join(dir, "foldmark-code.png"), bytes, 0600); err != nil {
		return err
	}
	if ok, err := page.Eval(`(() => { const input=document.querySelector('.preview-search input'); if (!input) return false; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, '跨格式'); input.dispatchEvent(new Event('input',{bubbles:true})); return true })()`); err != nil || ok != true {
		return fmt.Errorf("preview cross-format search could not type: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if ok, err := page.Eval(`document.querySelector('.preview-search-count')?.textContent?.includes('1 / 1')`); err != nil || ok != true {
		return fmt.Errorf("preview cross-format search failed: %v", err)
	}
	if ok, err := page.Eval(`(() => { const button = document.querySelector('.preview-image-open'); if (!button) return false; button.click(); return true })()`); err != nil || ok != true {
		return fmt.Errorf("image zoom did not open: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if ok, err := page.Eval(`!!document.querySelector('[data-slot="dialog-content"] img')`); err != nil || ok != true {
		return fmt.Errorf("image zoom dialog missing: %v", err)
	}
	if bytes, err := a.Window.CapturePage(); err != nil {
		return err
	} else if err := os.WriteFile(filepath.Join(dir, "foldmark-image-zoom.png"), bytes, 0600); err != nil {
		return err
	}
	_, _ = page.Eval(`document.querySelector('[data-slot="dialog-close"]')?.click()`)
	_, _ = page.Eval(`document.querySelector('.preview-search [aria-label="关闭查找"]')?.click()`)
	if ok, err := page.Eval(`(() => { const tab = [...document.querySelectorAll('.mode-switch button')].find(x => x.textContent?.includes('分栏')); if (!tab) return false; tab.click(); return true })()`); err != nil || ok != true {
		return fmt.Errorf("split mode did not reopen: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if os.Getenv("FOLDMARK_CAPTURE_SYNC") == "1" {
		if ok, err := page.Eval(`(() => { const e=document.querySelector('.cm-scroller'),p=document.querySelector('.preview-scroll'); if (!e || !p || e.scrollHeight-e.clientHeight<100 || p.scrollHeight-p.clientHeight<100) return false; e.scrollTop=0; p.scrollTop=0; return true })()`); err != nil || ok != true {
			return fmt.Errorf("split panes are not scrollable: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
		_, _ = page.Eval(`(() => {const e=document.querySelector('.cm-scroller');e.scrollTop=(e.scrollHeight-e.clientHeight)*.65})()`)
		time.Sleep(300 * time.Millisecond)
		if ok, err := page.Eval(`document.querySelector('.preview-scroll').scrollTop > 50`); err != nil || ok != true {
			return fmt.Errorf("preview did not follow editor scroll: %v", err)
		}
		if bytes, err := a.Window.CapturePage(); err != nil {
			return err
		} else if err := os.WriteFile(filepath.Join(dir, "foldmark-sync.png"), bytes, 0600); err != nil {
			return err
		}
		before, _ := page.Eval(`document.querySelector('.cm-scroller').scrollTop`)
		_, _ = page.Eval(`(() => {const p=document.querySelector('.preview-scroll');p.scrollTop=(p.scrollHeight-p.clientHeight)*.9})()`)
		time.Sleep(300 * time.Millisecond)
		after, _ := page.Eval(`document.querySelector('.cm-scroller').scrollTop`)
		log.Printf("split scroll: editor before=%v after=%v", before, after)
		if before == after {
			return errors.New("editor did not follow preview scroll")
		}
	}
	return nil
}
