package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"foldmark/source/internal/content"
	"github.com/egoist/mygo"
)

const maxImageBytes = 16 << 20

var imageFilters = []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "bmp"}}}

func imageExtension(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return strings.ToLower(filepath.Ext(name))
	}
	return ""
}

func validImage(name string, data []byte) bool {
	switch imageExtension(name) {
	case ".png":
		return len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
	case ".jpg", ".jpeg":
		return len(data) >= 3 && string(data[:3]) == "\xff\xd8\xff"
	case ".gif":
		return len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a")
	case ".webp":
		return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	case ".bmp":
		return len(data) >= 2 && string(data[:2]) == "BM"
	}
	return false
}

func saveImage(documentPath, name string, data []byte) (string, error) {
	if documentPath == "" {
		return "", errors.New("请先保存文档，再插入本地图片")
	}
	if len(data) == 0 || len(data) > maxImageBytes || !validImage(name, data) {
		return "", errors.New("请选择不超过 16 MB 的 PNG、JPG、GIF、WebP 或 BMP 图片")
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(documentPath))
	if err != nil {
		return "", err
	}
	assets := filepath.Join(root, "assets")
	if err := os.MkdirAll(assets, 0700); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(assets)
	if err != nil {
		return "", err
	}
	under, err := filepath.Rel(root, resolved)
	if err != nil || under == ".." || strings.HasPrefix(under, ".."+string(filepath.Separator)) {
		return "", errors.New("图片目录不在文档所在文件夹内")
	}
	filename := rand.Text() + imageExtension(name)
	target := filepath.Join(assets, filename)
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(target)
		return "", err
	}
	return filepath.ToSlash(filepath.Join("assets", filename)), nil
}

func importImagePath(documentPath, source string) (string, error) {
	if imageExtension(source) == "" {
		return "", errors.New("请选择 PNG、JPG、GIF、WebP 或 BMP 图片")
	}
	actual, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(documentPath))
	if err != nil {
		return "", err
	}
	if relative, relErr := filepath.Rel(root, actual); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		if _, err := content.ImagePath(documentPath, filepath.ToSlash(relative)); err == nil {
			return filepath.ToSlash(relative), nil
		}
	}
	f, err := os.Open(actual)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return "", err
	}
	return saveImage(documentPath, filepath.Base(actual), data)
}

// ImportImage accepts a native file picker, a dropped path, or clipboard data.
// It returns a relative path so the Markdown stays portable with its assets.
func (d *Desktop) ImportImage(ctx context.Context, id, name, dataURL string) (string, error) {
	var documentPath string
	_, err := d.onMain(ctx, func() error {
		for _, doc := range d.app.Documents {
			if doc.ID == id {
				documentPath = doc.Path
				break
			}
		}
		if documentPath == "" {
			return errors.New("请先保存文档，再插入本地图片")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if dataURL != "" {
		if len(dataURL) > 24<<20 {
			return "", errors.New("图片超过 16 MB")
		}
		prefix, encoded, ok := strings.Cut(dataURL, ",")
		if !ok || !strings.HasPrefix(prefix, "data:image/") || !strings.HasSuffix(prefix, ";base64") {
			return "", errors.New("剪贴板图片无效")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", fmt.Errorf("剪贴板图片无法读取：%w", err)
		}
		if imageExtension(name) == "" {
			name = "image.png"
		}
		return saveImage(documentPath, name, data)
	}
	if name == "" {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: d.app.Window, Title: "插入本地图片", Filters: imageFilters})
		if err != nil || len(paths) == 0 {
			return "", err
		}
		name = paths[0]
	}
	return importImagePath(documentPath, name)
}
