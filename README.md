# 折页 Foldmark

用于打开、预览和编辑 Markdown 的 Windows 桌面应用。桌面外壳采用 [MyGO](https://github.com/egoist/mygo)，界面采用 React 与系统 WebView2；**不使用 Electron**。浅色纸面与松绿色贯穿侧栏、编辑区和阅读区。

## 运行

- 安装版：`release/0.5.0/windows-amd64/Foldmark Setup 0.5.0 amd64.exe`。
- 免安装版：`release/0.5.0/windows-amd64/Foldmark.exe`，直接运行。
- 安装后，在 Windows 的「打开方式」中选择 **Foldmark** 并设为默认，以后可以双击 `.md` 文件打开。

安装包为当前用户安装，含开始菜单入口和卸载程序。系统需要 WebView2 Runtime；当前版本未签名。

## 已实现

- 打开文件、文件夹浏览、拖入 Markdown、最近文件、多文档切换与拖动排序。
- 可视化编辑 / 源码分栏 / 预览。源码分栏支持拖动并记住宽度、按正文段落双向同步滚动；CodeMirror 提供语法高亮、查找替换和 `/` 快速插入。
- 可视化编辑器由 Tiptap 承载：插入 2×2、3×3、4×4 表格，直接编辑单元格，按 Tab 移动，调整列宽，并可增删行列。保存时写回标准 Markdown 表格。
- 通过工具栏和快捷键插入格式；源码分栏仍可使用斜杠菜单。本地图片可选取、粘贴或拖入，复制到文档旁的 `assets/` 并使用相对路径。
- Markdown 标题、列表、引用、表格、任务清单、代码块、本地图片预览；代码块可复制，图片可点击放大。
- 预览模式可直接查找正文，显示命中数量并逐处跳转；支持跨加粗、链接等行内格式的连续文字。
- 保存、另存为、导出含本地图片的独立 HTML。
- 关闭前保存提示、外部修改冲突提醒、会话和未保存草稿恢复。
- 字数统计、字号调整、专注写作。

| 操作 | 快捷键 |
| --- | --- |
| 新建 / 打开 | Ctrl+N / Ctrl+O |
| 保存 / 另存为 | Ctrl+S / Ctrl+Shift+S |
| 关闭标签 | Ctrl+W |
| 查找 / 替换 | Ctrl+F / Ctrl+H；预览模式下 Ctrl+F 查找渲染正文 |
| 加粗 / 斜体 / 链接 | Ctrl+B / Ctrl+I / Ctrl+K |
| 删除线 / 有序列表 / 无序列表 | Ctrl+Shift+X / Ctrl+Shift+7 / Ctrl+Shift+8 |
| 撤销 / 重做 | Ctrl+Z / Ctrl+Y |
| 专注模式 / 退出 | F11 / Esc |

窄窗口会收起部分格式按钮；窗口关闭会逐一处理未保存文档。

## 文档与草稿

支持 UTF-8 编码，保留原文件 BOM 和 CRLF / LF 换行格式。单文档上限 8 MB，最多同时打开 30 份。文件夹扫描最多 1500 份文档、8 层子目录。

编辑后的恢复草稿约在 600 毫秒后保存在 `%APPDATA%/Foldmark/session.json`；按 `Ctrl+S` 才会写回原文件。草稿恢复不能代替独立备份。若文件已被外部修改，保存时可选择另存副本、取消或覆盖。

图片限文档所在目录及其子目录中的 PNG、JPG、GIF、WebP、BMP，每张不超过 16 MB；插入前需先保存文档。远程图片显示文字占位。导出 HTML 会嵌入本地图片并清理可执行 HTML。公式以原文显示，暂不支持 Mermaid、云同步和协作。[编辑页功能对照](docs/EDITOR_PARITY.md)列出了与语雀写作流程的对应关系及边界。

## 开发与构建

需要 Windows x64、Go **1.27.1+**、Node.js **22.12+**、pnpm 和 WebView2 Runtime。Go 依赖由 `go.sum` 固定，前端依赖由 `pnpm-lock.yaml` 固定。无需 Electron、Rust 或 C 编译器。

```powershell
cd source/frontend
pnpm install --frozen-lockfile
pnpm run build
cd ..
go test ./...
go vet ./...
go tool mygo build -platform windows/amd64 -skip-build-command
```

也可在仓库根目录运行：

```powershell
.\scripts\dev.ps1
.\scripts\build.ps1
.\scripts\smoke.ps1     # 对打包后的程序做原生窗口验证
```

脚本优先使用系统 Go；本工作空间的 `.tools/go` 提供了便携 Go。缓存保存在 `.cache`，不会上传到仓库。首次打包时 MyGO 会下载 NSIS。

测试默认不打开窗口，不访问真实用户配置。`FOLDMARK_DATA_DIR` 可指定独立配置目录；`FOLDMARK_CAPTURE_DIR` 用于窗口截图验证，验证完毕后应用自动退出。

## 项目结构

```text
source/
  main.go                  窗口与系统文件打开入口
  app.go                   文档状态、保存决策、会话恢复
  bridge.go                MyGO 与 React 的类型化通信、本地图片协议
  frontend/                React、shadcn/ui Nova 组件源码与 pnpm 锁文件
  internal/storage/        文件读写、冲突检测、会话持久化
  internal/content/        大纲、图片边界、HTML 导出
  locales/                 中文文案
  assets/                  应用图标
  go.mod / go.sum           固定的 Go 依赖
  mygo.json                Windows 打包与文件关联配置
scripts/                   开发和构建入口
docs/                      架构与验证记录
release/                   可执行文件和安装包（不提交）
```

可将当前目录作为源码仓库使用，也可镜像到 Codeup。依赖缓存、开发工具、测试输出和构建产物已排除，安装包通过发行附件或制品库分发。

验证范围见 [验证记录](docs/VALIDATION.md)。分发免安装程序时请保留同目录的 `THIRD_PARTY_NOTICES.txt`。

界面组件使用本地可编辑的 shadcn/ui Nova 源码；图标为 Lucide，动画为 Motion，分栏为 react-resizable-panels，拖动排序为 dnd-kit，状态管理为 Zustand。可视化编辑与 Markdown 表格使用 Tiptap，源码编辑使用 CodeMirror，预览由 react-markdown、remark-gfm 与 rehype-sanitize 处理。详细依赖及许可见发布目录的 `THIRD_PARTY_NOTICES.txt`。
