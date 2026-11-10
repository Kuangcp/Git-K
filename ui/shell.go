package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

func (a *root) topBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(46).PaddingX(14).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, a.repo.Name).FontSize(15).Bold().TextColor(t.Accent).SingleLine()
		ui.Text(c, a.repo.Root).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1)

		branch := a.status.Branch
		if branch == "" {
			branch = "(无分支)"
		}
		if a.status.Detached {
			branch = "detached"
		}
		ui.Row(c).Height(24).PaddingX(8).Radius(6).BorderWidth(1).BorderColor(t.Border).
			AlignItems(ui.Center).Children(func() {
			ui.Text(c, branch).FontSize(12).SingleLine()
		})
		if a.status.Ahead > 0 || a.status.Behind > 0 {
			ui.Text(c, fmt.Sprintf("↑%d ↓%d", a.status.Ahead, a.status.Behind)).
				FontSize(11).TextColor(t.TextMuted)
		}

		if ui.Button(c, "全部暂存").Disabled(a.busy || a.status.Total() == 0).Clicked() {
			a.stageAll()
		}
		if ui.Button(c, "全部取消").Disabled(a.busy || !a.status.HasStaged()).Clicked() {
			a.unstageAll()
		}
		if ui.Button(c, "打开仓库…").Clicked() {
			if a.pickRepo != nil {
				a.pickRepo()
			} else {
				a.chooseRepo()
			}
		}
		if ui.Button(c, "刷新").Clicked() {
			a.refreshStatus()
		}
	})
}

func (a *root) statusPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		a.paneTitle(c, "状态", fmt.Sprintf("%d 个变更", a.status.Total()))
		if a.statusErr != "" {
			ui.Text(c, a.statusErr).FontSize(12).TextColor(t.Danger).Padding(12)
			return
		}
		if len(a.entries) == 0 {
			if !a.loaded {
				ui.Text(c, "正在读取状态…").FontSize(12).TextColor(t.TextMuted).Padding(12)
			} else {
				ui.Text(c, "工作区干净").FontSize(12).TextColor(t.TextMuted).Padding(12)
			}
			return
		}
		a.list.Key = func(i int) any { return a.entries[i].key }
		a.list.Label = func(i int) string {
			e := a.entries[i]
			if e.header {
				return e.title
			}
			return gitx.DisplayPath(e.row.Path)
		}
		a.list.Header = func(i int) bool { return a.entries[i].header }
		var toggle listEntry
		hasToggle := false
		list := ui.List(c, &a.list, len(a.entries), func(i int) {
			if a.statusRow(c, i) {
				toggle, hasToggle = a.entries[i], true
			}
		}).Grow(1).Padding(0, 4)
		if list.Changed() {
			a.clearGroup()
		}
		// 双击在列表构建完后处理：暂存会同步刷新 entries，构建中改会越界。
		if hasToggle && !a.busy {
			a.toggleRow(toggle)
		}
	})
}

// statusRow builds one status row and reports a double click on a file.
func (a *root) statusRow(c *ui.Context, i int) bool {
	t := c.Theme()
	e := a.entries[i]
	if e.header {
		bg, fg := t.Surface, t.TextMuted
		if g, ok := a.groupActive(); ok && g == e.group {
			bg, fg = t.SurfaceHover, t.Accent
		}
		header := ui.Button(c, fmt.Sprintf("%s %d", e.title, e.count)).
			Fill().Height(28).PaddingX(12).PaddingY(0).Justify(ui.Start).TextAlign(ui.Start).
			FontSize(11).Bold().Background(bg).BorderWidth(0).Radius(0).TextColor(fg)
		if header.Clicked() {
			a.selectGroup(e.group)
			c.Invalidate()
		}
		return false
	}
	row := ui.Row(c).Height(28).PaddingX(12).Gap(8).AlignItems(ui.Center)
	row.ContextMenu(func(m *ui.Menu) {
		if e.row.Group == gitx.Staged {
			if m.Item("取消暂存").Chosen() {
				a.changeIndex(a.actionRows(e.row), true)
			}
		} else {
			if m.Item("暂存").Chosen() {
				a.changeIndex(a.actionRows(e.row), false)
			}
		}
		m.Separator()
		if m.Item("复制路径").Chosen() {
			mygo.Clipboard.WriteText(e.row.Path)
		}
		paths := untrackedPaths(a.actionRows(e.row))
		if len(paths) > 0 {
			m.Separator()
			label := "删除文件…"
			if len(paths) > 1 {
				label = fmt.Sprintf("删除 %d 个文件…", len(paths))
			}
			if m.Item(label).Danger().Chosen() {
				a.deletePaths = paths
				a.deleteAsk = true
				c.Invalidate()
			}
		}
	})
	row.Children(func() {
		ui.Text(c, string(e.row.Code)).Width(16).FontSize(12).Bold().
			TextColor(codeColor(t, e.row.Group, e.row.Code))
		ui.Text(c, gitx.DisplayPath(e.row.Path)).FontSize(13).SingleLine().Grow(1)
		if e.row.OldPath != "" {
			ui.Text(c, "← "+gitx.DisplayPath(e.row.OldPath)).FontSize(11).
				TextColor(t.TextMuted).SingleLine()
		}
	})
	return row.RowDoubleClicked()
}

