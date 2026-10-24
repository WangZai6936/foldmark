import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import type { ReactNode } from "react"
import CodeMirror from "@uiw/react-codemirror"
import type { Editor as RichEditor } from "@tiptap/react"
import { markdown } from "@codemirror/lang-markdown"
import { EditorView } from "@codemirror/view"
import { historyField, redo, undo } from "@codemirror/commands"
import { openSearchPanel } from "@codemirror/search"
import { useDefaultLayout } from "react-resizable-panels"
import { motion, useReducedMotion } from "motion/react"
import { DndContext, closestCenter } from "@dnd-kit/core"
import type { DragEndEvent } from "@dnd-kit/core"
import { SortableContext, useSortable, verticalListSortingStrategy, arrayMove } from "@dnd-kit/sortable"
import { CSS } from "@dnd-kit/utilities"
import { isMyGo } from "mygo-runtime"
import {
  BookOpen, Bold, ChevronDown, Code2, Columns2, Download, Ellipsis,
  Eye, FilePlus2, FileText, FolderClosed, FolderOpen, Heading2, Italic,
  ImagePlus, Link2, List, ListChecks, ListOrdered, Maximize2, Menu, Minus,
  PanelLeftClose, PanelLeftOpen, Plus, Quote, Redo2, Save, Search,
  Settings2, Strikethrough, Table2, Undo2, X,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Separator } from "@/components/ui/separator"
import { ScrollArea } from "@/components/ui/scroll-area"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { PreviewPane } from "@/components/PreviewPane"
import { VisualEditor } from "@/components/VisualEditor"
import { Desktop, events, type Document } from "@/lib/mygo"
import { countWords, outlineOf } from "@/lib/markdown"
import { formatSelection, insertImageMarkdown, slashInsert, type FormatKind } from "@/lib/editor-commands"
import { syncEditorFromPreview, syncPreviewFromEditor } from "@/lib/split-scroll"
import { queueEditFlush, showError, useWorkspace } from "@/lib/workspace"

function action(promise: Promise<unknown>) { promise.catch(showError) }

function applyRichFormat(editor: RichEditor, kind: FormatKind) {
  const chain = editor.chain().focus()
  switch (kind) {
    case "heading1": chain.toggleHeading({ level: 1 }).run(); break
    case "heading2": chain.toggleHeading({ level: 2 }).run(); break
    case "heading3": chain.toggleHeading({ level: 3 }).run(); break
    case "bold": chain.toggleBold().run(); break
    case "italic": chain.toggleItalic().run(); break
    case "strike": chain.toggleStrike().run(); break
    case "quote": chain.toggleBlockquote().run(); break
    case "bullet": chain.toggleBulletList().run(); break
    case "ordered": chain.toggleOrderedList().run(); break
    case "task": chain.toggleTaskList().run(); break
    case "code": chain.toggleCodeBlock().run(); break
    case "rule": chain.setHorizontalRule().run(); break
    case "table": chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(); break
    case "link": {
      const url = window.prompt("链接地址", "https://")
      if (url && /^https?:\/\//i.test(url)) chain.setLink({ href: url }).run()
      break
    }
  }
}

const imageTypes: Record<string, string> = { "image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp", "image/bmp": ".bmp" }

function imageDataURL(file: File): Promise<string> {
  if (file.size > 16 << 20) return Promise.reject(new Error("图片不能超过 16 MB"))
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error || new Error("剪贴板图片无法读取"))
    reader.readAsDataURL(file)
  })
}

function IconButton({ label, children, onClick, disabled, className = "" }: {
  label: string; children: ReactNode; onClick: () => void; disabled?: boolean; className?: string
}) {
  return <Button type="button" size="icon" variant="ghost" aria-label={label} title={label} disabled={disabled} className={className} onClick={onClick}>{children}</Button>
}

