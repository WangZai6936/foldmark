const MAX_HIGHLIGHTS = 3000

// Search rendered text by block, so emphasis and links do not split a phrase.
// CSS Highlights paint ranges without changing React's DOM nodes.
export function findPreviewRanges(article: HTMLElement, query: string): { ranges: Range[]; limited: boolean } {
  if (!query) return { ranges: [], limited: false }
  const pattern = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi")
  const ranges: Range[] = []
  for (const block of Array.from(article.children)) {
    const nodes: Text[] = []
    const offsets: number[] = []
    let content = ""
    const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT)
    while (walker.nextNode()) {
      const node = walker.currentNode as Text
      if (node.parentElement?.closest("[data-preview-tools], .sr-only")) continue
      offsets.push(content.length)
      nodes.push(node)
      content += node.data
    }
    pattern.lastIndex = 0
    for (const match of content.matchAll(pattern)) {
      const start = match.index
      const end = start + match[0].length
      const first = offsets.findLastIndex((offset) => offset <= start)
      const last = offsets.findLastIndex((offset) => offset < end)
      if (first < 0 || last < 0 || !nodes[first] || !nodes[last]) continue
      const range = document.createRange()
      range.setStart(nodes[first], start - offsets[first])
      range.setEnd(nodes[last], end - offsets[last])
      ranges.push(range)
      if (ranges.length >= MAX_HIGHLIGHTS) return { ranges, limited: true }
    }
  }
  return { ranges, limited: false }
}

export function paintPreviewMatches(ranges: Range[], active: number) {
  if (!("highlights" in CSS) || typeof Highlight === "undefined") return
  CSS.highlights.delete("foldmark-search")
  CSS.highlights.delete("foldmark-search-active")
  if (ranges.length) {
    CSS.highlights.set("foldmark-search", new Highlight(...ranges))
    CSS.highlights.set("foldmark-search-active", new Highlight(ranges[active] || ranges[0]))
  }
}

export function scrollPreviewMatch(container: HTMLElement, range: Range) {
  const target = range.getBoundingClientRect()
  const bounds = container.getBoundingClientRect()
  container.scrollTo({ top: container.scrollTop + target.top - bounds.top - bounds.height * .35, behavior: "smooth" })
}