// toggleRow stages an unstaged row, or unstages a staged one. A file with
// conflicts is left alone: staging it would mark the conflict resolved.
func (a *root) toggleRow(e listEntry) {
	if e.header {
		return
	}
	switch e.row.Group {
	case gitx.Modified, gitx.Untracked:
		a.changeIndex([]gitx.Row{e.row}, false)
	case gitx.Staged:
		a.changeIndex([]gitx.Row{e.row}, true)
	}
}

// deleteDialog asks before untracked files are deleted: they are gone
// from disk, not restored from git.
func (a *root) deleteDialog(c *ui.Context) {
	if !a.deleteAsk {
		a.deletePaths = nil
		return
	}
	switch ui.AlertDialog(c, &a.deleteAsk, "删除文件？", deleteMessage(a.deletePaths), "取消", "删除") {
	case 0, ui.AlertDismissed:
		a.deletePaths = nil
	case 1:
		paths := a.deletePaths
		a.deletePaths = nil
		a.deleteFiles(paths)
	}
}

// deleteMessage names the files to delete, capping a long list so the
// alert stays readable.
func deleteMessage(paths []string) string {
	const shown = 10
	var b strings.Builder
	if len(paths) == 1 {
		b.WriteString("将从磁盘删除未跟踪的文件：\n")
	} else {
		fmt.Fprintf(&b, "将从磁盘删除 %d 个未跟踪的文件：\n", len(paths))
	}
	for i, path := range paths {
		if i == shown {
			fmt.Fprintf(&b, "…等 %d 个文件\n", len(paths))
			break
		}
		b.WriteString(gitx.DisplayPath(path))
		b.WriteByte('\n')
	}
	b.WriteString("\n此操作无法撤销。")
	return b.String()
}

func (a *root) deleteFiles(paths []string) {
	if a.busy || a.repo == nil || len(paths) == 0 {
		return
	}
	a.setBusy(true)
	repo := a.repo
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := repo.RemoveFiles(ctx, paths)
		cancel()
		a.update(func() {
			a.setBusy(false)
			if err != nil {
				a.note = "删除失败：" + err.Error()
				return
			}
			a.note = ""
			a.diffKey = ""
			a.refreshStatus()
		})
	})
}

func (a *root) commitPane(c *ui.Context) {
	t := c.Theme()
	staged := a.status.Count(gitx.Staged)
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		ui.Column(c).Fill().MinHeight(0).Padding(12).Gap(4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.summary).Fill().Height(30).Placeholder("提交摘要").
					Label("提交摘要")
				label := "提交"
				if a.commitBusy {
					label = "提交中…"
				}
				can := !a.commitBusy && strings.TrimSpace(a.summary) != "" &&
					(staged > 0 || a.amend)
				if ui.PrimaryButton(c, label).Disabled(!can).Clicked() {
					a.doCommit()
				}
			})
			ui.Column(c).Fill().MinHeight(0).BorderWidth(1).BorderColor(t.Border).Radius(8).Clip().Children(func() {
				editor.View(c, &a.bodyView).Fill().MinHeight(0).Padding(8)
			})
			if a.commitErr != "" {
				ui.Text(c, a.commitErr).FontSize(12).TextColor(t.Danger).SingleLine().
					Tooltip(a.commitErr)
			} else if a.note != "" {
				ui.Text(c, a.note).FontSize(12).TextColor(t.Success).SingleLine().Tooltip(a.note)
			}
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c, &a.amend, "修正上次")
				ui.Text(c, "Ctrl+↵ 提交").FontSize(11).TextColor(t.TextMuted).Grow(1)
			})
		})
	})
}

