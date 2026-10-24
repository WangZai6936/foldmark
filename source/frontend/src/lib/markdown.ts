import { unified } from "unified"
import remarkParse from "remark-parse"
import remarkGfm from "remark-gfm"
import { visit } from "unist-util-visit"
import type { Heading, Root, Text } from "mdast"

export type OutlineItem = { depth: number; text: string; line: number; slug: string }

function textOf(node: Heading): string {
  const fragments: string[] = []
  visit(node, "text", (part: Text) => { fragments.push(part.value) })
  return fragments.join("")
}

export function outlineOf(source: string): OutlineItem[] {
  const tree = unified().use(remarkParse).use(remarkGfm).parse(source) as Root
  const seen = new Map<string, number>()
  const headings: OutlineItem[] = []
  visit(tree, "heading", (node: Heading) => {
    const text = textOf(node)
    const base = text.toLowerCase().trim().replace(/\s+/g, "-").replace(/[^\p{L}\p{N}_-]/gu, "") || "heading"
    const count = seen.get(base) || 0
    seen.set(base, count + 1)
    headings.push({ depth: node.depth, text, line: (node.position?.start.line ?? 1) - 1, slug: count ? `${base}-${count}` : base })
  })
  return headings
}

export function countWords(source: string): number {
  return (source.match(/[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]|[\p{L}\p{N}]+/gu) || []).length
}
