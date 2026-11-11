package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

func (a *root) commitPane(c *ui.Context) {
	t := c.Theme()
	staged := a.status.Count(gitx.Staged)
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		ui.Column(c).Fill().MinHeight(0).Padding(12).Gap(4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.summary).Fill().Height(34).Placeholder("提交摘要").
					Label("提交摘要")
				label := "提交"
				if a.amendMode {
					label = "修正提交"
				}
				if a.commitBusy {
					label = "提交中…"
				}
				can := !a.commitBusy && strings.TrimSpace(a.summary) != "" &&
					(staged > 0 || a.amendMode)
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
				if a.amendMode {
					ui.Text(c, "正在修正上次提交").FontSize(11).TextColor(t.Warning)
					if ui.Button(c, "取消修正").Height(22).PaddingX(8).Clicked() {
						a.amendMode = false
						a.setNote("")
						c.Invalidate()
					}
				} else {
					ui.Text(c, "Ctrl+M 修正上次").FontSize(11).TextColor(t.TextMuted)
				}
				ui.Text(c, "Ctrl+↵ 提交").FontSize(11).TextColor(t.TextMuted).Grow(1)
			})
		})
	})
}
