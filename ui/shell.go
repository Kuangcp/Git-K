package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"git-k/app/gitx"
)

var (
	refreshIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 12a8 8 0 1 1-2.3-5.7"/><path d="M20 4v4h-4"/></svg>`))
	pullIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M4 20h16"/></svg>`))
	pushIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 21V9"/><path d="m7 14 5-5 5 5"/><path d="M4 4h16"/></svg>`))
	stageAllIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m3 17 2 2 4-4"/><path d="m3 7 2 2 4-4"/><path d="M13 6h8"/><path d="M13 12h8"/><path d="M13 18h8"/></svg>`))
	unstageAllIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 14 4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/></svg>`))
	recentIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 2.6-6.4L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l4 2"/></svg>`))
	openRepoIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 14 1.5-2.9A2 2 0 0 1 9.4 10H20a2 2 0 0 1 1.9 2.5l-1.6 6A2 2 0 0 1 18.4 20H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.7.9l.8 1.2a2 2 0 0 0 1.7.9H18a2 2 0 0 1 2 2v2"/></svg>`))
)

// toolbarDivider separates groups of related toolbar buttons.
func toolbarDivider(c *ui.Context) {
	ui.Box(c).Width(1).Height(18).Margin(0, 6, 0, 6).Background(c.Theme().Border)
}

// iconButton builds a small toolbar button showing an icon.
func iconButton(c *ui.Context, icon *ui.SVG, label, tip string, onClick func()) ui.Element {
	return iconButtonDisabled(c, icon, label, tip, false, onClick)
}

// iconButtonDisabled is iconButton with a disabled state, for actions
// that have nothing to do.
func iconButtonDisabled(c *ui.Context, icon *ui.SVG, label, tip string, disabled bool, onClick func()) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Width(28).Height(28).Label(label).Tooltip(tip).Disabled(disabled)
	if disabled {
		b = b.TextColor(t.TextMuted)
	}
	b.Children(func() {
		ui.Icon(c, icon).Width(16).Height(16)
	})
	if b.Hovered() && !disabled {
		b = b.Background(t.SurfaceHover).Radius(t.Radius)
	}
	if b.Clicked() {
		onClick()
	}
	return b
}

func (a *root) topBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(46).PaddingX(14).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, "").Grow(1)
		iconButtonDisabled(c, stageAllIcon, "全部暂存", a.keyTip("stageAll", "全部暂存"),
			a.busy || a.status.Total() == 0, a.stageAll)
		iconButtonDisabled(c, unstageAllIcon, "全部取消", "全部取消暂存",
			a.busy || !a.status.HasStaged(), a.unstageAll)
		toolbarDivider(c)
		load := iconButton(c, recentIcon, "加载仓库", "加载最近仓库", func() {
			if a.loadOpen != nil {
				*a.loadOpen = !*a.loadOpen
				c.Invalidate()
			}
		})
		if a.loadMenu != nil {
			a.loadMenu(c, load)
		}
		iconButton(c, openRepoIcon, "打开仓库", "打开仓库…", func() {
			if a.pickRepo != nil {
				a.pickRepo()
			} else {
				a.chooseRepo()
			}
		})
		toolbarDivider(c)
		iconButton(c, pullIcon, "拉取", a.keyTip("pull", "拉取"), a.pull)
		iconButton(c, pushIcon, "推送", a.keyTip("push", "推送"), a.push)
		iconButton(c, refreshIcon, "刷新", a.keyTip("refresh", "刷新"), a.refreshStatus)
	})
}

// keyTip is a toolbar tooltip with the action's current chord.
func (a *root) keyTip(id, label string) string {
	if s := a.keys.chordText(id); s != "" {
		return label + " (" + s + ")"
	}
	return label
}

// keyHint is "Ctrl+S 暂存" for the footer, or "" when unbound.
func (a *root) keyHint(id, label string) string {
	return a.keys.hintFor(id, label)
}

func (a *root) footer(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(26).PaddingX(12).Gap(16).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		var hints []string
		for _, h := range [][2]string{
			{"stage", "暂存"}, {"unstage", "取消暂存"}, {"stageAll", "全部暂存"},
			{"refresh", "刷新"}, {"pull", "拉取"}, {"push", "推送"},
			{"minimize", "最小化"},
		} {
			if s := a.keyHint(h[0], h[1]); s != "" {
				hints = append(hints, s)
			}
		}
		hints = append(hints, "双击行 切换暂存")
		ui.Text(c, strings.Join(hints, " · ")).
			FontSize(11).TextColor(t.TextMuted).Grow(1)
		branch := a.status.Branch
		if branch == "" {
			branch = "(无分支)"
		}
		if a.status.Detached {
			branch = "detached"
		}
		ui.Text(c, branch).FontSize(11).TextColor(t.TextMuted).SingleLine()
		if a.status.Ahead > 0 || a.status.Behind > 0 {
			ui.Text(c, fmt.Sprintf("↑%d ↓%d", a.status.Ahead, a.status.Behind)).
				FontSize(11).TextColor(t.TextMuted)
		}
		ui.Text(c, a.repo.Root).FontSize(11).TextColor(t.TextMuted).SingleLine().
			MaxWidth(440).Tooltip(a.repo.Root)
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
