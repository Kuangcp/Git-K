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
