import { create } from "zustand"
import { Desktop, type Document, type EditRequest, type ViewState } from "@/lib/mygo"
import { isMyGo } from "mygo-runtime"

export type EditorMode = 0 | 1 | 2

type WorkspaceStore = {
  state: ViewState | null
  pending: Record<string, EditRequest>
  setState: (incoming: ViewState) => void
  edit: (id: string, content: string) => void
  flush: () => Promise<void>
  command: (name: string, id?: string, argument?: string) => Promise<void>
  preferences: (mode: number, fontSize: number, sidebar: boolean) => Promise<void>
  reorder: (ids: string[]) => Promise<void>
}

const example = `# 把想法，慢慢写下来。\n\n欢迎来到 **折页 Foldmark**。打开一份 Markdown，开始阅读与写作。\n\n> 写下第一行，让散落的想法在这里相遇。\n\n## 从一份文档开始\n\n点击 **打开文件**，或将 \`.md\` 文件拖到窗口中。按 **Ctrl + N**，也可以从空白开始。\n\n## 让文字有自己的层次\n\n- **加粗**，留住一句话的重点\n- *斜体*，给文字一点语气\n- 用列表，把零散的想法逐一安放\n\n### 今天，做一点小事\n\n- [ ] 写下一个新想法\n- [ ] 整理一份旧笔记\n- [ ] 好好读完一篇文章\n`

const demo: ViewState = {
  documents: [{ id: "demo", name: "欢迎使用.md", path: "", content: example, baseline: example, eol: "LF", bom: false, fingerprint: "", revision: 0 }],
  recent: [], files: [], folder: "", activeId: "demo", settings: { mode: 0, fontSize: 15, sidebar: true, activeId: "demo" }, busy: false, message: "", messageError: false,
}

function mergeDocument(incoming: Document, local?: Document): Document {
  return local && local.revision > incoming.revision ? local : incoming
}

export const useWorkspace = create<WorkspaceStore>((set, get) => ({
  state: isMyGo() ? null : demo,
  pending: {},
  setState: (incoming) => set(({ state }) => ({
    state: { ...incoming, documents: incoming.documents.map((doc) => mergeDocument(doc, state?.documents.find((candidate) => candidate.id === doc.id))) },
  })),
  edit: (id, content) => set(({ state, pending }) => {
    if (!state) return {}
    const current = state.documents.find((doc) => doc.id === id)
    if (!current) return {}
    const revision = current.revision + 1
    const updated = { ...current, content, revision }
    const request = { id, content, revision }
    return {
      state: { ...state, documents: state.documents.map((doc) => doc.id === id ? updated : doc) },
      pending: { ...pending, [id]: request },
    }
  }),
  flush: async () => {
    const edits = Object.values(get().pending)
    if (!edits.length) return
    set(({ pending }) => {
      const next = { ...pending }
      for (const edit of edits) if (next[edit.id]?.revision === edit.revision) delete next[edit.id]
      return { pending: next }
    })
    if (!isMyGo()) return
    try { get().setState(await Desktop.edit(edits)) }
    catch (error) {
      set(({ pending }) => ({ pending: { ...Object.fromEntries(edits.map((edit) => [edit.id, edit])), ...pending } }))
      throw error
    }
  },
  command: async (name, id = "", argument = "") => {
    await get().flush()
    if (!isMyGo()) return
    get().setState(await Desktop.command(name, id, argument))
  },
  preferences: async (mode, fontSize, sidebar) => {
    const current = get().state
    if (current) set({ state: { ...current, settings: { ...current.settings, mode, fontSize, sidebar } } })
    if (isMyGo()) get().setState(await Desktop.preferences(mode, fontSize, sidebar))
  },
  reorder: async (ids) => {
    const current = get().state
    if (!current) return
    const byId = new Map(current.documents.map((doc) => [doc.id, doc]))
    set({ state: { ...current, documents: ids.map((id) => byId.get(id)!).filter(Boolean) } })
    if (isMyGo()) get().setState(await Desktop.reorder(ids))
  },
}))

let flushTimer: ReturnType<typeof setTimeout> | undefined
export function queueEditFlush() {
  clearTimeout(flushTimer)
  flushTimer = setTimeout(() => { useWorkspace.getState().flush().catch(showError) }, 280)
}
export function showError(error: unknown) {
  const state = useWorkspace.getState().state
  if (!state) return
  useWorkspace.setState({ state: { ...state, message: error instanceof Error ? error.message : String(error), messageError: true } })
}
