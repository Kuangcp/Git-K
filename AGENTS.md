# AGENTS.md

git-k 是一个用 Go 写原生界面的 Git 客户端：交互骨架对齐 git-cola，渲染和控件来自
[mygo](https://github.com/egoist/mygo) 的 `ui` 包。目标是小、快、无 WebView、无 cgo。

## 技术约束

- **无 cgo**：所有构建都设 `CGO_ENABLED=0`。不要 `import "C"`，不要引入需要 cgo 的依赖。
- **无 WebView**：窗口内容是 `mygo.NewWindow(WindowOptions{Content: ui.View(...)})`，
  不加载 HTML/JS。不要给 `mygo.json` 打开 webview。
- **无边框窗口**：主窗口 `Frameless: true`，由 `ui/titlebar.go` 自绘 32px 标题栏：
  `DragWindow` 拖动/双击最大化；正中间显示当前 tab 的仓库名；左侧图标和应用名 Git-K，右侧是设置、
  最小化、最大化/还原、关闭。不要再依赖系统标题栏；home 与仓库页共用同一条标题栏。
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
- **Windows 图标**：`make icon` 还会跑 `tools/winres`，生成 `assets/icon.ico` 和
  `icon_windows_{amd64,arm64}.syso`。Windows 固定到任务栏读取的是 exe 的图标资源，
  运行时 `SetIcon` 不够；这两个 `.syso` 必须随仓库提交，`go build` 会自动链接对应
  GOARCH 的文件。

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
main.go              CLI、单实例、窗口生命周期、settings 注入
assets/              图标设计源、PNG、ICO
icon_windows_*.syso  嵌入 exe 的 Windows 图标资源
tools/genicon/       与 SVG 同一几何的纯 Go 光栅化器
tools/winres/        生成 icon.ico 与 Windows 图标 .syso 资源
app/gitx/            唯一的 git 边界
  repo.go            运行器、固定环境、Repo 元数据、Windows 控制台隐藏
  status.go          porcelain v2 -z 解析与分组
  diff.go            三种 diff 命令与大小上限
  stage.go           add / restore --staged / unborn 的 rm --cached
  commit.go          commit -F -、LoadAmend、GIT_K_MSG 草稿
  push.go/pull.go    push / pull
  remove.go          untracked 文件删除
  history.go         提交图 log、提交信息、文件列表、单文件 diff
app/settings/        通用 JSON 配置存储：默认值、合并、防抖原子写入；Theme 预设
ui/                  窗口与 tab
  app.go             单窗口多 tab：去重、切换、关闭、标题、窗口 bounds
  titlebar.go        无边框窗口标题栏、拖动区、窗口控制
  theme.go           Lobe 主题预设解析与无边框窗口底色
  settings_view.go   设置弹窗（左侧外观/渲染/快捷键分类，右侧具体设置）
  tabs_view.go       tab 条视图
  home.go            首页、最近记录、加载历史仓库弹窗、选择目录
  root.go            单个 tab 的根状态、构造、View、布局、openRepo、异步桥
  status.go          status 刷新/应用、列表项构建、当前行
  diff.go            单文件/分组 diff 的加载、解析、统计
  changes.go         选中行、暂存/取消暂存、删除、索引操作
  commit.go          提交、修正提交、草稿、message 拆分
  remote.go          pull / push
  watch.go           工作树与 .git 监听
  feedback.go        busy、3 秒自动消失的 note
  keymap.go          可配置快捷键：动作表、键位解析/格式化、持久化
  commands.go        仓库快捷键分发、选择目录
  shell.go           顶栏、图标按钮、底栏、空态、通用视图 helpers
  status_view.go     状态面板与右键/删除确认
  commit_view.go     提交面板
  diff_view.go       差异面板与分组汇总；工作区/提交共用差异渲染
  branch.go          分支 tab：提交图、提交详情、文件列表、提交 diff
  stash.go           Stash tab：Stash 列表、变更文件列表、单文件 diff、Pop/Drop
ui/lobe/             vendored Lobe UI 主题 token（MIT）
ui/app_test.go       tab、首页、最近记录、单实例参数相关测试
ui/ui_test.go        无窗口视图 + 临时仓库端到端测试
```


## 架构

分层与依赖方向：

```
main.go ──> ui.App / ui.root ──> app/gitx
                    │
                    └──────────> app/settings
```

- `main.go` 只做进程级工作：参数、单实例锁、唯一窗口、settings store、恢复窗口位置。
- `ui.App` 是窗口级状态：tab 列表、活动 tab、首页、最近记录、加载仓库弹窗、标题。
- `ui.root` 是单个仓库 tab 的控制器：repo、status、diff、编辑器状态、watcher、异步请求。
- `ui/*_view.go` 与 `shell.go` 是视图函数，只读状态、发请求；不直接跑 git，不阻塞。
- `app/gitx` 不知道 UI，也不依赖 `app/settings`；所有 git 命令、路径、错误都在这里收口。
- `app/settings` 不知道仓库和 UI 语义，只负责 JSON 的默认值、更新和原子写入。

约束：

- **状态归属**：跨 tab 的只放 `App`；仓库相关只放各自的 `root`。不要把仓库状态塞回
  `App`，也不要在多个 root 间共享可变指针（布局比例通过 settings 同步）。
- **异步**：git 只在 goroutine 跑，结果经 `root.update` 回主线程；所有请求带
  `statusGen` / `diffGen`，旧结果直接丢弃。关闭 tab 后 `closed` 丢弃所有回调。
- **主题**：`App.applyTheme` 在帧首把 `settings.Theme`（system/light/dark ×
  default/ocean/forest/warm）解析成 vendored Lobe token，覆盖中性色后 `c.SetTheme`。
  设置弹窗修改后写回 `settings.Store` 并更新无边框窗口底色；不要再散落硬编码颜色。
- **视图**：每帧重新构建，不缓存 `ui.Element`。持久控件状态放 `root` 的
  `ui.ListState` / `editor.State` / `editor.DiffState`。
- **文件边界**：新增功能按职责落到对应文件；单个文件接近 400 行或出现第二种职责时拆分，
  不要继续堆回 `root.go`。
- **性能**：启动先出窗口再读 status；文件监听等第一次 status 回来后再开；分组 diff
  最多 4 个并发 git 进程；首页最近记录缓存路径检查，避免每帧 stat；切换 diff 时释放
  旧 buffer；Windows 子进程必须 `hideConsole`。

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
  根，`App.touchRecent` 去重后放最前，最多 20 个；每条记录前是带仓库名首字母的
  文件夹图标，点击直接作为 tab 打开。首页条目
  右键「删除记录」只删历史，不动仓库。
- 有 tab 时顶栏的「加载仓库」图标用 `ui.Popover` 列出最近记录中当前未打开的仓库；点击即
  作为 tab 打开。`App.closedRecent` 过滤已打开仓库。
- 外观预设存 `settings.Config.Theme`：`Mode` 为 system/light/dark，`Scheme` 为
  default/ocean/forest/warm；设置弹窗修改即写回，启动时由 `App.applyTheme` 应用。
- 渲染方式存 `settings.Config.SoftwareRender`：设置为关闭 GPU 时，`main` 在创建窗口前
  设 `MYGO_GPU=0`，走 CPU 光栅化；`MYGO_GPU` 只在首帧选渲染器时读取，因此修改后
  需重启应用。设置弹窗「渲染」里的「启用 GPU 渲染」开关默认开启，用于规避部分
  Linux/X11 驱动（如旧 NVIDIA + 合成器）在文本密集页面的黑屏闪烁。
- 窗口位置/大小存 `settings.Config.Window`；`App.Attach` 注册 OnMove/OnResize 保存，
  `main` 启动时用 `windowPosition` 校验仍有部分在屏幕内才恢复，否则居中。
- tab：`App.open` 用 `NewRootLazy` 打开仓库（`gitx.Open` 的 `rev-parse --show-toplevel`
  会把任意子目录解析到仓库根），`canonicalKey` 解析 symlink 后比较；同仓库直接激活
  已有 tab。从第三级子目录启动也等价于从仓库根启动。顶栏「打开仓库…」图标走同一个入口。关闭 tab 调
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

- `app/settings` 是唯一的配置读写边界：`Config` 新增字段即可扩展（例如
  `Shortcuts`）；`Open` 对缺失/
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
- MSYS2/Cygwin 下的 `/d/Code/...`、`/cygdrive/d/Code/...` 路径在 `gitx.Open` 里经
  `nativePath` 转成 `D:\Code\...` 后再 `filepath.Abs`；`git rev-parse` 的输出也走同一
  转换，避免 Windows 上 `chdir D:\d\...` 找不到目录。
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
- 历史浏览只读：`git log --topo-order --max-count=300`，主线分支时 rev 为
  `refs/heads/<branch>`（含已合入的其它分支提交），ALL 时并集 `HEAD --branches`；NUL 分隔
  字段解析出 hash/parents/author/time/refs/subject；lane 由 UI 计算。提交文件用
  `git show --name-status -z --format=`，单文件补丁用
  `git diff <first-parent> <hash> -- <path>`（根提交用 `git show --patch`）。
  这些查询都不改仓库。
- 打开仓库时，若存在 `MERGE_HEAD` 则用 `.git/MERGE_MSG` 初始化提交信息（去掉
  `#` 开头的冲突说明），除非 `GIT_K_MSG` 的 mtime 比 MERGE_MSG 新（用户已编辑）；
  这样合并冲突后提交的仍是 merge commit 信息。提交仍用 `git commit -F -`，消息走
  stdin；不要拼 shell 字符串，不要写临时消息文件。
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
- 顶栏右侧全部用图标按钮，按功能分组并用 1px 竖线分隔：全部暂存/全部取消、
  加载最近仓库/打开仓库、拉取/推送/刷新。无内容可操作时按钮置灰，tooltip 带当前键位。
- 每个仓库 tab 顶栏左侧有「工作区 / 分支 / Stash」二级 tab（`root.pane`）。工作区就是
  原来的 commit/diff/status 三栏；分支 tab 左栏是提交图，右栏上下分为提交详情+文件列表和
  文件 diff。两个 split 比例存 `Layout.BranchGraphPercent` / `Layout.BranchDetailPercent`，
  与工作区比例互不影响。切换 tab 时重新读 log，但保留仍存在的选中提交。
- Stash tab 复用工作区的左右比例（`Layout.SidePercent` / `CommitPercent`）：左上是
  Stash 列表（替代提交面板），左下是选中文件的差异，右侧是选中 Stash 的变更文件
  列表（`stashStatusPane`，不再是工作区 status）。Stash 行显示 `stash@{n}`、基提交
  短 ID（第一父提交，tooltip 是完整 ID）、时间和消息；点击行进入预览，默认选中第一
  个文件，右侧顶部有聚合条（全部文件数 + 新增/修改/删除计数），点击聚合条堆叠展示
  整个 stash 的 diff；点击单个文件切到该文件 diff。差异头的「工作区差异」回到工作区
  diff 和工作区状态。右键 `Pop` / `Drop`；Drop 先走 `ui.AlertDialog` 二次确认。Pop/Drop 用
  列表里的 `stash@{n}` 作为参数，成功后刷新状态并重读 Stash 列表（`applyStatus` 在
  Stash tab 会触发 `loadStashes`）。
- Stash 后端在 `app/gitx/stash.go`：`Stashes` 用
  `git stash list --format=%gd%x00%H%x00%P%x00%ct%x00%s` 一次读回，基提交取 `%P` 的
  第一父；`StashDiff` 用 `git stash show -p --include-untracked`（旧 git 回退到不带
  `--include-untracked`）；`StashPop` / `StashDrop` 只接受 `stash@{n}` 形式的引用。
  Stash diff 是多文件 patch，`groupFilesFromPatch` 一次 `editor.ParsePatch` 后按文件
  建 diff，并保留每个文件的 status 字母（A/M/D/R）与统计，不按文件逐个起 git 进程；
  右侧聚合条按 status 汇总新增/修改/删除数量，文件列表按这些项渲染，选中后直接展示
  对应文件的 fitted diff，聚合时复用 `groupDiffPane`。
- 分支页复用工作区的差异渲染（`diffRender` / `showDiffBody`、`groupDiffPane`）和
  文件行（`fileRowContent`）；提交详情、文件列表、diff 的选择状态各自放在
  `root.branch`。默认选中「修改的文件」头，堆叠展示全部文件补丁；点单个文件再切到
  该文件 diff。提交 diff 一律对第一父提交计算，避免 combined diff；补丁超限仍回退
  只读原文。提交行显示指向它的本地分支/tag 标记（HEAD 当前分支高亮），选中行用
  淡色背景；右键可复制完整 Commit ID。
- `Ctrl+Q` 最小化窗口，不退出应用（`App.minimizeWindow`，测试里替换
  `app.minimize` 钩子）。窗口级动作（最小化/关闭 tab/切 tab）在 `App.shortcuts`
  分发，仓库级动作在 `root.shortcuts` 分发。
- 右键菜单动作作用于多选：右键的行在选中集合里时，暂存/取消暂存/撤销/删除都作用于
  全部选中行（`root.actionRows`）；撤销只取已修改行，删除只取未跟踪行。
- 已修改行右键有「撤销修改…」，先用 `ui.AlertDialog` 二次确认，再走
  `Repo.RevertFiles`（`git restore -- <paths...>`）：只丢弃工作区修改，index 不动，
  因此同一文件已暂存的内容会保留。
- 未跟踪行右键有「删除文件…」，先用 `ui.AlertDialog` 二次确认，再走
  `Repo.RemoveFiles`（一次 `git clean -q -f -- <paths...>`）。`git clean` 不会删已跟踪
  或已忽略的路径，状态过期也不会误删；失败写 `note`。
- 仓库路径放底栏，紧跟分支/ahead/behind 之后；仓库名只在标题栏和 tab 出现，顶栏不再
  重复高亮仓库名。状态字母 `A/M/D/R/C/U/?` 用主题语义色（`Success/Warning/Danger/Accent/TextMuted`），
  不要在组件里写死十六进制。已暂存的 `M` 和 `R` 用 `Success`，和暂存的新增一致；
  工作区的 `M` 仍用 `Warning`，未暂存的重命名仍用 `Accent`。
- 提交信息是两个输入框：单行 `TextInput`（`root.summary`，提交摘要）和
  `editor.View` + `editor.Buffer`（`root.body`，详细描述）。提交时由
  `root.message()` 拼成「摘要 + 空行 + 正文」；空正文时只有摘要。草稿 500ms 防抖
  写入 `.git/GIT_K_MSG`，读回时由 `splitMessage()` 拆成两个框，提交成功后清空。
- 快捷键默认对齐 git-cola：`Ctrl+S` 暂存、`Ctrl+Shift+S` 全部暂存、
  `Ctrl+U` 取消暂存、`Ctrl+R` / `F5` 刷新、`Ctrl+Enter` 提交、`Ctrl+M` 加载上次
  提交进入修正模式、`Ctrl+P` 推送、`Ctrl+L` 拉取、`Ctrl+1` 工作区、`Ctrl+2` 分支、
  `Ctrl+3` Stash。
  全部动作在 `ui/keymap.go` 的
  `keyActions` 里声明（ID、标签、作用域、默认键位），用 `bindings.hit` 走
  `Context.Shortcut`，不要全局 hook。设置弹窗的「快捷键」分区可录制新组合、单项/
  全部恢复默认；同作用域冲突时旧动作自动解绑。键位只把与默认不同的项存进
  `settings.Config.Shortcuts`（`action → "Ctrl+Shift+S"`），启动时由 `NewApp`
  加载。底栏和 tooltip 的提示从 `bindings` 生成，不要写死。
- 错误必须可见：状态错误在列表里，diff 错误在 diff 面板，提交失败在提交面板；
  不要把错误吞掉或只写日志。`root.setNote` 的提示（提交成功、操作失败等）显示 3 秒
  后自动消失；新的提示会刷新计时。
- 中文是界面语言；代码标识符和注释用英文，说明性注释可以中文。避免无意义文案。

## 测试与验收

- `app/gitx/status_test.go`：porcelain v2 字节级 fixture，覆盖部分暂存、重命名、
  冲突、未跟踪、空格/换行文件名、unborn。
- `app/gitx/gitx_test.go`：`t.TempDir()` 里真实 `git init`，验证 stage/unstage/
  diff/commit/amend、字面量 pathspec、草稿往返。
- `app/gitx/history_test.go`：log 顺序/parents/refs、提交信息、name-status 文件、
  根提交与空仓库的 diff。
- `ui/branch_test.go`：Ctrl+2 打开分支页、首个提交与文件列表、点击文件后的 diff、
  Ctrl+1 返回工作区、提交图 lane 分配。
- `app/gitx/stash_test.go`：真实 stash 的列表/基提交/多文件（含 untracked）diff、
  Pop 恢复工作区、Drop 不动工作区、非法引用。
- `ui/stash_test.go`：Ctrl+3 打开 Stash 页、基提交与消息显示、首次预览、右侧 stash
  文件列表与单文件选择、「工作区差异」退出预览、Pop 后清空、Drop 确认框、右键菜单 Pop。
- `ui/ui_test.go`：`ui.NewTester` 无窗口跑完整视图；用临时仓库验证三栏出现、
  diff 加载、暂存后计数、提交后状态清空且旧 diff 被清掉。
- 测试必须隔离用户配置：`GIT_CONFIG_GLOBAL=/dev/null`、`GIT_CONFIG_SYSTEM=/dev/null`，
  固定 author/committer。不要依赖网络、桌面或已有仓库。
- 改动 UI 或 git 边界后必须至少做一次真实窗口冒烟（`make run` 或构建后启动），
  并在临时仓库里走一遍「改文件 → 看 status → 看 diff → 写信息 → 提交」。
- 不要用 xdotool、鼠标移动/点击、截图脚本等自动化方式操作真实窗口做 UI 验收：
  这会抢占用户正在使用的鼠标和键盘。启动窗口后，把需要目视确认的界面变化交给
  用户人工确认；可自动验证的逻辑用 `ui.NewTester` 覆盖。
- 不要写只验证「不 panic」「长度变大」的测试；断言分组、路径、计数、提交内容。

## 当前范围与已知限制

已实现：仓库发现、porcelain v2 状态四组、多选暂存/取消暂存/全部、三种 diff、
编辑器草稿、commit/amend、文件监听刷新、快捷键、空态和错误提示、单窗口多 tab、
单实例启动与命令行转交、分支页的提交图与提交详情浏览、Stash 列表预览与 Pop/Drop。

未实现（不要假装支持）：部分暂存（hunk/line）、分支切换、Stash 创建/apply、
merge/rebase/cherry-pick 操作、冲突 combined diff 的专用渲染、图片 diff、
子模块操作、提交模板/签名/spellcheck、多窗口。