func (a *root) diffPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		ui.Row(c).Height(34).PaddingX(12).Gap(10).AlignItems(ui.Center).
			BorderWidth(1, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "差异").FontSize(12).Bold().TextColor(t.TextMuted)
			g, groupMode := a.groupActive()
			switch {
			case groupMode:
				ui.Text(c, fmt.Sprintf("%d 个文件", len(a.groupDiffs))).FontSize(12).Bold().SingleLine()
				ui.Text(c, g.String()).FontSize(11).TextColor(t.TextMuted)
			case a.diffPath != "":
				ui.Text(c, gitx.DisplayPath(a.diffPath)).FontSize(12).Bold().SingleLine()
				ui.Text(c, a.diffGroup.String()).FontSize(11).TextColor(t.TextMuted)
			}
			if groupMode || a.diffPath != "" {
				if a.diffAdded > 0 {
					ui.Text(c, fmt.Sprintf("+%d", a.diffAdded)).FontSize(11).TextColor(t.Success)
				}
				if a.diffDeleted > 0 {
					ui.Text(c, fmt.Sprintf("−%d", a.diffDeleted)).FontSize(11).TextColor(t.Danger)
				}
			}
			ui.Text(c, "").Grow(1)
			if a.diffLoading {
				ui.Text(c, "读取中…").FontSize(11).TextColor(t.TextMuted)
			}
		})
		if _, ok := a.groupActive(); ok {
			if a.diffLoading {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "正在读取差异…").FontSize(12).TextColor(t.TextMuted)
				})
			} else {
				a.groupDiffPane(c)
			}
		} else {
			switch a.diffMode {
			case diffError:
				ui.Column(c).Fill().Center().Padding(16).Children(func() {
					ui.Text(c, "无法显示差异").FontSize(12).Bold()
					ui.Text(c, a.diffErr).FontSize(11).TextColor(t.Danger).SingleLine().Tooltip(a.diffErr)
				})
			case diffBinary:
				ui.Column(c).Fill().Center().Gap(6).Children(func() {
					ui.Text(c, "二进制文件").FontSize(13).Bold()
					ui.Text(c, fmt.Sprintf("+%d −%d（按行统计）", a.diffAdded, a.diffDeleted)).
						FontSize(11).TextColor(t.TextMuted)
				})
			case diffEmpty:
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, "没有可显示的差异").FontSize(12).TextColor(t.TextMuted)
				})
			case diffRaw:
				editor.View(c, &a.rawView).Fill()
			case diffPatch:
				editor.DiffView(c, &a.diff).Fill()
			default:
				ui.Column(c).Fill().Center().Children(func() {
					switch {
					case a.diffLoading:
						ui.Text(c, "正在读取差异…").FontSize(12).TextColor(t.TextMuted)
					case a.loaded && a.status.Total() == 0:
						ui.Text(c, "工作区干净，没有差异").FontSize(12).TextColor(t.TextMuted)
					default:
						ui.Text(c, "选择一个文件查看差异").FontSize(12).TextColor(t.TextMuted)
					}
				})
			}
		}
	})
}

// groupDiffPane stacks the group's diffstat and each file's diff, which
// keep the single-file view's colors.
func (a *root) groupDiffPane(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(12).Gap(10).Children(func() {
			if a.groupStat.LineCount() > 0 {
				editor.View(c, &a.groupStatView).FillWidth()
			}
			for i := range a.groupFiles {
				f := &a.groupFiles[i]
				ui.Text(c, gitx.DisplayPath(f.path)).FontSize(12).Bold().SingleLine()
				switch f.mode {
				case diffPatch:
					editor.DiffView(c, &f.diff).FillWidth()
				case diffError:
					ui.Text(c, f.raw.String()).FontSize(11).TextColor(t.Danger).SingleLine()
				default:
					editor.View(c, &f.view).FillWidth()
				}
			}
		})
	})
}

func (a *root) footer(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(26).PaddingX(12).Gap(16).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, "Ctrl+S 暂存 · Ctrl+U 取消暂存 · Ctrl+Shift+S 全部暂存 · 双击行 切换暂存 · F5 刷新 · Ctrl+Q 退出").
			FontSize(11).TextColor(t.TextMuted).Grow(1)
		if a.status.Upstream != "" {
			ui.Text(c, a.status.Upstream).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
	})
}

func (a *root) noRepo(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Padding(32).Children(func() {
		ui.Text(c, "Git-K").FontSize(28).Bold()
		ui.Text(c, "原生 Git 客户端").FontSize(13).TextColor(t.TextMuted)
		if a.openErr != "" {
			ui.Text(c, a.openErr).FontSize(12).TextColor(t.Danger).SingleLine().Tooltip(a.openErr)
		}
		ui.Row(c).Gap(10).Children(func() {
			if ui.PrimaryButton(c, "选择仓库…").Clicked() {
				a.chooseRepo()
			}
			if ui.Button(c, "使用当前目录").Clicked() {
				a.openRepo("")
			}
		})
		ui.Text(c, "也可以这样启动：git-k /path/to/repo").FontSize(11).TextColor(t.TextMuted)
	})
}

func (a *root) paneTitle(c *ui.Context, title, meta string) {
	t := c.Theme()
	ui.Row(c).Height(38).PaddingX(12).AlignItems(ui.Center).Gap(8).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, title).FontSize(13).Bold()
		ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted).Grow(1)
	})
}

func codeColor(t *ui.Theme, group gitx.Group, code byte) ui.Color {
	// A staged modification or rename is a change entering the index, so
	// it reads green like a staged addition rather than the warning or
	// accent of a working tree change.
	if group == gitx.Staged && (code == 'M' || code == 'R') {
		return t.Success
	}
	switch code {
	case 'A':
		return t.Success
	case 'D':
		return t.Danger
	case 'U':
		return t.Danger
	case 'M', 'T':
		return t.Warning
	case 'R', 'C':
		return t.Accent
	case '?':
		return t.TextMuted
	}
	return t.Text
}
