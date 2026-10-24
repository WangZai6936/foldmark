import type { EditorView } from "@codemirror/view"

type Anchor = { line: number; top: number }

function anchors(preview: HTMLElement, totalLines: number): Anchor[] {
  const article = preview.querySelector<HTMLElement>(".reading-paper")
  const maxScroll = Math.max(0, preview.scrollHeight - preview.clientHeight)
  const result: Anchor[] = [{ line: 1, top: 0 }]
  if (article) {
    const bounds = preview.getBoundingClientRect()
    for (const block of Array.from(article.children)) {
      if (!(block instanceof HTMLElement)) continue
      const line = Number(block.dataset.sourceLine)
      if (!Number.isFinite(line) || line < 1 || line > totalLines) continue
      const top = block.getBoundingClientRect().top - bounds.top + preview.scrollTop
      if (line > result[result.length - 1].line) result.push({ line, top: Math.max(0, Math.min(maxScroll, top)) })
    }
  }
  if (totalLines > result[result.length - 1].line) result.push({ line: totalLines, top: maxScroll })
  return result
}

function interpolate(items: Anchor[], value: number, input: "line" | "top", output: "line" | "top") {
  if (items.length === 1 || value <= items[0][input]) return items[0][output]
  for (let i = 1; i < items.length; i++) {
    if (value <= items[i][input]) {
      const before = items[i - 1]
      const after = items[i]
      const span = after[input] - before[input]
      return span <= 0 ? after[output] : before[output] + (value - before[input]) / span * (after[output] - before[output])
    }
  }
  return items[items.length - 1][output]
}

export function syncPreviewFromEditor(view: EditorView, preview: HTMLElement) {
  const bounds = view.scrollDOM.getBoundingClientRect()
  const position = view.posAtCoords({ x: bounds.left + 12, y: bounds.top + 12 }) ?? view.viewport.from
  const line = view.state.doc.lineAt(position).number
  const target = interpolate(anchors(preview, view.state.doc.lines), line, "line", "top")
  preview.scrollTop = Math.max(0, target)
}

export function syncEditorFromPreview(preview: HTMLElement, view: EditorView) {
  const line = Math.max(1, Math.min(view.state.doc.lines, Math.round(interpolate(anchors(preview, view.state.doc.lines), preview.scrollTop, "top", "line"))))
  view.scrollDOM.scrollTop = view.lineBlockAt(view.state.doc.line(line).from).top
}
