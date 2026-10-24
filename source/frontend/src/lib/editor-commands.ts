import { autocompletion, startCompletion, type CompletionContext } from "@codemirror/autocomplete"
import { EditorView } from "@codemirror/view"

export type FormatKind = "heading1" | "heading2" | "heading3" | "bold" | "italic" | "strike" | "link" | "quote" | "bullet" | "ordered" | "task" | "code" | "table" | "rule"

type Snippet = { text: string; selectFrom: number; selectTo: number }

const blockSnippets: Record<"code" | "table" | "rule", Snippet> = {
  code: { text: "```\n在这里写代码\n```", selectFrom: 4, selectTo: 11 },
  table: { text: "| 标题 | 内容 |\n| --- | --- |\n| 第一项 | 写下内容 |", selectFrom: 2, selectTo: 4 },
  rule: { text: "---", selectFrom: 3, selectTo: 3 },
}

const linePrefixes: Partial<Record<FormatKind, string>> = {
  heading1: "# ", heading2: "## ", heading3: "### ", quote: "> ",
  bullet: "- ", ordered: "1. ", task: "- [ ] ",
}

export function insertSnippet(view: EditorView, snippet: Snippet, from = view.state.selection.main.from, to = view.state.selection.main.to) {
  view.dispatch({
    changes: { from, to, insert: snippet.text },
    selection: { anchor: from + snippet.selectFrom, head: from + snippet.selectTo },
    scrollIntoView: true,
  })
  view.focus()
}

export function formatSelection(view: EditorView, kind: FormatKind) {
  const range = view.state.selection.main
  const chosen = view.state.doc.sliceString(range.from, range.to)
  if (kind === "bold" || kind === "italic" || kind === "strike" || kind === "link") {
    const [before, after, placeholder] = {
      bold: ["**", "**", "加粗文字"], italic: ["*", "*", "斜体文字"],
      strike: ["~~", "~~", "删除线文字"], link: ["[", "](https://example.com)", "链接文字"],
    }[kind]
    const middle = chosen || placeholder
    insertSnippet(view, { text: before + middle + after, selectFrom: before.length, selectTo: before.length + middle.length })
    return
  }
  if (kind in blockSnippets) {
    if (kind === "code" && chosen) {
      insertSnippet(view, { text: `\`\`\`\n${chosen}\n\`\`\``, selectFrom: 4, selectTo: 4 + chosen.length })
    } else {
      insertSnippet(view, blockSnippets[kind as keyof typeof blockSnippets], range.from, range.from)
    }
    return
  }
  const prefix = linePrefixes[kind]
  if (!prefix) return
  const start = view.state.doc.lineAt(range.from).from
  const end = view.state.doc.lineAt(range.to).to
  const original = view.state.doc.sliceString(start, end)
  const lines = original.split("\n")
  const result = lines.map((line, index) => {
    if (kind === "ordered") return `${index + 1}. ${line.replace(/^\d+\.\s*/, "")}`
    if (kind.startsWith("heading")) return prefix + line.replace(/^#{1,6}\s*/, "")
    return prefix + line
  }).join("\n")
  view.dispatch({ changes: { from: start, to: end, insert: result }, selection: { anchor: start + result.length }, scrollIntoView: true })
  view.focus()
}

export function insertImageMarkdown(view: EditorView, relativePath: string) {
  const path = relativePath.split("/").map(encodeURIComponent).join("/")
  insertSnippet(view, { text: `![图片](${path})`, selectFrom: 2, selectTo: 4 })
}

const slashItems: Array<{ label: string; detail: string; kind: FormatKind | "image" }> = [
  { label: "一级标题", detail: "# 标题", kind: "heading1" },
  { label: "二级标题", detail: "## 标题", kind: "heading2" },
  { label: "三级标题", detail: "### 标题", kind: "heading3" },
  { label: "无序列表", detail: "- 列表项", kind: "bullet" },
  { label: "有序列表", detail: "1. 列表项", kind: "ordered" },
  { label: "任务清单", detail: "- [ ] 待办", kind: "task" },
  { label: "引用", detail: "> 引用内容", kind: "quote" },
  { label: "代码块", detail: "```", kind: "code" },
  { label: "表格", detail: "两列表格", kind: "table" },
  { label: "分割线", detail: "---", kind: "rule" },
  { label: "图片", detail: "插入本地图片", kind: "image" },
]

export function slashInsert(onImage: () => void) {
  return [autocompletion({
    override: [(context: CompletionContext) => {
      const match = context.matchBefore(/\/[\p{L}\d-]*$/u)
      if (!match) return null
      const line = context.state.doc.lineAt(match.from)
      if (context.state.doc.sliceString(line.from, match.from).trim()) return null
      return {
        from: match.from,
        filter: false,
        options: slashItems.filter((item) => !match.text.slice(1) || `${item.label} ${item.detail}`.includes(match.text.slice(1))).map((item) => ({
          label: item.label,
          detail: item.detail,
          apply(view: EditorView, _completion: unknown, from: number, to: number) {
            view.dispatch({ changes: { from, to, insert: "" }, selection: { anchor: from } })
            if (item.kind === "image") onImage()
            else formatSelection(view, item.kind)
          },
        })),
      }
    }],
  }), EditorView.updateListener.of((update) => {
    if (!update.docChanged) return
    const cursor = update.state.selection.main.head
    if (!cursor || update.state.doc.sliceString(cursor - 1, cursor) !== "/") return
    const line = update.state.doc.lineAt(cursor)
    if (update.state.doc.sliceString(line.from, cursor - 1).trim()) return
    queueMicrotask(() => startCompletion(update.view))
  })]
}
