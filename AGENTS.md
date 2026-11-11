# AGENTS.md

git-k 是一个用 Go 写原生界面的 Git 客户端：交互骨架对齐 git-cola，渲染和控件来自
[mygo](https://github.com/egoist/mygo) 的 `ui` 包。目标是小、快、无 WebView、无 cgo。

## 技术约束

- **无 cgo**：所有构建都设 `CGO_ENABLED=0`。不要 `import "C"`，不要引入需要 cgo 的依赖。
- **无 WebView**：窗口内容是 `mygo.NewWindow(WindowOptions{Content: ui.View(...)})`，
  不加载 HTML/JS。不要给 `mygo.json` 打开 webview。
- **Go 1.27.3 / mygo v0.8.3**：`go.mod` 固定 mygo 版本，和 `db-g` 一致。升级前先跑
  `go test ./...` 和一次真实窗口冒烟。
- **单窗口、多标签**：一个进程一个窗口，每个 tab 绑定一个工作树；同一仓库只能有一个
  tab（按规范化后的 `git rev-parse --show-toplevel` 去重）。窗口级共享状态只放在
  `ui.App`，仓库级状态放各自的 `ui.root`。
- **git 后端是 CLI**：所有操作走 `git` 子进程，不实现对象数据库，也不引入 go-git。
  这样行为、错误信息和钩子都与用户命令行里的 git 一致。
- **图标**：`assets/icon.svg` 是设计源；`tools/genicon` 用 `x/image/vector` 按同一
  几何/配色光栅化出 256 与 512 两张 PNG，不依赖外部 SVG 渲染器。改图标时两份一起改，
  再跑 `make icon`。`main.go` 用 `//go:embed assets/icon-256.png` 设置窗口与 Dock 图标，
  `mygo.json` 的 `icon` 指向 512。`main_test.go` 会抽样像素，防止图标画空。

## 常用命令

```sh
make run      # CGO_ENABLED=0 go run -tags mygo_noinspector . -d test-repo
              # 也可 go run . -d <目录>：打开该目录所在的仓库，便于用 test-repo 调试
make test     # CGO_ENABLED=0 go test ./...
make vet      # CGO_ENABLED=0 go vet ./...
make build    # 静态发布二进制 ./git-k（-trimpath -tags mygo_noinspector -s -w）
make release  # 交叉编译 6 个平台到 dist/，tar.gz/zip + SHA256SUMS
make fmt      # go fmt ./...
make icon     # go run ./tools/genicon，重生成 assets/icon-{256,512}.png
```

发布构建不要带 inspector：`mygo_noinspector` 同时会关掉自动化插件。

`make release` 默认构建 linux/darwin/windows 的 amd64+arm64，产物名为
`git-k_<version>_<goos>_<goarch>.{tar.gz,zip}`，附带 LICENSE/Readme；可用
`RELEASE_PLATFORMS` 覆盖（逗号或空格分隔），例如
`make release RELEASE_PLATFORMS=linux/amd64,darwin/arm64`。版本来自
`git describe --tags --always --dirty`。

## 目录

```
main.go              mygo 应用、窗口、命令行参数、嵌入窗口图标
assets/icon.svg      应用图标设计源：512 圆角底 + 紫→蓝 K
assets/icon-{256,512}.png  由 tools/genicon 生成的位图
tools/genicon/       与 SVG 同一几何的纯 Go 光栅化器
app/gitx/            唯一的 git 边界：发现仓库、状态、diff、stage、commit、push、草稿
app/settings/        通用 JSON 配置存储：默认值、合并、防抖原子写入
  repo.go            运行器、环境变量、Repo 元数据
  status.go          porcelain v2 -z 解析与分组
  diff.go            三种 diff 命令与大小上限
  stage.go           add / restore --staged / unborn 的 rm --cached
  commit.go          commit -F -、amend、GIT_K_MSG 草稿
ui/                  窗口状态与视图
  app.go             单窗口多 tab：去重、切换、关闭、标题、单实例请求
  root.go            单个 tab 的状态、刷新、异步、watch、快捷键、git 动作
  shell.go           顶栏、状态列表、提交面板、diff 面板、空态
ui/app_test.go       tab 去重/切换/共享布局测试
ui/ui_test.go        无窗口的视图 + 临时仓库端到端测试
```

## 数据流与线程

- 视图是状态的纯函数：`App.View(c)` 画 tab 条和当前 tab；`root.View(c)` 只读
  `root` 的字段、发请求，不阻塞。
- 启动顺序：`NewRootLazy` 只打开仓库，窗口先显示；`Attach` 后异步读第一次
  status。文件监听在第一次 status 回来后才启动，避免和窗口抢 CPU。测试用
  `NewRoot`（同步）保持确定性。
- 单实例：`main` 先 `RequestSingleInstanceLock`，后启动的进程把 args + cwd 转给
  第一个进程后退出；`OnSecondInstance` 解析参数、按仓库根去重 tab、切换并
  `win.Show/Focus`。转发路径相对第二个进程的 cwd 解析。
- 首页（无 tab）显示应用图标和最近打开仓库；`settings.Config.Recent` 保存规范化仓库
  根，`App.touchRecent` 去重后放最前，最多 20 个；点击直接作为 tab 打开。首页条目
  右键「删除记录」只删历史，不动仓库。
- 有 tab 时顶栏「加载仓库」用 `ui.Popover` 列出最近记录中当前未打开的仓库；点击即
  作为 tab 打开。`App.closedRecent` 过滤已打开仓库。
- 窗口位置/大小存 `settings.Config.Window`；`App.Attach` 注册 OnMove/OnResize 保存，
  `main` 启动时用 `windowPosition` 校验仍有部分在屏幕内才恢复，否则居中。
- tab：`App.open` 用 `NewRootLazy` 打开仓库，`canonicalKey` 解析 symlink 后比较；
  同仓库直接激活已有 tab。顶栏「打开仓库…」走同一个入口。关闭 tab 调
  `root.close`，异步结果经 `closed` 丢弃。打开失败用 `Context.Toast` 提示，不弹确认框。
- 布局比例是所有 tab 共享的 `settings.Store` 值；切 tab 时 `root.loadLayout` 取最新。
- git 查询/修改在 goroutine 里跑；结果只能通过 `root.update`（有窗口时是
  `Window.Update`，测试里同步执行）回主线程改状态。
- 每个异步请求带 `gen`：`statusGen`、`diffGen`。回调先比 gen，旧结果直接丢弃。
- 切换 diff 时先重置 `root.diff` / `root.raw` 再装载新 patch，避免旧文件的大缓冲
  叠加；离开汇总时清掉 `groupFiles` / `groupStat`。
- 测试（`win == nil`）时 `update` / `background` 同步执行，保证 `ui.Tester` 可预测；
  不要为了测试另写一套路径。
- `syncDiff` 每帧在列表构建后运行：当前行或 `Status.DiffKey` 变了才重新取 patch。
  没有当前行必须 `resetDiff()`，否则提交后会留下旧 diff（已有回归测试）。
- 视图里不要保存 `ui.Element`；它会过期。持久状态放 `root`，用 `ui.ListState` /
  `editor.State` / `editor.DiffState` 保存控件自身状态。

## 配置规则

- `app/settings` 是唯一的配置读写边界：`Config` 新增字段即可扩展；`Open` 对缺失/
  损坏/缺字段的文件回退默认值；`Update` 防抖 400ms 后原子写入
  `$XDG_CONFIG_HOME/git-k/settings.json`（`os.UserConfigDir`），`Flush` 立即落盘。
- 主程序在 `App.OnBeforeQuit` 和 `Run` 返回后都调用 `Flush`。
- 窗口分隔条比例 `sidePercent` / `commitPercent` 在每次 `View` 末尾比较并保存；
  加载时 0/100 视为无效配置，保存时钳到 1..99，避免下次启动隐藏面板。
- 测试用 `settings.Open("")` 或临时文件，不要写用户真实配置。

## gitx 规则

- 所有命令在 `Repo.Root` 下执行，路径参数前加 `--`。
- Windows 上所有 git 子进程走 `hideConsole`（`CREATE_NO_WINDOW` + `HideWindow`），
  否则每次 status/diff 都会闪一个黑色控制台窗口，并增加进程启动开销。
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
- 推送/拉取用 `git push` / `git pull`（`Repo.Push` / `Repo.Pull`），不拼 shell；
  失败信息写回 `note`，超时 5 分钟。环境设 `GIT_MERGE_AUTOEDIT=no`，避免 pull 打开
  编辑器。
- 修正提交不再用勾选框：`Ctrl+M` 调 `Repo.LoadAmend` 读取 HEAD 的 message 并把 index
  reset 到 HEAD（工作区不动），UI 进入 `amendMode`；用户改完提交时才走
  `git commit --amend`。

## UI 规则

- 布局对齐 git-cola：左列上方提交信息、下方 diff，右列整块状态。左侧宽度用
  `SplitPercent`，左列上下用 `SplitVerticalPercent`；两个 split 都要 `.Fill()`，
  否则窗口底部会留空。分隔条可拖动调整比例。
- 没有实现 Qt 那种可浮动、可停靠、可叠加的 dock 系统：目前只有可拖动的 split。
  如要 dock，需要另做布局持久化、拖拽命中和浮动面板，工作量大，先不做。
- 状态列表用 `ui.List` + `ListState.Header` 做粘性分组，`Selection` 多选，
  `Key` 用 `Row.Key`，`Label` 给读屏和首字母定位。不要为每个文件建真实控件。
- 双击状态行切换暂存：已修改/未跟踪 → 暂存，已暂存 → 取消暂存；冲突行不响应。
  双击用行元素的 `RowDoubleClicked`，动作在 `ui.List` 构建返回后执行，因为测试里
  暂存会同步刷新 `entries`。
- 状态列表里的分组行（已暂存/冲突/已修改/未跟踪）本身可点：点击后 diff 区汇总
  该组所有文件的 patch（`root.groupDiffs`），再点同一行或点文件行退出。分组行用
  `ui.Button` 铺满整行来接收点击——`ListState.Header` 的行不参与命中测试。
  汇总是可滚动的堆叠：顶部是 git 风格的 diffstat（`formatDiffStat`），下面是
  每个文件的路径和独立的 `editor.DiffView`（`FitContent`），配色与单文件一致。
  `editor.DiffState` 一次只接受一个文件，所以按文件各存一个 state。列表变选
  （`List.Changed`）退出汇总。
- `Ctrl+Q` 退出应用（`root.quit`，测试里替换为钩子）。
- 右键菜单动作作用于多选：右键的行在选中集合里时，暂存/取消暂存/删除都作用于全部
  选中行（`root.actionRows`）；删除只取其中的未跟踪行。
- 未跟踪行右键有「删除文件…」，先用 `ui.AlertDialog` 二次确认，再走
  `Repo.RemoveFiles`（一次 `git clean -q -f -- <paths...>`）。`git clean` 不会删已跟踪
  或已忽略的路径，状态过期也不会误删；失败写 `note`。
- 状态字母 `A/M/D/R/C/U/?` 用主题语义色（`Success/Warning/Danger/Accent/TextMuted`），
  不要在组件里写死十六进制。已暂存的 `M` 和 `R` 用 `Success`，和暂存的新增一致；
  工作区的 `M` 仍用 `Warning`，未暂存的重命名仍用 `Accent`。
- 提交信息是两个输入框：单行 `TextInput`（`root.summary`，提交摘要）和
  `editor.View` + `editor.Buffer`（`root.body`，详细描述）。提交时由
  `root.message()` 拼成「摘要 + 空行 + 正文」；空正文时只有摘要。草稿 500ms 防抖
  写入 `.git/GIT_K_MSG`，读回时由 `splitMessage()` 拆成两个框，提交成功后清空。
- 快捷键对齐 git-cola：`Ctrl+S` 暂存、`Ctrl+Shift+S` 全部暂存、`Ctrl+U` 取消暂存、
  `Ctrl+R` / `F5` 刷新、`Ctrl+Enter` 提交、`Ctrl+M` 加载上次提交进入修正模式、
  `Ctrl+P` 推送、`Ctrl+L` 拉取。用 `Context.Shortcut`，不要全局 hook。
- 错误必须可见：状态错误在列表里，diff 错误在 diff 面板，提交失败在提交面板；
  不要把错误吞掉或只写日志。`root.setNote` 的提示（提交成功、操作失败等）显示 3 秒
  后自动消失；新的提示会刷新计时。
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
编辑器草稿、commit/amend、文件监听刷新、快捷键、空态和错误提示、单窗口多 tab、
单实例启动与命令行转交。

未实现（不要假装支持）：部分暂存（hunk/line）、历史/log、分支切换、stash、
merge/rebase/cherry-pick 操作、冲突 combined diff 的专用渲染、图片 diff、
子模块操作、提交模板/签名/spellcheck、多窗口。
