package ui

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
)

const recentLimit = 20

// loadPopover lists historical repositories not open in a tab.
func (a *App) loadPopover(c *ui.Context, anchor ui.Element) {
	t := c.Theme()
	ui.Popover(c, anchor, &a.loadOpen, func() {
		ui.Column(c).Width(300).Gap(4).Padding(8).Children(func() {
			closed := a.closedRecent()
			if len(closed) == 0 {
				ui.Text(c, "没有其他历史仓库").FontSize(12).TextColor(t.TextMuted).Padding(4, 6)
			}
			for _, path := range closed {
				if ui.Button(c, filepath.Base(path)).FillWidth().Height(30).
					Justify(ui.Start).TextAlign(ui.Start).SingleLine().
					Tooltip(path).Clicked() {
					a.loadOpen = false
					a.OpenAndFocus(path)
				}
			}
		})
	})
}

// closedRecent lists history repositories that are not open.
func (a *App) closedRecent() []string {
	open := make(map[string]bool, len(a.tabs))
	for _, tab := range a.tabs {
		open[tab.key] = true
	}
	var out []string
	for _, path := range a.recent() {
		if !open[path] {
			out = append(out, path)
		}
	}
	return out
}

// removeRecent forgets a repository in the home page's history.
func (a *App) removeRecent(path string) {
	if a.settings == nil {
		return
	}
	a.settings.Update(func(c *settings.Config) {
		out := make([]string, 0, len(c.Recent))
		for _, old := range c.Recent {
			if old != path {
				out = append(out, old)
			}
		}
		c.Recent = out
	})
	a.dirty = true
}

func (a *App) empty(c *ui.Context) {
	t := c.Theme()
	recent := a.recent()
	ui.Column(c).Fill().AlignItems(ui.Center).Padding(56, 24, 24, 24).Gap(18).Children(func() {
		if a.icon != nil {
			ui.Image(c, a.icon).Width(88).Height(88)
		} else {
			ui.Text(c, "Git-K").FontSize(32).Bold()
		}
		ui.Text(c, "Git-K").FontSize(20).Bold()
		ui.Column(c).Width(520).Gap(2).Children(func() {
			ui.Row(c).Width(520).Height(30).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "最近打开").FontSize(12).Bold().TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, "打开其他仓库…").Height(28).PaddingX(10).Clicked() {
					a.chooseRepo()
				}
			})
			if len(recent) == 0 {
				ui.Text(c, "还没有最近打开的仓库").FontSize(13).TextColor(t.TextMuted).Padding(8, 0)
			}
			for _, path := range recent {
				item := ui.ButtonBase(c).FillWidth().Height(48).Padding(4, 12).
					Justify(ui.Start).AlignItems(ui.Center).
					BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Radius(0).
					Tooltip(path)
				item.ContextMenu(func(m *ui.Menu) {
					if m.Item("删除记录").Chosen() {
						a.removeRecent(path)
						c.Invalidate()
					}
				})
				item.Children(func() {
					ui.Column(c).Grow(1).Gap(0).Children(func() {
						ui.Text(c, filepath.Base(path)).FontSize(16).Bold().SingleLine()
						ui.Text(c, path).FontSize(11).TextColor(t.TextMuted).
							PaddingStart(22).SingleLine()
					})
				})
				if item.Hovered() {
					item = item.Background(t.SurfaceHover)
				}
				if item.Clicked() {
					a.OpenAndFocus(path)
				}
			}
		})
	})
}

func (a *App) chooseRepo() {
	if a.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       "选择 Git 仓库",
			Directory:   true,
			ButtonLabel: "打开",
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { a.OpenAndFocus(paths[0]) })
	}()
}

// touchRecent puts path first among the recent repositories.
func (a *App) touchRecent(path string) {
	if a.settings == nil || path == "" {
		return
	}
	a.settings.Update(func(c *settings.Config) {
		recent := []string{path}
		for _, old := range c.Recent {
			if old != path && len(recent) < recentLimit {
				recent = append(recent, old)
			}
		}
		c.Recent = recent
	})
	a.dirty = true
}

// recent lists repositories that still exist, newest first. The list is
// cached: stat'ing every history entry on every home frame is slow on
// Windows, and history only changes in this app.
func (a *App) recent() []string {
	if !a.dirty {
		return a.recentList
	}
	a.dirty = false
	a.recentList = a.recentList[:0]
	if a.settings == nil {
		return a.recentList
	}
	for _, path := range a.settings.Config().Recent {
		if _, err := os.Stat(path); err == nil {
			a.recentList = append(a.recentList, path)
		}
	}
	return a.recentList
}
