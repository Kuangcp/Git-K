package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"git-k/app/gitx"
)

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
		modified := modifiedPaths(a.actionRows(e.row))
		if len(modified) > 0 {
			m.Separator()
			if m.Item("撤销修改…").Danger().Chosen() {
				a.revertPaths = modified
				a.revertAsk = true
				c.Invalidate()
			}
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

// revertDialog asks before working tree changes are discarded.
func (a *root) revertDialog(c *ui.Context) {
	if !a.revertAsk {
		a.revertPaths = nil
		return
	}
	switch ui.AlertDialog(c, &a.revertAsk, "撤销修改？", fileListMessage("将丢弃这些文件在工作区的修改：", a.revertPaths), "取消", "撤销") {
	case 0, ui.AlertDismissed:
		a.revertPaths = nil
	case 1:
		paths := a.revertPaths
		a.revertPaths = nil
		a.revertFiles(paths)
	}
}

func (a *root) revertFiles(paths []string) {
	if a.busy || a.repo == nil || len(paths) == 0 {
		return
	}
	a.setBusy(true)
	repo := a.repo
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := repo.RevertFiles(ctx, paths)
		cancel()
		a.update(func() {
			a.setBusy(false)
			a.diffKey = ""
			if err != nil {
				a.setNote("撤销失败：" + err.Error())
			} else {
				a.setNote("已撤销修改")
			}
			a.refreshStatus()
		})
	})
}

// fileListMessage names paths in a confirmation, capping a long list.
func fileListMessage(prefix string, paths []string) string {
	const shown = 10
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteByte('\n')
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
	if len(paths) == 1 {
		return fileListMessage("将从磁盘删除未跟踪的文件：", paths)
	}
	return fileListMessage(fmt.Sprintf("将从磁盘删除 %d 个未跟踪的文件：", len(paths)), paths)
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
				a.setNote("删除失败：" + err.Error())
				return
			}
			a.setNote("")
			a.diffKey = ""
			a.refreshStatus()
		})
	})
}
