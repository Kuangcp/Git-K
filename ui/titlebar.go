package ui

import (
	"github.com/egoist/mygo/ui"
)

var (
	settingsIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915"/><circle cx="12" cy="12" r="3"/></svg>`))
	minimizeIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 12h12"/></svg>`))
	maximizeIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>`))
	restoreIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="8" width="11" height="11" rx="1"/><path d="M9 8V5h10v10h-3"/></svg>`))
	closeIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>`))
)

// titleBar is the frameless window's drag region and window controls.
func (a *App) titleBar(c *ui.Context) {
	t := c.Theme()
	const side = 120
	bar := ui.Row(c).Height(32).FillWidth().Padding(0, 8).Gap(2).
		AlignItems(ui.Center).DragWindow().Background(t.Surface)
	bar.Children(func() {
		left := ui.Row(c).Width(side).AlignItems(ui.Center)
		left.Children(func() {
			if a.icon != nil {
				ui.Image(c, a.icon).Width(16).Height(16)
			}
		})
		ui.Text(c, a.activeName()).FontSize(24).TextAlign(ui.Center).
			SingleLine().Grow(1)
		right := ui.Row(c).Width(side).Gap(2).AlignItems(ui.Center).Justify(ui.End)
		right.Children(func() {
			iconButton(c, settingsIcon, "设置", "设置", func() { a.settingsOpen = true })
			if a.win != nil {
				iconButton(c, minimizeIcon, "最小化", "最小化", a.win.Minimize)
				if a.win.IsMaximized() {
					iconButton(c, restoreIcon, "还原", "还原", a.win.ToggleMaximize)
				} else {
					iconButton(c, maximizeIcon, "最大化", "最大化", a.win.ToggleMaximize)
				}
				iconButton(c, closeIcon, "关闭", "关闭窗口", a.win.Close)
			}
		})
	})
}

func (a *App) activeName() string {
	if a.active >= 0 && a.active < len(a.tabs) {
		return a.tabs[a.active].root.repo.Name
	}
	return "Git-K"
}
