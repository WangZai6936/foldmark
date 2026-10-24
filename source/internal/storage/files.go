package storage

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 8 << 20

var ErrConflict = errors.New("文件已被其他程序修改，请另存副本或确认覆盖")

type Document struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	Baseline    string `json:"baseline"`
	EOL         string `json:"eol"`
	BOM         bool   `json:"bom"`
	Fingerprint string `json:"fingerprint"`
}

func (d Document) Dirty() bool { return d.Content != d.Baseline }

func New(name, content string) Document {
	return Document{ID: rand.Text(), Name: name, Content: content, Baseline: content, EOL: "LF"}
}

func IsMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown":
		return true
	}
	return false
}

func Hash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func Read(path string) (Document, error) {
	if !IsMarkdown(path) {
		return Document{}, errors.New("请选择 .md、.markdown 或 .mdown 文件")
	}
	actual, err := filepath.Abs(path)
	if err != nil {
		return Document{}, err
	}
	actual, err = filepath.EvalSymlinks(actual)
	if err != nil {
		return Document{}, err
	}
	f, err := os.Open(actual)
	if err != nil {
		return Document{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Document{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return Document{}, errors.New("请选择不超过 8 MB 的 Markdown 文件")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return Document{}, err
	}
	if len(b) > MaxBytes || !utf8.Valid(b) || bytes.ContainsRune(b, 0) {
		return Document{}, errors.New("支持不超过 8 MB 的 UTF-8 文本，请先转换文件编码")
	}
	bom := bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf})
	content := strings.TrimPrefix(string(b), "\ufeff")
	eol := "LF"
	if strings.Contains(content, "\r\n") {
		eol = "CRLF"
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	doc := New(filepath.Base(actual), content)
	doc.Path, doc.EOL, doc.BOM, doc.Fingerprint = actual, eol, bom, Hash(b)
	return doc, nil
}

// AtomicWrite keeps the old file intact until its complete replacement is ready.
func AtomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".foldmark-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func Save(doc Document, destination string, overwrite bool) (Document, error) {
	if !IsMarkdown(destination) {
		return doc, errors.New("文件扩展名必须是 .md、.markdown 或 .mdown")
	}
	if len(doc.Content) > MaxBytes || !utf8.ValidString(doc.Content) {
		return doc, errors.New("文档超出 8 MB 或不是有效的 UTF-8 文本")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return doc, err
	}
	if strings.EqualFold(absolute, doc.Path) && doc.Fingerprint != "" && !overwrite {
		b, readErr := os.ReadFile(absolute)
		if errors.Is(readErr, os.ErrNotExist) {
			return doc, ErrConflict
		}
		if readErr != nil {
			return doc, readErr
		}
		if Hash(b) != doc.Fingerprint {
			return doc, ErrConflict
		}
	}
	text := strings.ReplaceAll(doc.Content, "\r\n", "\n")
	if doc.EOL == "CRLF" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if doc.BOM {
		text = "\ufeff" + text
	}
	b := []byte(text)
	if err := AtomicWrite(absolute, b); err != nil {
		return doc, fmt.Errorf("保存失败：%w", err)
	}
	doc.Path, doc.Name, doc.Baseline, doc.Fingerprint = absolute, filepath.Base(absolute), doc.Content, Hash(b)
	return doc, nil
}

type File struct{ Path, Name string }

func Scan(root string) ([]File, bool, error) {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, false, errors.New("无法读取这个文件夹")
	}
	files := make([]File, 0)
	truncated := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			if strings.Count(rel, string(filepath.Separator)) >= 8 {
				truncated = true
				return filepath.SkipDir
			}
			if path != root {
				switch entry.Name() {
				case ".git", "node_modules", ".cache", "release", "dist":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if IsMarkdown(path) {
			if len(files) == 1500 {
				truncated = true
				return filepath.SkipAll
			}
			files = append(files, File{path, rel})
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	return files, truncated, err
}
