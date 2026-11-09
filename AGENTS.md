# AGENTS.md

git-k 是一个用 Go 写原生界面的 Git 客户端：交互骨架对齐 git-cola，渲染和控件来自
[mygo](https://github.com/egoist/mygo) 的 `ui` 包。目标是小、快、无 WebView、无 cgo。

## 技术约束

- **无 cgo**：所有构建都设 `CGO_ENABLED=0`。不要 `import "C"`，不要引入需要 cgo 的依赖。
- **无 WebView**：窗口内容是 `mygo.NewWindow(WindowOptions{Content: ui.View(...)})`，
  不加载 HTML/JS。不要给 `mygo.json` 打开 webview。
- **Go 1.27.3 / mygo v0.8.3**：`go.mod` 固定 mygo 版本，和 `db-g` 一致。升级前先跑
  `go test ./...` 和一次真实窗口冒烟。
- **单仓库、单窗口**：一个进程打开一个工作树。不要引入全局仓库状态。
- **git 后端是 CLI**：所有操作走 `git` 子进程，不实现对象数据库，也不引入 go-git。
  这样行为、错误信息和钩子都与用户命令行里的 git 一致。

## 常用命令

```sh
make run      # CGO_ENABLED=0 go run . [仓库路径]
make test     # CGO_ENABLED=0 go test ./...
make vet      # CGO_ENABLED=0 go vet ./...
make build    # 静态发布二进制 ./git-k（-trimpath -tags mygo_noinspector -s -w）
make fmt      # go fmt ./...
```

发布构建不要带 inspector：`mygo_noinspector` 同时会关掉自动化插件。

## 目录

```
main.go              mygo 应用、窗口、命令行参数
app/gitx/            唯一的 git 边界：发现仓库、状态、diff、stage、commit、草稿
  repo.go            运行器、环境变量、Repo 元数据
  status.go          porcelain v2 -z 解析与分组
  diff.go            三种 diff 命令与大小上限
  stage.go           add / restore --staged / unborn 的 rm --cached
  commit.go          commit -F -、amend、GIT_K_MSG 草稿
ui/                  窗口状态与视图
  root.go            根状态、刷新、异步、watch、快捷键、git 动作
  shell.go           顶栏、状态列表、提交面板、diff 面板、空态
ui/ui_test.go        无窗口的视图 + 临时仓库端到端测试
```

## 数据流与线程

- 视图是状态的纯函数：`root.View(c)` 只读 `root` 的字段、发请求，不阻塞。
- git 查询/修改在 goroutine 里跑；结果只能通过 `root.update`（有窗口时是
  `Window.Update`，测试里同步执行）回主线程改状态。
- 每个异步请求带 `gen`：`statusGen`、`diffGen`。回调先比 gen，旧结果直接丢弃。
- 测试（`win == nil`）时 `update` / `background` 同步执行，保证 `ui.Tester` 可预测；
  不要为了测试另写一套路径。
- `syncDiff` 每帧在列表构建后运行：当前行或 `Status.DiffKey` 变了才重新取 patch。
  没有当前行必须 `resetDiff()`，否则提交后会留下旧 diff（已有回归测试）。
- 视图里不要保存 `ui.Element`；它会过期。持久状态放 `root`，用 `ui.ListState` /
  `editor.State` / `editor.DiffState` 保存控件自身状态。

## gitx 规则

- 所有命令在 `Repo.Root` 下执行，路径参数前加 `--`。
- 环境固定：`GIT_OPTIONAL_LOCKS=0`、`GIT_TERMINAL_PROMPT=0`、
  `GIT_LITERAL_PATHSPECS=1`、`LC_ALL=C`。因此文件名里的 `*`、`:(...)` 不会被当成
  pathspec。
- 状态只跑一条：
  `git -c core.quotepath=false status --porcelain=v2 -z --branch --untracked-files=all --ignore-submodules=none`。
  分支、upstream、ahead/behind、所有变更都从这一条解析。
- porcelain v2 规则：
  - `1` 记录：`X != '.'` 进「已暂存」，`Y != '.'` 进「已修改」；同一个文件可以同时
    出现在两组，`Row.Key` 必须带组，不能用路径当唯一键。
  - `2` 记录：紧跟其后的 NUL 字段是原路径（重命名/复制）。
  - `u` 进「冲突」，`?` 进「未跟踪」，`!` 忽略。
  - `-z` 下路径是原始字节，只按 `NUL` 切；显示时用 `gitx.DisplayPath` 转义换行/制表。
- diff：
  - staged → `git diff --cached`；modified → `git diff`；untracked →
    `git diff --no-index /dev/null <path>`；冲突 → `git diff --cc`。
  - `git diff` 退出码 1 表示有差异，是成功；只有其他非零码才是错误。
  - patch 上限 `MaxPatchSize`（8 MiB）。超限返回 `ErrPatchTooLarge`，不要解析。
  - 显示用 `editor.ParsePatch` + `DiffState.SetPatch`，保留 hunk 头和文件真实行号。
    `@@@` combined diff 解析失败时回退到只读原文，不要自己实现 combined 解析。
- stage/unstage 多选合并成一个进程；删除用 `git add -A`。
- unborn HEAD 时 `git restore --staged` 不可用，改用
  `git rm -r --cached --ignore-unmatch`。
- 提交用 `git commit -F -`，消息走 stdin；不要拼 shell 字符串，不要写临时消息文件。
  提交钩子可能很久，超时给 10 分钟，期间 UI 显示忙碌态。

## UI 规则

- 布局：上边 `SplitPercent`（状态 / 提交），下边 diff；两个 split 都要 `.Fill()`，
  否则窗口底部会留空。
- 状态列表用 `ui.List` + `ListState.Header` 做粘性分组，`Selection` 多选，
  `Key` 用 `Row.Key`，`Label` 给读屏和首字母定位。不要为每个文件建真实控件。
- 状态字母 `A/M/D/R/C/U/?` 用主题语义色（`Success/Warning/Danger/Accent/TextMuted`），
  不要在组件里写死十六进制。
- 提交编辑器是 `editor.View` + `editor.Buffer`；第一行摘要，空行后正文，软换行、
  无行号。草稿 500ms 防抖写入 `.git/GIT_K_MSG`，提交成功后清空。
- 快捷键对齐 git-cola：`Ctrl+S` 暂存、`Ctrl+Shift+S` 全部暂存、`Ctrl+U` 取消暂存、
  `Ctrl+R` / `F5` 刷新、`Ctrl+Enter` 提交。用 `Context.Shortcut`，不要全局 hook。
- 错误必须可见：状态错误在列表里，diff 错误在 diff 面板，提交失败在提交面板；
  不要把错误吞掉或只写日志。
- 中文是界面语言；代码标识符和注释用英文，说明性注释可以中文。避免无意义文案。

## 测试与验收

- `app/gitx/status_test.go`：porcelain v2 字节级 fixture，覆盖部分暂存、重命名、
  冲突、未跟踪、空格/换行文件名、unborn。
- `app/gitx/gitx_test.go`：`t.TempDir()` 里真实 `git init`，验证 stage/unstage/
  diff/commit/amend、字面量 pathspec、草稿往返。
- `ui/ui_test.go`：`ui.NewTester` 无窗口跑完整视图；用临时仓库验证三栏出现、
  diff 加载、暂存后计数、提交后状态清空且旧 diff 被清掉。
- 测试必须隔离用户配置：`GIT_CONFIG_GLOBAL=/dev/null`、`GIT_CONFIG_SYSTEM=/dev/null`，
  固定 author/committer。不要依赖网络、桌面或已有仓库。
- 改动 UI 或 git 边界后必须至少做一次真实窗口冒烟（`make run` 或构建后启动），
  并在临时仓库里走一遍「改文件 → 看 status → 看 diff → 写信息 → 提交」。
- 不要写只验证「不 panic」「长度变大」的测试；断言分组、路径、计数、提交内容。

## 当前范围与已知限制

已实现：仓库发现、porcelain v2 状态四组、多选暂存/取消暂存/全部、三种 diff、
编辑器草稿、commit/amend、文件监听刷新、快捷键、空态和错误提示。

未实现（不要假装支持）：部分暂存（hunk/line）、历史/log、分支切换、stash、
merge/rebase/cherry-pick 操作、冲突 combined diff 的专用渲染、图片 diff、
子模块操作、提交模板/签名/spellcheck、多窗口/多仓库。
