package ui

import "github.com/egoist/mygo/ui"

func (a *App) tabBar(c *ui.Context) {
	t := c.Theme()
	// Tabs are the main way between repositories; keep them big enough to
	// read at a glance instead of relying on the base font size.
	size := t.FontSize + 2
	closeAt := -1
	ui.Row(c).Height(44).PaddingX(8).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		for i, tab := range a.tabs {
			name := tab.root.repo.Name
			// Only the active tab's label is colored: accent fills from the
			// Lobe dark theme are near-white, too loud for a tab bar.
			bg, fg := t.Surface, t.Text
			if i == a.active {
				bg, fg = t.SurfaceHover, activeTabColor(t)
			}
			// Name and close are two buttons whose meeting borders are
			// dropped: together they read as one tab.
			ui.Row(c).Gap(0).Children(func() {
				label := ui.Button(c, name).Height(32).PaddingX(14).
					FontSize(size).
					Background(bg).TextColor(fg).
					BorderWidth(1, 0, 1, 1).BorderColor(t.Border).
					Radius(t.Radius, 0, 0, t.Radius).
					Tooltip(tab.root.repo.Root)
				if label.Clicked() {
					a.activate(i)
					c.Invalidate()
				}
				close := ui.Button(c, "×").Height(32).PaddingX(9).
					FontSize(size).
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

// activeTabColor is the cyan that marks the active tab's name. It follows
// the window background so the name stays readable in both modes: Lobe's
// accent is near-white in dark themes, which made a filled tab too bright.
func activeTabColor(t *ui.Theme) ui.Color {
	bg := t.Background
	if 299*int(bg.R)+587*int(bg.G)+114*int(bg.B) < 128*1000 {
		return ui.Hex("#22d3ee")
	}
	return ui.Hex("#0e7490")
}
