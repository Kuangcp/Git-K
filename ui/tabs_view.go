package ui

import "github.com/egoist/mygo/ui"

func (a *App) tabBar(c *ui.Context) {
	t := c.Theme()
	closeAt := -1
	ui.Row(c).Height(34).PaddingX(8).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		for i, tab := range a.tabs {
			name := tab.root.repo.Name
			bg, fg := t.Surface, t.Text
			if i == a.active {
				bg, fg = t.SurfaceHover, t.Accent
			}
			// Name and close are two buttons whose meeting borders are
			// dropped: together they read as one tab.
			ui.Row(c).Gap(0).Children(func() {
				label := ui.Button(c, name).Height(26).PaddingX(10).
					Background(bg).TextColor(fg).
					BorderWidth(1, 0, 1, 1).BorderColor(t.Border).
					Radius(t.Radius, 0, 0, t.Radius).
					Tooltip(tab.root.repo.Root)
				if label.Clicked() {
					a.activate(i)
					c.Invalidate()
				}
				close := ui.Button(c, "×").Height(26).PaddingX(6).
					Background(bg).TextColor(t.TextMuted).
					BorderWidth(1, 1, 1, 0).BorderColor(t.Border).
					Radius(0, t.Radius, t.Radius, 0).
					Tooltip("关闭标签")
				if close.Clicked() {
					closeAt = i
				}
			})
		}
		ui.Text(c, "").Grow(1)
	})
	if closeAt >= 0 {
		a.closeAt(closeAt)
		c.Invalidate()
	}
}
