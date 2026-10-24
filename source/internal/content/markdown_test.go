package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeadingsIgnoreCodeAndSupportSetext(t *testing.T) {
	got := Headings("# 主标题\n\n```md\n# 这是代码\n```\n\n第二节\n---\n")
	if len(got) != 2 || got[0].Text != "主标题" || got[1].Text != "第二节" || got[1].Line != 6 {
		t.Fatalf("bad headings: %+v", got)
	}
	if WordCount("你好 world 2026") != 4 {
		t.Fatal("word count should combine CJK characters and words")
	}
}

func TestExportEmbedsImagesAndEscapesUnsafeHTML(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, "note.md")
	os.WriteFile(filepath.Join(root, "pic.png"), []byte("png"), 0600)
	html, err := Export("标题", "# Hello\n\n![图](pic.png)\n\n<script>alert('x')</script>\n\n<a href=\"javascript:alert(1)\" onclick=\"bad()\">link</a>\n", doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "data:image/png;base64,cG5n") {
		t.Fatal("image not embedded")
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, `href="javascript:`) || strings.Contains(html, `onclick=`) {
		t.Fatal("export contains active HTML")
	}
	if !strings.Contains(html, "Content-Security-Policy") {
		t.Fatal("missing export CSP")
	}
}

func TestImagePathsStayInDocumentFolder(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "notes"), 0700)
	os.WriteFile(filepath.Join(root, "outside.png"), []byte("png"), 0600)
	doc := filepath.Join(root, "notes", "a.md")
	for _, raw := range []string{"../outside.png", "https://example.com/pic.png", "file:///C:/secret.png", "%2e%2e/outside.png"} {
		if _, err := ImagePath(doc, raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
