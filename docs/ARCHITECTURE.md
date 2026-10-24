# Foldmark 架构

桌面框架为 MyGO v0.6.3，负责 Windows 窗口、文件打开、系统对话框与 WebView2。界面采用 React 19 + TypeScript，由 Vite 构建为本地静态资源打入程序；不使用 Electron。

应用以单实例运行。MyGO 的 `OnOpenFile` 接收首次命令行文件参数、系统文件关联入口，以及后续实例转交的文件。原生文件对话框和窗口关闭事件由 MyGO 管理。

`application` 在 MyGO 主线程保留文档、设置和消息的权威状态；文件操作在工作 goroutine 中执行，返回后通过 `Window.Update` 更新状态。`Desktop` 使用 MyGO 生成的 TypeScript 绑定连接 React。Zustand store 即时显示编辑内容，280 毫秒后批量提交带修订号的文本；较慢的状态更新不会覆盖新输入。窗口关闭先刷写待提交内容，再执行逐文档保存决策。

界面组件为 shadcn/ui base-nova 预设生成的本地源码；Lucide、Motion、react-resizable-panels、dnd-kit 和 CodeMirror 分别承担图标、动画、分栏、排序和编辑。阅读区使用 react-markdown、remark-gfm 与 rehype-sanitize；本地图片经 MyGO 自定义协议并由 Go 限定在当前 Markdown 目录内。外部网页由系统浏览器打开。

`internal/storage` 负责 UTF-8 解码、BOM / 换行格式、SHA-256 冲突检测、同目录临时文件替换、文件夹扫描和会话恢复。会话由单个后台写入器延迟合并，关闭时强制写入完成；恢复未保存草稿时保留原文件指纹，防止绕过外部修改检查。

`internal/content` 使用 MyGO 的 Markdown 解析器提取标题与生成导出内容。本地图片限制在 Markdown 文档目录内；HTML 导出嵌入图片，通过标签和属性白名单移除可执行内容，并添加内容安全策略。

`source/mygo.json` 固定应用标识、版本、图标、中文文案、文件关联和前端构建命令。MyGO CLI 生成 Windows x64 可执行文件和 NSIS 安装程序。Go 依赖由 `go.sum` 固定，React 依赖由 `pnpm-lock.yaml` 固定。发布目录包含第三方许可证文本。
