<p align="center">
  <img src="assets/icon-256.png" width="128" alt="Git-K 图标">
</p>

<p align="center">
  <a href="Readme.md">简体中文</a> · <a href="Readme.en.md">English</a>
</p>

# Git-K

一个用 Go 写原生界面的 Git 客户端，交互骨架受 [git-cola](https://github.com/git-cola/git-cola) 启发，渲染和控件来自 [mygo](https://github.com/egoist/mygo) 的 `ui` 包。

目标是**小、快、无 WebView、无 cgo**：一个静态二进制，双击即用；所有 Git 操作都走本机的 `git` 子进程，行为、错误信息和钩子与你命令行里的 git 一致。

## 为什么选 Git-K

**一个二进制，原生界面，键盘上一路到底。**

- **原生绘制，无 WebView**：窗口内容由 mygo 直接绘制，不加载 HTML/JS，不需要 Electron、WebView2 或浏览器运行时。
- **无 cgo**：所有发布构建都设 `CGO_ENABLED=0`，Linux / macOS / Windows 的 amd64、arm64 交叉编译产物都是独立可执行文件。
- **和命令行 git 一致**：状态、diff、暂存、提交、推送、拉取全部调用 `git`，不实现对象数据库，也不引入 go-git。
- **单窗口多标签**：一个窗口打开多个仓库，同一仓库自动复用已有标签；从仓库任意子目录启动都会解析到仓库根。
- **工作区对齐 git-cola**：左列提交信息 + 差异，右列状态列表；支持分组 diff、多选暂存、右键撤销/删除。
- **提交图**：分支页展示本地分支的提交图，可按某个分支作为主线或切到 `ALL` 查看全部分支进展；合并提交用菱形和曲线标出。
- **快捷键可改绑**：设置窗口里录制新组合，键位持久化到 `settings.json`；底栏和 tooltip 跟着更新。

## 运行

```sh
git-k                       # 打开当前目录所在的仓库
git-k /path/to/repo         # 打开指定仓库
git-k -d /path/to/repo      # 同上，显式指定目录
git-k -version              # 查看版本
```

已有实例运行时，新进程会把参数转交给它并聚焦已有窗口，不再开第二个进程。

开发运行：

```sh
make run                    # CGO_ENABLED=0 go run . -d test-repo
```

`test-repo` 是一个随仓库提供的示例工作树，便于调试状态列表、diff 和提交。

## 功能

- [x] 仓库发现：从任意子目录解析到 `git rev-parse --show-toplevel`，按规范化路径去重标签
- [x] 状态：`git status --porcelain=v2 -z` 一次读取分支、upstream、ahead/behind 和全部变更；四组展示（已暂存 / 冲突 / 已修改 / 未跟踪）
- [x] 暂存：多选暂存、取消暂存、全部暂存、全部取消；双击行切换暂存；unborn HEAD 的取消暂存走 `git rm --cached`
- [x] 差异：已暂存用 `git diff --cached`，已修改用 `git diff`，未跟踪用 `git diff --no-index`，冲突用 `git diff --cc`；支持分组汇总、diffstat 和补丁大小上限
- [x] 提交：摘要 + 正文两个输入框，`git commit -F -` 走 stdin；草稿防抖写入 `.git/GIT_K_MSG`，重启恢复；合并冲突期间打开会载入 git 的 `MERGE_MSG`，保留 merge 提交信息
- [x] 修正提交：`Ctrl+M` 读取 HEAD 信息并把 index 重置到 HEAD，再次提交走 `git commit --amend`
- [x] 分支页：左栏提交图（HEAD + 本地分支，客户端 lane 布局），右栏提交详情、修改文件列表和单文件 diff；提交 diff 取第一父提交
- [x] 主线切换：提交图上方可选某个本地分支，只看该分支及其已合入的提交；选 `ALL` 查看所有本地分支
- [x] Stash：列出 stash，右侧汇总新增/修改/删除文件数并列出变更文件；点聚合条看整个 stash 的差异，点文件看单文件 diff；右键 Pop / Drop（Drop 二次确认）
- [x] 文件监听：工作树和 `.git` 变化后自动刷新状态，忽略 `objects/`、`logs/` 和常见重型目录
- [x] 右键操作：已修改文件可撤销修改（二次确认，`git restore --`），未跟踪文件可删除（`git clean -f --`）
- [x] 外观：跟随系统 / 浅色 / 深色，默认 / 海洋 / 森林 / 暖阳配色；基于 Lobe UI 设计 token
- [x] 渲染：设置里可关闭 GPU 渲染，改用 CPU 光栅化（重启生效），规避部分 Linux/X11 驱动在文本密集页面的黑屏闪烁
- [x] 窗口：无边框自绘标题栏，记忆位置和大小；首页显示最近打开的仓库
- [x] 快捷键：统一动作表，设置窗口可录制、恢复默认，同作用域冲突自动解绑
- [x] 发布：`make release` 交叉编译 6 个平台，生成 `tar.gz` / `zip` 与 `SHA256SUMS`

## 默认快捷键

| 操作 | 键位 |
|---|---|
| 暂存选中 | `Ctrl+S` |
| 全部暂存 | `Ctrl+Shift+S` |
| 取消暂存 | `Ctrl+U` |
| 刷新状态 / 提交图 | `Ctrl+R` / `F5` |
| 提交 | `Ctrl+Enter` |
| 修正上次提交 | `Ctrl+M` |
| 推送 / 拉取 | `Ctrl+P` / `Ctrl+L` |
| 工作区 / 分支页 | `Ctrl+1` / `Ctrl+2` |
| 下一个 / 上一个标签 | `Ctrl+Tab` / `Ctrl+Shift+Tab` |
| 关闭标签 | `Ctrl+W` |
| 最小化窗口 | `Ctrl+Q` |

所有动作都可以在「设置 → 快捷键」里改绑，包括 `Ctrl+Q`。

## 构建与测试

需要 Go 1.27.3 和本机的 `git`；Go 依赖已固定 mygo 版本。

```sh
make run      # 开发运行（CGO_ENABLED=0）
make test     # CGO_ENABLED=0 go test ./...
make vet      # CGO_ENABLED=0 go vet ./...
make build    # 静态二进制 ./git-k
make release  # dist/ 下 6 个平台的发布包
make fmt      # go fmt ./...
make icon     # 由 assets/icon.svg 重新生成 PNG / ICO / Windows .syso
```

测试隔离用户配置和网络：`GIT_CONFIG_GLOBAL=/dev/null`、`GIT_CONFIG_SYSTEM=/dev/null`，在 `t.TempDir()` 里执行真实 git 命令。UI 测试使用 mygo 的 `ui.NewTester` 无窗口运行完整视图。

## 已知限制

当前专注「看状态、看差异、暂存、提交、看提交图」这条闭环，尚未实现：

- 部分暂存（hunk / line）
- 分支切换、Stash 创建/apply、merge / rebase / cherry-pick 等写操作
- 冲突 combined diff 的专用渲染、图片 diff、子模块操作
- 提交模板、签名、spellcheck、多窗口

这些能力不会假装支持；状态栏和界面只暴露已经跑通的路径。

## 致谢

- [git-cola](https://github.com/git-cola/git-cola)：交互与布局的灵感来源
- [mygo](https://github.com/egoist/mygo)：原生窗口、控件与编辑器
- [Lobe UI](https://github.com/lobehub/lobe-ui)：主题设计 token

## License

[Apache-2.0](LICENSE)
