import { useEffect, useMemo } from "react"
import { EditorContent, useEditor, useEditorState } from "@tiptap/react"
import type { Editor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { TableKit } from "@tiptap/extension-table"
import TaskList from "@tiptap/extension-task-list"
import TaskItem from "@tiptap/extension-task-item"
import Image from "@tiptap/extension-image"
import { Markdown } from "@tiptap/markdown"
import { Button } from "@/components/ui/button"
import type { Document } from "@/lib/mygo"
import { queueEditFlush, useWorkspace } from "@/lib/workspace"

const extensions = [
  StarterKit.configure({ link: { openOnClick: false } }),
  TableKit.configure({ table: { resizable: true } }),
  TaskList,
  TaskItem.configure({ nested: true }),
  Image,
  Markdown.configure({ markedOptions: { gfm: true } }),
]

export function VisualEditor({ doc, fontSize, busy, onReady, onImageFile }: {
  doc: Document
  fontSize: number
  busy: boolean
  onReady: (editor: Editor | null) => void
  onImageFile: (file: File) => void
}) {
  const edit = useWorkspace((state) => state.edit)
  const imageExtension = useMemo(() => Image.extend({
    renderHTML({ HTMLAttributes }) {
      const src = String(HTMLAttributes.src || "")
      const local = src && !/^(?:[a-z][\w+.-]*:|\/|\\)/i.test(src)
      return ["img", { ...HTMLAttributes, src: local ? `http://images.localhost/?document=${encodeURIComponent(doc.id)}&src=${encodeURIComponent(src)}` : src }]
    },
  }), [doc.id])
  const configuredExtensions = useMemo(() => [
    ...extensions.filter((item) => item !== Image), imageExtension,
  ], [imageExtension])
  const editor = useEditor({
    extensions: configuredExtensions,
    content: doc.content,
    contentType: "markdown",
    editable: !busy,
    editorProps: {
      attributes: { class: "visual-content", "aria-label": "可视化 Markdown 编辑器" },
      handlePaste(_view, event) {
        const file = Array.from(event.clipboardData?.files || []).find((item) => /^image\/(png|jpeg|gif|webp|bmp)$/.test(item.type))
        if (!file) return false
        event.preventDefault()
        onImageFile(file)
        return true
      },
    },
    onUpdate({ editor: updated }) {
      edit(doc.id, updated.getMarkdown())
      queueEditFlush()
    },
  }, [doc.id])

  useEffect(() => {
    onReady(editor)
    return () => onReady(null)
  }, [editor, onReady])
  useEffect(() => { editor?.setEditable(!busy) }, [editor, busy])
  useEffect(() => {
    if (editor && editor.getMarkdown() !== doc.content) editor.commands.setContent(doc.content, { contentType: "markdown", emitUpdate: false })
  }, [editor, doc.content])
  const inTable = useEditorState({ editor, selector: ({ editor: current }) => current?.isActive("table") ?? false })
  const tableActions = useMemo(() => [
    { label: "上方插入行", command: () => editor?.chain().focus().addRowBefore().run() },
    { label: "下方插入行", command: () => editor?.chain().focus().addRowAfter().run() },
    { label: "左侧插入列", command: () => editor?.chain().focus().addColumnBefore().run() },
    { label: "右侧插入列", command: () => editor?.chain().focus().addColumnAfter().run() },
    { label: "删除行", command: () => editor?.chain().focus().deleteRow().run() },
    { label: "删除列", command: () => editor?.chain().focus().deleteColumn().run() },
    { label: "删除表格", command: () => editor?.chain().focus().deleteTable().run() },
  ], [editor])
  return <div className="visual-editor-pane">
    <div className="pane-label"><span>可视化编辑</span><span>Tab 可切换单元格</span></div>
    <div className="visual-scroll" style={{ fontSize }}><EditorContent editor={editor} className="visual-paper" /></div>
    {inTable && <div className="table-actions" aria-label="表格操作">{tableActions.map((item) => <Button key={item.label} type="button" variant="ghost" size="sm" onClick={item.command}>{item.label}</Button>)}</div>}
  </div>
}
