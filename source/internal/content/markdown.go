package content

import (
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	xhtml "golang.org/x/net/html"
)

type Heading struct {
	Text        string
	Level, Line int
}

func Headings(text string) []Heading {
	d := parser.Parse(text)
	result := make([]Heading, 0)
	for i := 1; i < d.Block(0).Next; i++ {
		b := d.Block(i)
		if b.Kind == parser.Heading {
			result = append(result, Heading{parser.PlainText(d.Inlines(i)), b.Level, strings.Count(text[:b.Start], "\n")})
		}
	}
	return result
}

func WordCount(text string) int {
	count, inWord := 0, false
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			count++
			inWord = false
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				count++
			}
			inWord = true
		} else {
			inWord = false
		}
	}
	return count
}

// ImagePath limits document images to regular files beneath the document folder.
func ImagePath(document, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || document == "" {
		return "", errors.New("仅加载文档目录内的本地图片")
	}
	rel := u.Path
	if filepath.IsAbs(rel) {
		return "", errors.New("图片路径无效")
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(document))
	if err != nil {
		return "", err
	}
	file, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	under, err := filepath.Rel(root, file)
	if err != nil || under == ".." || strings.HasPrefix(under, ".."+string(filepath.Separator)) {
		return "", errors.New("图片不在文档目录内")
	}
	switch strings.ToLower(filepath.Ext(file)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
	default:
		return "", errors.New("暂不支持该图片格式")
	}
	info, err := os.Stat(file)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return "", errors.New("图片超过 16 MB")
	}
	return file, nil
}

func Images(document string) func(string) (ui.ImageSource, error) {
	return func(raw string) (ui.ImageSource, error) {
		file, err := ImagePath(document, raw)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		return ui.DecodeBitmap(b)
	}
}

func Export(name, source, document string) (string, error) {
	d := parser.Parse(source)
	body := d.HTML()
	// The parser includes raw HTML. Sanitize it before writing a browser page.
	seen, total := map[string]bool{}, 0
	for i := 1; i < d.Block(0).Next; i++ {
		for _, in := range d.Inlines(i) {
			if in.Kind != parser.Image || seen[in.Dest] {
				continue
			}
			seen[in.Dest] = true
			file, err := ImagePath(document, in.Dest)
			if err != nil {
				return "", fmt.Errorf("图片 %s 无法导出：%w", in.Dest, err)
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return "", err
			}
			total += len(b)
			if total > 64<<20 {
				return "", errors.New("导出图片总大小超过 64 MB")
			}
			mime := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp"}[strings.ToLower(filepath.Ext(file))]
			body = strings.ReplaceAll(body, `src="`+html.EscapeString(in.Dest)+`"`, `src="data:`+mime+`;base64,`+base64.StdEncoding.EncodeToString(b)+`"`)
		}
	}
	return `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>` + html.EscapeString(name) + `</title><style>body{max-width:820px;margin:60px auto;padding:0 28px;background:#fcfdf9;color:#283b30;font:16px/1.85 "Segoe UI","Microsoft YaHei",sans-serif}h1,h2,h3{line-height:1.5}h2{margin-top:2em}a{color:#2f6b4f}pre,blockquote{padding:18px;background:#eef2eb;border-radius:6px}code{font-family:Consolas,monospace}pre{overflow:auto}table{width:100%;border-collapse:collapse}td,th{border:1px solid #d6dfd3;padding:8px 12px}img{max-width:100%}hr{border:0;border-top:1px solid #d6dfd3}</style><body>` + safeHTML(body) + `</body></html>`, nil
}

func safeHTML(source string) string {
	allowed := map[string]bool{}
	for _, tag := range strings.Fields("p h1 h2 h3 h4 h5 h6 strong b em i u s del code pre blockquote ul ol li table thead tbody tr th td a img hr br input sup sub section div span") {
		allowed[tag] = true
	}
	z := xhtml.NewTokenizer(strings.NewReader(source))
	var result strings.Builder
	for {
		typ := z.Next()
		if typ == xhtml.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			break
		}
		if typ == xhtml.TextToken {
			result.WriteString(html.EscapeString(string(z.Text())))
			continue
		}
		if typ == xhtml.CommentToken || typ == xhtml.DoctypeToken {
			continue
		}
		token := z.Token()
		if !allowed[token.Data] {
			result.WriteString(html.EscapeString(token.String()))
			continue
		}
		attrs := token.Attr[:0]
		for _, attr := range token.Attr {
			switch attr.Key {
			case "alt", "title", "id", "class", "start", "align", "colspan", "rowspan":
				attrs = append(attrs, attr)
			case "href":
				if u, err := url.Parse(attr.Val); err == nil && (u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "mailto" || strings.HasPrefix(attr.Val, "#")) {
					attrs = append(attrs, attr)
				}
			case "src":
				if strings.HasPrefix(attr.Val, "data:image/") {
					attrs = append(attrs, attr)
				}
			case "type":
				if token.Data == "input" && attr.Val == "checkbox" {
					attrs = append(attrs, attr)
				}
			case "checked", "disabled":
				if token.Data == "input" {
					attrs = append(attrs, attr)
				}
			}
		}
		if token.Data == "input" {
			attrs = append(attrs, xhtml.Attribute{Key: "disabled", Val: ""})
		}
		token.Attr = attrs
		result.WriteString(token.String())
	}
	return result.String()
}