function SortableDocument({ doc, active, busy, activate, close }: {
  doc: Document; active: boolean; busy: boolean; activate: () => void; close: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: doc.id, disabled: busy })
  return <div ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={`document-row ${active ? "is-active" : ""} ${isDragging ? "is-dragging" : ""}`}>
    <button className="document-select" type="button" onClick={activate} title={doc.path || doc.name} aria-current={active ? "page" : undefined}>
      <FileText size={16} strokeWidth={1.8} className="doc-icon" />
      <span className="truncate">{doc.name}</span>
      {doc.content !== doc.baseline && <span className="dirty-dot" aria-label="未保存" />}
    </button>
    <button className="document-drag" type="button" aria-label={`拖拽排列 ${doc.name}`} title="拖拽排序" {...attributes} {...listeners}><Menu size={13} /></button>
    <button className="document-close" type="button" aria-label={`关闭 ${doc.name}`} title="关闭文档" onClick={close} disabled={busy}><X size={13} /></button>
  </div>
}

function Sidebar({ openSettings, navigate }: { openSettings: () => void; navigate: (line: number) => void }) {
  const state = useWorkspace((s) => s.state)!
  const command = useWorkspace((s) => s.command)
  const reorder = useWorkspace((s) => s.reorder)
  const [search, setSearch] = useState("")
  const [section, setSection] = useState<"documents" | "outline">("documents")
  const active = state.documents.find((doc) => doc.id === state.activeId)
  const outlined = useMemo(() => outlineOf(active?.content || ""), [active?.content])
  const filtered = state.documents.filter((doc) => doc.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()))
  const recent = state.recent.filter((file) => !state.documents.some((doc) => doc.path.toLocaleLowerCase() === file.Path.toLocaleLowerCase())).slice(0, 5)
  const files = state.files.filter((file) => file.Name.toLocaleLowerCase().includes(search.toLocaleLowerCase()))
  const dragEnd = (event: DragEndEvent) => {
    const { active: source, over } = event
    if (!over || source.id === over.id) return
    const ids = state.documents.map((doc) => doc.id)
    action(reorder(arrayMove(ids, ids.indexOf(String(source.id)), ids.indexOf(String(over.id)))))
  }
  return <aside className="workspace-sidebar">
    <div className="sidebar-top">
      <div className="sidebar-workspace"><div className="workspace-mark"><FolderClosed size={18} strokeWidth={1.9} /></div><div className="min-w-0"><strong>我的文档</strong><span>本地工作空间</span></div><ChevronDown size={14} className="ml-auto text-muted-foreground" /></div>
      <div className="sidebar-actions"><Button className="flex-1" onClick={() => action(command("new"))}><Plus size={16} />新建文档</Button><IconButton label="打开文件" onClick={() => action(command("open"))}><FolderOpen size={17} /></IconButton></div>
      <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索文档…" aria-label="搜索文档" className="sidebar-search" />
      <Tabs value={section} onValueChange={(value) => setSection(value as "documents" | "outline")} className="sidebar-tabs"><TabsList variant="line" className="w-full justify-start"><TabsTrigger value="documents">文档</TabsTrigger><TabsTrigger value="outline">大纲</TabsTrigger></TabsList></Tabs>
    </div>
    <ScrollArea className="sidebar-scroll">
      {section === "documents" ? <div className="sidebar-groups">
        <div className="group-heading"><span>已打开</span><span>{state.documents.length}</span></div>
        <DndContext collisionDetection={closestCenter} onDragEnd={dragEnd}>
          <SortableContext items={filtered.map((doc) => doc.id)} strategy={verticalListSortingStrategy}>
            <div className="document-list">{filtered.map((doc) => <SortableDocument key={doc.id} doc={doc} active={doc.id === state.activeId} busy={state.busy} activate={() => action(command("activate", doc.id))} close={() => action(command("close", doc.id))} />)}</div>
          </SortableContext>
        </DndContext>
        {state.folder && <><div className="group-heading group-heading-spaced"><span title={state.folder}>{state.folder.split(/[\\/]/).pop()}</span><span>{files.length}</span></div><div className="document-list">{files.slice(0, 250).map((file) => <button className="file-link" title={file.Path} key={file.Path} onClick={() => action(command("open-path", "", file.Path))}><FileText size={15} /><span>{file.Name}</span></button>)}</div></>}
        {recent.length > 0 && <><div className="group-heading group-heading-spaced">最近打开</div><div className="document-list">{recent.map((file) => <button className="file-link" title={file.Path} key={file.Path} onClick={() => action(command("open-path", "", file.Path))}><FileText size={15} /><span>{file.Name}</span></button>)}</div></>}
        <button className="sidebar-folder-link" onClick={() => action(command("folder"))}><FolderOpen size={15} />打开文件夹</button>
      </div> : <div className="outline-list">{outlined.length ? outlined.map((heading, index) => <button key={`${heading.line}-${index}`} style={{ paddingLeft: 10 + (heading.depth - 1) * 13 }} onClick={() => navigate(heading.line)} title={heading.text}>{heading.text}</button>) : <p>使用 # 标题组织文档，大纲会在这里出现。</p>}</div>}
    </ScrollArea>
    <div className="sidebar-bottom"><div className="local-caption"><span className="local-point" />文件保存在本机</div><Separator /><Button variant="ghost" className="settings-link" onClick={openSettings}><Settings2 size={16} />偏好设置</Button></div>
  </aside>
}

const editorSessions = new Map<string, ReturnType<EditorView["state"]["toJSON"]>>()

function EditorPane({ doc, fontSize, viewRef, busy, onImageFile, onPickImage, onScroll }: { doc: Document; fontSize: number; viewRef: React.MutableRefObject<EditorView | null>; busy: boolean; onImageFile: (file: File, view: EditorView) => void; onPickImage: () => void; onScroll: (view: EditorView) => void }) {
  const edit = useWorkspace((s) => s.edit)
  const extensions = useMemo(() => [
    markdown(), EditorView.lineWrapping, slashInsert(onPickImage),
    EditorView.domEventHandlers({
      paste(event, view) {
        const file = Array.from(event.clipboardData?.files || []).find((item) => item.type in imageTypes)
        if (!file) return false
        event.preventDefault()
        onImageFile(file, view)
        return true
      },
    }),
  ], [onImageFile, onPickImage])
  const localView = useRef<EditorView | null>(null)
  const scrollListener = useRef<(() => void) | null>(null)
  useEffect(() => () => {
    if (localView.current) {
      if (scrollListener.current) localView.current.scrollDOM.removeEventListener("scroll", scrollListener.current)
      editorSessions.set(doc.id, localView.current.state.toJSON({ history: historyField }))
    }
    if (viewRef.current === localView.current) viewRef.current = null
  }, [doc.id, viewRef])
  return <div className="editor-pane"><div className="pane-label"><span>MARKDOWN</span><span>自动换行</span></div><div className="editor-paper" style={{ fontSize }}><CodeMirror key={doc.id} value={doc.content} readOnly={busy} initialState={editorSessions.has(doc.id) ? { json: editorSessions.get(doc.id), fields: { history: historyField } } : undefined} autoFocus={!doc.content} height="100%" extensions={extensions} basicSetup={{ lineNumbers: false, foldGutter: false, highlightActiveLine: false, highlightActiveLineGutter: false }} onCreateEditor={(view) => { localView.current = view; viewRef.current = view; const listener = () => onScroll(view); scrollListener.current = listener; view.scrollDOM.addEventListener("scroll", listener, { passive: true }) }} onChange={(value) => { edit(doc.id, value); queueEditFlush() }} aria-label="Markdown 编辑器" className="codemirror-root" /></div></div>
}

function EmptyState({ create, open }: { create: () => void; open: () => void }) {
  const reduce = useReducedMotion()
  return <motion.div className="empty-state" initial={reduce ? false : { opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.22 }}><div className="empty-symbol"><BookOpen size={29} strokeWidth={1.5} /></div><h1>写作，从这里开始</h1><p>打开一份 Markdown，或创建一页空白文档。</p><div><Button onClick={create}><Plus size={16} />新建文档</Button><Button variant="outline" onClick={open}><FolderOpen size={16} />打开文件</Button></div><small>也可以将 .md 文件拖入窗口</small></motion.div>
}

function SettingsDialog({ open, setOpen }: { open: boolean; setOpen: (value: boolean) => void }) {
  const settings = useWorkspace((s) => s.state?.settings)
  const preferences = useWorkspace((s) => s.preferences)
  return <Dialog open={open} onOpenChange={setOpen}><DialogContent><DialogHeader><DialogTitle>偏好设置</DialogTitle><DialogDescription>调整写作与阅读的舒适度。</DialogDescription></DialogHeader><div className="setting-row"><div><strong>正文字号</strong><span>同时应用于编辑和预览</span></div><div className="setting-stepper"><IconButton label="减小字号" disabled={!settings || settings.fontSize <= 12} onClick={() => settings && action(preferences(settings.mode, settings.fontSize - 1, settings.sidebar))}><Minus size={15} /></IconButton><span>{settings?.fontSize ?? 15}</span><IconButton label="增大字号" disabled={!settings || settings.fontSize >= 22} onClick={() => settings && action(preferences(settings.mode, settings.fontSize + 1, settings.sidebar))}><Plus size={15} /></IconButton></div></div><p className="settings-note">草稿恢复文件保存在本机。按 Ctrl+S 将更改写回 Markdown 文件。</p></DialogContent></Dialog>
}

export default function App() {
  const state = useWorkspace((s) => s.state)
  const setState = useWorkspace((s) => s.setState)
  const command = useWorkspace((s) => s.command)
  const flush = useWorkspace((s) => s.flush)
  const preferences = useWorkspace((s) => s.preferences)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [focus, setFocus] = useState(false)
  const [previewSearchRequest, setPreviewSearchRequest] = useState(0)
  const viewRef = useRef<EditorView | null>(null)
  const richRef = useRef<RichEditor | null>(null)
  const [richEditor, setRichEditor] = useState<RichEditor | null>(null)
  const pendingRichCommand = useRef<FormatKind | null>(null)
  const pendingTableSize = useRef<[number, number] | null>(null)
  const onRichReady = useCallback((editor: RichEditor | null) => { richRef.current = editor; setRichEditor(editor) }, [])
  const previewScrollRef = useRef<HTMLDivElement | null>(null)
  const scrollOriginRef = useRef<{ source: "editor" | "preview"; until: number } | null>(null)
  const { defaultLayout, onLayoutChanged } = useDefaultLayout({ id: "foldmark-writing-split", panelIds: ["editor", "preview"], onlySaveAfterUserInteractions: true })
  const active = state?.documents.find((doc) => doc.id === state.activeId)
  const mode = state?.settings.mode ?? 1
  const modeRef = useRef(mode)
  modeRef.current = mode
  const fontSize = state?.settings.fontSize ?? 15
  const sidebar = !!state?.settings.sidebar
  const onEditorScroll = useCallback((view: EditorView) => {
    const preview = previewScrollRef.current
    if (modeRef.current !== 1 || !preview || (scrollOriginRef.current?.source === "preview" && performance.now() < scrollOriginRef.current.until)) return
    scrollOriginRef.current = { source: "editor", until: performance.now() + 120 }
    syncPreviewFromEditor(view, preview)
  }, [])
  const onPreviewScroll = useCallback(() => {
    const preview = previewScrollRef.current
    const view = viewRef.current
    if (modeRef.current !== 1 || !preview || !view || (scrollOriginRef.current?.source === "editor" && performance.now() < scrollOriginRef.current.until)) return
    scrollOriginRef.current = { source: "preview", until: performance.now() + 120 }
    syncEditorFromPreview(preview, view)
  }, [])
  useEffect(() => {
    if (mode !== 0 || !richEditor) return
    if (pendingTableSize.current) {
      const [rows, cols] = pendingTableSize.current
      pendingTableSize.current = null
      richEditor.chain().focus().insertTable({ rows, cols, withHeaderRow: true }).run()
    }
    if (!pendingRichCommand.current) return
    const command = pendingRichCommand.current
    pendingRichCommand.current = null
    applyRichFormat(richEditor, command)
  }, [mode, richEditor])
  const importImage = useCallback(async (name = "", dataURL = "", sourceView?: EditorView) => {
    const current = useWorkspace.getState().state
    const doc = current?.documents.find((item) => item.id === current.activeId)
    if (!doc) throw new Error("请先打开文档")
    if (!doc.path) throw new Error("请先保存文档，再插入本地图片")
    if (current?.settings.mode === 2) {
      await useWorkspace.getState().preferences(0, current.settings.fontSize, current.settings.sidebar)
    }
    if (!isMyGo()) return
    const relative = await Desktop.importImage(doc.id, name, dataURL)
    if (!relative) return
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    if (useWorkspace.getState().state?.settings.mode === 0 && richRef.current && useWorkspace.getState().state?.activeId === doc.id) {
      richRef.current.chain().focus().setImage({ src: relative, alt: "图片" }).run()
      return
    }
    const view = sourceView && viewRef.current === sourceView ? sourceView : viewRef.current
    if (view && useWorkspace.getState().state?.activeId === doc.id) insertImageMarkdown(view, relative)
  }, [])
  const pickImage = useCallback(() => action(importImage()), [importImage])
  const pasteImage = useCallback((file: File, view: EditorView) => action(imageDataURL(file).then((data) => importImage(`clipboard${imageTypes[file.type]}`, data, view))), [importImage])
  const pasteVisualImage = useCallback((file: File) => action(imageDataURL(file).then((data) => importImage(`clipboard${imageTypes[file.type]}`, data))), [importImage])
  const run = useCallback((name: string, id = "", argument = "") => action(command(name, id, argument)), [command])
  const setMode = (next: number) => state && action(preferences(next, fontSize, sidebar))
  const toggleSidebar = () => state && action(preferences(mode, fontSize, !sidebar))
  const closeWindow = () => action(flush().then(() => command("close-window")))
  useEffect(() => {
    if (!isMyGo()) { document.documentElement.dataset.ready = "true"; return }
    let mounted = true
    const offState = events.stateChanged.on((next) => { if (mounted) setState(next) })
    const offClose = events.closeRequested.on(() => { if (mounted) closeWindow() })
    const offImage = events.imageDropped.on((path) => { if (mounted) action(importImage(path)) })
    Desktop.state().then((next) => { if (mounted) { setState(next); document.documentElement.dataset.ready = "true" } }).catch(showError)
    return () => { mounted = false; offState(); offClose(); offImage() }
  }, [setState, importImage])
  const navigateOutline = (line: number) => {
    const view = viewRef.current
    if (view) {
      const position = view.state.doc.line(Math.min(line + 1, view.state.doc.lines)).from
      view.dispatch({ selection: { anchor: position }, effects: EditorView.scrollIntoView(position, { y: "start" }) })
    }
    document.querySelector(`[data-heading-line="${line}"]`)?.scrollIntoView({ behavior: "smooth", block: "start" })
  }
  const openFind = () => {
    if (mode === 2) { setPreviewSearchRequest((current) => current + 1); return }
    if (mode === 0) { setMode(1); requestAnimationFrame(() => { if (viewRef.current) openSearchPanel(viewRef.current) }); return }
    requestAnimationFrame(() => { if (viewRef.current) openSearchPanel(viewRef.current) })
  }
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const ctrl = event.ctrlKey || event.metaKey
      const editing = event.target instanceof Element && !!event.target.closest(".cm-editor")
      if (ctrl && !event.altKey) {
        const key = event.key.toLowerCase()
        if (key === "n") { event.preventDefault(); run("new") }
        if (key === "o") { event.preventDefault(); run("open") }
        if (key === "s") { event.preventDefault(); run(event.shiftKey ? "save-as" : "save", active?.id) }
        if (key === "w") { event.preventDefault(); if (active) run("close", active.id) }
        if (key === "f") { event.preventDefault(); openFind() }
        if (key === "h") { event.preventDefault(); if (mode === 2) setMode(1); requestAnimationFrame(() => { if (viewRef.current) openSearchPanel(viewRef.current) }) }
        if (editing && key === "b") { event.preventDefault(); format("bold") }
        if (editing && key === "i") { event.preventDefault(); format("italic") }
        if (editing && key === "k") { event.preventDefault(); format("link") }
        if (editing && event.shiftKey && key === "x") { event.preventDefault(); format("strike") }
        if (editing && event.shiftKey && event.code === "Digit7") { event.preventDefault(); format("ordered") }
        if (editing && event.shiftKey && event.code === "Digit8") { event.preventDefault(); format("bullet") }
      }
      if (event.key === "F11") { event.preventDefault(); setFocus((current) => !current) }
      if (event.key === "Escape" && focus) setFocus(false)
    }
    window.addEventListener("keydown", onKey, true)
    return () => window.removeEventListener("keydown", onKey, true)
  })
  const format = (kind: FormatKind) => {
    if (!active) return
    if (mode === 0 && richRef.current) { applyRichFormat(richRef.current, kind); return }
    if (kind === "table" || mode === 2) {
      pendingRichCommand.current = kind
      setMode(0)
      return
    }
    const apply = () => { if (viewRef.current) formatSelection(viewRef.current, kind) }
    apply()
  }
  const insertTable = (rows: number, cols: number) => {
    if (!active) return
    if (mode === 0 && richRef.current) richRef.current.chain().focus().insertTable({ rows, cols, withHeaderRow: true }).run()
    else { pendingTableSize.current = [rows, cols]; setMode(0) }
  }
  const saveLabel = !active ? "保存" : active.content === active.baseline ? "已保存" : "保存"
  return <div className="app-shell">
    <header className="app-titlebar"><div className="brand"><div className="brand-icon"><BookOpen size={18} strokeWidth={2.1} /></div><strong>折页</strong><span>Foldmark</span></div><div className="titlebar-divider" /><IconButton label={sidebar ? "收起侧栏" : "展开侧栏"} onClick={toggleSidebar}>{sidebar ? <PanelLeftClose size={17} /> : <PanelLeftOpen size={17} />}</IconButton><span className="titlebar-context">{active?.name || "我的文档"}</span><div className="titlebar-spacer" /></header>
    <div className={`workspace ${sidebar && !focus ? "with-sidebar" : ""}`}>
      {sidebar && !focus && state && <Sidebar openSettings={() => setSettingsOpen(true)} navigate={navigateOutline} />}
      <main className="main-surface">
        <div className="document-bar"><div className="document-identity"><span className="document-location">我的文档 <span>/</span> {active?.path ? "本地文件" : "草稿"}</span><div className="document-heading"><h1>{active?.name || "开始写作"}</h1>{active && <span className={`save-indicator ${active.content !== active.baseline ? "is-unsaved" : ""}`}>{active.content !== active.baseline ? "未保存" : active.path ? "已保存" : "草稿"}</span>}</div></div><div className="document-bar-actions"><IconButton label="查找 (Ctrl+F)" disabled={!active} onClick={openFind}><Search size={17} /></IconButton><IconButton label="专注模式 (F11)" onClick={() => setFocus(!focus)}><Maximize2 size={16} /></IconButton><Button variant="outline" size="sm" onClick={() => run("save", active?.id)} disabled={!active || !!state?.busy}><Save size={15} />{saveLabel}</Button><DropdownMenu><DropdownMenuTrigger render={<Button size="icon" variant="ghost" aria-label="更多操作" title="更多操作"><Ellipsis size={18} /></Button>} /><DropdownMenuContent align="end"><DropdownMenuItem onClick={() => run("save-as", active?.id)}><FilePlus2 />另存为副本</DropdownMenuItem><DropdownMenuItem onClick={() => run("export", active?.id)}><Download />导出 HTML</DropdownMenuItem><DropdownMenuItem onClick={() => setSettingsOpen(true)}><Settings2 />偏好设置</DropdownMenuItem></DropdownMenuContent></DropdownMenu></div></div>
        {active && <div className="editor-toolbar"><div className="format-tools">
          <IconButton label="撤销 (Ctrl+Z)" onClick={() => mode === 0 ? richRef.current?.chain().focus().undo().run() : viewRef.current && undo(viewRef.current)}><Undo2 size={16} /></IconButton>
          <IconButton label="重做 (Ctrl+Y)" onClick={() => mode === 0 ? richRef.current?.chain().focus().redo().run() : viewRef.current && redo(viewRef.current)}><Redo2 size={16} /></IconButton>
          <Separator orientation="vertical" className="mx-2 h-5" />
          <DropdownMenu><DropdownMenuTrigger render={<Button variant="ghost" size="sm" className="tool-menu-trigger"><Heading2 size={15} />标题<ChevronDown size={13} /></Button>} /><DropdownMenuContent align="start"><DropdownMenuItem onClick={() => format("heading1")}>一级标题</DropdownMenuItem><DropdownMenuItem onClick={() => format("heading2")}>二级标题</DropdownMenuItem><DropdownMenuItem onClick={() => format("heading3")}>三级标题</DropdownMenuItem></DropdownMenuContent></DropdownMenu>
          <IconButton label="加粗 (Ctrl+B)" onClick={() => format("bold")}><Bold size={16} /></IconButton>
          <IconButton label="斜体 (Ctrl+I)" onClick={() => format("italic")}><Italic size={16} /></IconButton>
          <IconButton label="删除线 (Ctrl+Shift+X)" className="minor-format" onClick={() => format("strike")}><Strikethrough size={16} /></IconButton>
          <IconButton label="链接 (Ctrl+K)" className="minor-format" onClick={() => format("link")}><Link2 size={16} /></IconButton>
          <Separator orientation="vertical" className="mx-2 h-5" />
          <DropdownMenu><DropdownMenuTrigger render={<Button variant="ghost" size="sm" className="tool-menu-trigger"><Plus size={15} />插入<ChevronDown size={13} /></Button>} /><DropdownMenuContent align="start"><DropdownMenuItem onClick={() => format("bullet")}><List />无序列表</DropdownMenuItem><DropdownMenuItem onClick={() => format("ordered")}><ListOrdered />有序列表</DropdownMenuItem><DropdownMenuItem onClick={() => format("task")}><ListChecks />任务清单</DropdownMenuItem><DropdownMenuItem onClick={() => format("quote")}><Quote />引用</DropdownMenuItem><DropdownMenuItem onClick={() => format("code")}><Code2 />代码块</DropdownMenuItem><DropdownMenuItem onClick={() => insertTable(2, 2)}><Table2 />表格 · 2 × 2</DropdownMenuItem><DropdownMenuItem onClick={() => insertTable(3, 3)}><Table2 />表格 · 3 × 3</DropdownMenuItem><DropdownMenuItem onClick={() => insertTable(4, 4)}><Table2 />表格 · 4 × 4</DropdownMenuItem><DropdownMenuItem onClick={() => format("rule")}><Minus />分割线</DropdownMenuItem><DropdownMenuItem onClick={pickImage}><ImagePlus />本地图片</DropdownMenuItem></DropdownMenuContent></DropdownMenu>
        </div><Tabs value={String(mode)} onValueChange={(value) => setMode(Number(value))}><TabsList className="mode-switch"><TabsTrigger value="0"><BookOpen size={14} />编辑</TabsTrigger><TabsTrigger value="1"><Columns2 size={14} />源码分栏</TabsTrigger><TabsTrigger value="2"><Eye size={14} />预览</TabsTrigger></TabsList></Tabs></div>}
        <div className="content-area">{!state ? <div className="loading-state">正在打开工作空间…</div> : !active ? <EmptyState create={() => run("new")} open={() => run("open")} /> : mode === 1 ? <ResizablePanelGroup orientation="horizontal" defaultLayout={defaultLayout} onLayoutChanged={onLayoutChanged}><ResizablePanel id="editor" defaultSize={50} minSize={25}><EditorPane doc={active} fontSize={fontSize} viewRef={viewRef} busy={state.busy} onImageFile={pasteImage} onPickImage={pickImage} onScroll={onEditorScroll} /></ResizablePanel><ResizableHandle withHandle /><ResizablePanel id="preview" defaultSize={50} minSize={25}><PreviewPane doc={active} fontSize={fontSize} scrollRef={previewScrollRef} onScroll={onPreviewScroll} searchRequest={previewSearchRequest} /></ResizablePanel></ResizablePanelGroup> : mode === 0 ? <VisualEditor key={active.id} doc={active} fontSize={fontSize} busy={state.busy} onReady={onRichReady} onImageFile={pasteVisualImage} /> : <PreviewPane doc={active} fontSize={fontSize} scrollRef={previewScrollRef} onScroll={onPreviewScroll} searchRequest={previewSearchRequest} />}</div>
        {state?.message && <motion.div className={`notice ${state.messageError ? "is-error" : ""}`} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}><span>{state.message}</span><IconButton label="关闭提示" onClick={() => run("dismiss")}><X size={14} /></IconButton></motion.div>}
        <footer className="statusbar"><span><span className="status-dot" />本地草稿恢复已开启</span>{active && <span>{countWords(active.content)} 字词</span>}<span className="status-spacer" />{active && <><span>UTF-8{active.bom ? " BOM" : ""}</span><span>{active.eol}</span></>}{focus && <span>按 Esc 退出专注</span>}</footer>
      </main>
    </div>
    <SettingsDialog open={settingsOpen} setOpen={setSettingsOpen} />
  </div>
}

