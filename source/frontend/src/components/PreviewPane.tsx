import { useEffect, useMemo, useRef, useState } from "react"
import type { ReactNode, RefObject } from "react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import rehypeSanitize from "rehype-sanitize"
import { Check, ChevronDown, ChevronUp, Copy, Search, X } from "lucide-react"
import { isMyGo } from "mygo-runtime"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Desktop, type Document } from "@/lib/mygo"
import { outlineOf } from "@/lib/markdown"
import { findPreviewRanges, paintPreviewMatches, scrollPreviewMatch } from "@/lib/preview-search"
import { showError } from "@/lib/workspace"

function CodeBlock({ children, line }: { children: ReactNode; line?: number }) {
  const codeRef = useRef<HTMLPreElement>(null)
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 1800)
    return () => window.clearTimeout(timer)
  }, [copied])
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(codeRef.current?.textContent || "")
      setCopied(true)
    } catch (error) { showError(error) }
  }
  return <div className="preview-code-block" data-source-line={line}>
    <div className="preview-code-tools" data-preview-tools><span>代码</span><Button type="button" size="sm" variant="ghost" onClick={copy} aria-label="复制代码">{copied ? <Check size={13} /> : <Copy size={13} />}{copied ? "已复制" : "复制"}</Button></div>
    <pre ref={codeRef}>{children}</pre>
  </div>
}

type PreviewPaneProps = {
  doc: Document
  fontSize: number
  scrollRef: RefObject<HTMLDivElement | null>
  onScroll: () => void
  searchRequest: number
}

