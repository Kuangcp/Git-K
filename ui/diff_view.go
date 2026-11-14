package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

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
			idle := "选择一个文件查看差异"
			if a.loaded && a.status.Total() == 0 {
				idle = "工作区干净，没有差异"
			}
			showDiffBody(c, diffRender{
				state:    &a.diff,
				rawView:  &a.rawView,
				mode:     a.diffMode,
				err:      a.diffErr,
				added:    a.diffAdded,
				deleted:  a.diffDeleted,
				loading:  a.diffLoading,
				empty:    "没有可显示的差异",
				idle:     idle,
				selected: a.diffPath != "",
			})
		}
	})
}

// diffRender is what showDiffBody needs from a diff view, whether the
// workspace's or a commit's.
type diffRender struct {
	state    *editor.DiffState
	rawView  *editor.State
	mode     diffMode
	err      string
	added    int
	deleted  int
	loading  bool
	empty    string
	idle     string
	selected bool
}

// showDiffBody renders a patch, a raw fallback, or an empty/error state.
func showDiffBody(c *ui.Context, d diffRender) {
	t := c.Theme()
	switch d.mode {
	case diffError:
		ui.Column(c).Fill().Center().Padding(16).Children(func() {
			ui.Text(c, "无法显示差异").FontSize(12).Bold()
			ui.Text(c, d.err).FontSize(11).TextColor(t.Danger).SingleLine().Tooltip(d.err)
		})
	case diffBinary:
		ui.Column(c).Fill().Center().Gap(6).Children(func() {
			ui.Text(c, "二进制文件").FontSize(13).Bold()
			ui.Text(c, fmt.Sprintf("+%d −%d（按行统计）", d.added, d.deleted)).
				FontSize(11).TextColor(t.TextMuted)
		})
	case diffEmpty:
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, d.empty).FontSize(12).TextColor(t.TextMuted)
		})
	case diffRaw:
		editor.View(c, d.rawView).FillWidth().Grow(1).Shrink(1).MinHeight(0)
	case diffPatch:
		editor.DiffView(c, d.state).FillWidth().Grow(1).Shrink(1).MinHeight(0)
	default:
		ui.Column(c).Fill().Center().Children(func() {
			switch {
			case d.loading:
				ui.Text(c, "正在读取差异…").FontSize(12).TextColor(t.TextMuted)
			case !d.selected && d.idle != "":
				ui.Text(c, d.idle).FontSize(12).TextColor(t.TextMuted)
			default:
				ui.Text(c, "选择一个文件查看差异").FontSize(12).TextColor(t.TextMuted)
			}
		})
	}
}

// groupDiffPane stacks the group's diffstat and each file's diff, which
// keep the single-file view's colors.
func (a *root) groupDiffPane(c *ui.Context) {
	groupDiffPane(c, a.groupFiles, a.groupStat, &a.groupStatView)
}

// groupDiffPane stacks a group's diffstat and each file's diff.
func groupDiffPane(c *ui.Context, files []groupFileDiff, stat *editor.Buffer, statView *editor.State) {
	t := c.Theme()
	// Grow into the room left after the diff header rather than taking the
	// whole parent as Fill does: the scroll's own bottom would then sit
	// below the pane, and its last rows would stay clipped at the edge.
	ui.Scroll(c).FillWidth().Grow(1).Shrink(1).MinHeight(0).Children(func() {
		// Extra room below the last diff: FitContent measures the lines,
		// and the scroll's edge would otherwise clip the final row.
		ui.Column(c).FillWidth().Padding(12, 12, 28, 12).Gap(10).Children(func() {
			if stat.LineCount() > 0 {
				editor.View(c, statView).FillWidth()
			}
			for i := range files {
				f := &files[i]
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