export function PreviewPane({ doc, fontSize, scrollRef, onScroll, searchRequest }: PreviewPaneProps) {
  const headings = useMemo(() => outlineOf(doc.content), [doc.content])
  const articleRef = useRef<HTMLElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const matchesRef = useRef<Range[]>([])
  const lastSearchRequest = useRef(searchRequest)
  const [searchOpen, setSearchOpen] = useState(false)
  const [query, setQuery] = useState("")
  const [matchCount, setMatchCount] = useState(0)
  const [limited, setLimited] = useState(false)
  const [activeMatch, setActiveMatch] = useState(0)
  const [image, setImage] = useState<{ src: string; alt: string } | null>(null)

  useEffect(() => {
    if (searchRequest > lastSearchRequest.current) { setSearchOpen(true); inputRef.current?.focus() }
    lastSearchRequest.current = searchRequest
  }, [searchRequest])
  useEffect(() => { if (searchOpen) inputRef.current?.focus() }, [searchOpen])
  useEffect(() => {
    const article = articleRef.current
    const found = searchOpen && article ? findPreviewRanges(article, query.trim()) : { ranges: [], limited: false }
    matchesRef.current = found.ranges
    setMatchCount(found.ranges.length)
    setLimited(found.limited)
    setActiveMatch(0)
    paintPreviewMatches(found.ranges, 0)
    if (found.ranges.length && scrollRef.current) scrollPreviewMatch(scrollRef.current, found.ranges[0])
    return () => paintPreviewMatches([], 0)
  }, [doc.id, doc.content, query, searchOpen, scrollRef])
  useEffect(() => {
    paintPreviewMatches(matchesRef.current, activeMatch)
    const range = matchesRef.current[activeMatch]
    if (range && scrollRef.current) scrollPreviewMatch(scrollRef.current, range)
  }, [activeMatch, matchCount, scrollRef])

  const nextMatch = (direction: number) => {
    if (matchCount) setActiveMatch((current) => (current + direction + matchCount) % matchCount)
  }
  const closeSearch = () => { setSearchOpen(false); setQuery("") }
  const transformUrl = (url: string) => {
    if (/^(https?:|mailto:|#)/i.test(url)) return url
    if (/^[a-z][\w+.-]*:/i.test(url) || url.startsWith("/") || url.startsWith("\\")) return ""
    return url
  }
  const imageUrl = (src?: string) => {
    if (!src || /^(?:[a-z][\w+.-]*:|\/|\\)/i.test(src)) return ""
    return `http://images.localhost/?document=${encodeURIComponent(doc.id)}&src=${encodeURIComponent(src)}`
  }
  const renderedMarkdown = useMemo(() => {
    let headingIndex = 0
    return <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]} urlTransform={transformUrl} components={{
      h1: ({ children, node }) => { const item = headings[headingIndex++]; return <h1 id={item?.slug} data-heading-line={item?.line} data-source-line={node?.position?.start.line}>{children}</h1> },
      h2: ({ children, node }) => { const item = headings[headingIndex++]; return <h2 id={item?.slug} data-heading-line={item?.line} data-source-line={node?.position?.start.line}>{children}</h2> },
      h3: ({ children, node }) => { const item = headings[headingIndex++]; return <h3 id={item?.slug} data-heading-line={item?.line} data-source-line={node?.position?.start.line}>{children}</h3> },
      h4: ({ children, node }) => { const item = headings[headingIndex++]; return <h4 id={item?.slug} data-heading-line={item?.line} data-source-line={node?.position?.start.line}>{children}</h4> },
      h5: ({ children, node }) => { const item = headings[headingIndex++]; return <h5 id={item?.slug} data-heading-line={item?.line} data-source-line={node?.position?.start.line}>{children}</h5> },
      h6: ({ children, node }) => { const item = headings[headingIndex++]; return <h6 id={item?.slug} data-heading-line={item?.line} data-source-line={node?.position?.start.line}>{children}</h6> },
      p: ({ children, node }) => <p data-source-line={node?.position?.start.line}>{children}</p>,
      ul: ({ children, node }) => <ul data-source-line={node?.position?.start.line}>{children}</ul>,
      ol: ({ children, node }) => <ol data-source-line={node?.position?.start.line}>{children}</ol>,
      blockquote: ({ children, node }) => <blockquote data-source-line={node?.position?.start.line}>{children}</blockquote>,
      table: ({ children, node }) => <table data-source-line={node?.position?.start.line}>{children}</table>,
      hr: ({ node }) => <hr data-source-line={node?.position?.start.line} />,
      pre: ({ children, node }) => <CodeBlock line={node?.position?.start.line}>{children}</CodeBlock>,
      a: ({ href, children }) => <a href={href} onClick={(event) => { if (href?.startsWith("#")) return; event.preventDefault(); if (href && isMyGo()) (href.match(/^https?:|^mailto:/i) ? Desktop.openLink(href) : Desktop.command("open-relative", doc.id, href)).catch(showError) }}>{children}</a>,
      img: ({ src, alt }) => { const resolved = imageUrl(src); return resolved ? <button type="button" className="preview-image-open" onClick={() => setImage({ src: resolved, alt: alt || "图片" })} aria-label={`放大查看 ${alt || "图片"}`}><img src={resolved} alt={alt || ""} loading="lazy" /></button> : <span>{alt || "图片无法显示"}</span> },
    }}>{doc.content || "_第一句话，从这里开始。_"}</ReactMarkdown>
  }, [doc.content, doc.id, headings])
  return <div className="preview-pane">
    <div className="pane-label"><span>阅读预览</span><div className="preview-label-actions"><span className="preview-live"><span />实时更新</span><Button type="button" size="icon-sm" variant="ghost" onClick={() => { setSearchOpen(true); inputRef.current?.focus() }} aria-label="在预览中查找"><Search size={14} /></Button></div></div>
    {searchOpen && <div className="preview-search" role="search">
      <Search size={15} aria-hidden="true" />
      <Input ref={inputRef} value={query} onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); nextMatch(event.shiftKey ? -1 : 1) } if (event.key === "Escape") closeSearch() }} placeholder="查找预览正文" aria-label="查找预览正文" />
      <span className="preview-search-count" aria-live="polite">{query ? matchCount ? `${activeMatch + 1} / ${matchCount}${limited ? "+" : ""}` : "无结果" : ""}</span>
      <Button type="button" variant="ghost" size="icon-sm" onClick={() => nextMatch(-1)} disabled={!matchCount} aria-label="上一处"><ChevronUp size={15} /></Button>
      <Button type="button" variant="ghost" size="icon-sm" onClick={() => nextMatch(1)} disabled={!matchCount} aria-label="下一处"><ChevronDown size={15} /></Button>
      <Button type="button" variant="ghost" size="icon-sm" onClick={closeSearch} aria-label="关闭查找"><X size={15} /></Button>
    </div>}
    <div className="preview-scroll" ref={scrollRef} onScroll={onScroll}>
      <article ref={articleRef} className="reading-paper" style={{ fontSize: fontSize + 1 }}>{renderedMarkdown}</article>
    </div>
    <Dialog open={!!image} onOpenChange={(open) => { if (!open) setImage(null) }}><DialogContent className="preview-image-dialog"><DialogHeader><DialogTitle>{image?.alt || "图片"}</DialogTitle><DialogDescription>本地文档图片</DialogDescription></DialogHeader>{image && <img src={image.src} alt={image.alt} />}</DialogContent></Dialog>
  </div>
}
