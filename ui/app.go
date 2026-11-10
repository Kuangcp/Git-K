package ui

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"git-k/app/settings"
)

// Tab is one open repository.
type Tab struct {
	// key is the repository's canonical root, so a repository opens once.
	key  string
	root *root
}

// App is the window and its tabs. Each tab owns one repository; the
// window shows the active tab.
type App struct {
	win      *mygo.Window
	tabs     []*Tab
	active   int
	settings *settings.Store
	openErr  string
	toast    string
	icon     ui.ImageSource
	loadOpen bool
	// pending holds launch directories that arrived before the window.
	pending []pendingOpen
}

type pendingOpen struct {
	dir   string
	quiet bool
}

// NewApp creates an app with no tabs.
func NewApp(st *settings.Store) *App {
	return &App{settings: st, active: -1}
}

// SetIconPNG gives the home page the app's icon.
func (a *App) SetIconPNG(data []byte) {
	if src, err := ui.DecodeImage(data); err == nil {
		a.icon = src
	}
}

// OpenInitial opens the first launch's directory, leaving the home page
// without an error when it is not a repository.
func (a *App) OpenInitial(dir string) {
	if a.win == nil {
		a.pending = append(a.pending, pendingOpen{dir: dir, quiet: true})
		return
	}
	a.OpenFocus(dir)
}

// Attach gives the app its window and opens any launches that arrived
// before it was ready.
func (a *App) Attach(win *mygo.Window) {
	a.win = win
	win.OnResize(a.saveBounds)
	win.OnMove(a.saveBounds)
	a.saveBounds()
	for _, t := range a.tabs {
		if t.root.win == nil {
			t.root.Attach(win)
		}
	}
	if a.active >= 0 {
		a.syncTitle()
	}
	pending := a.pending
	a.pending = nil
	for _, p := range pending {
		if p.quiet {
			a.OpenFocus(p.dir)
		} else {
			a.OpenAndFocus(p.dir)
		}
	}
}

// saveBounds remembers where the window is and how big it is.
func (a *App) saveBounds() {
	if a.win == nil || a.settings == nil {
		return
	}
	b := a.win.Bounds()
	a.settings.Update(func(c *settings.Config) {
		c.Window = settings.Window{
			X: b.X, Y: b.Y, Width: b.Width, Height: b.Height, Positioned: true,
		}
	})
}

// View builds the tab bar and the active repository.
func (a *App) View(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		if len(a.tabs) > 1 {
			a.tabBar(c)
		}
		if a.active >= 0 && a.active < len(a.tabs) {
			a.tabs[a.active].root.View(c)
		} else {
			a.empty(c)
		}
		if a.toast != "" {
			c.Toast(a.toast)
			a.toast = ""
		}
	})
	a.shortcuts(c)
}

// OpenAndFocus opens dir in a tab, focusing the window. A repository
// already open selects its existing tab.
func (a *App) OpenAndFocus(dir string) {
	if a.win == nil {
		a.pending = append(a.pending, pendingOpen{dir: dir})
		return
	}
	if err := a.open(dir); err != nil {
		a.openErr = err.Error()
		a.toast = "无法打开仓库：" + err.Error()
	} else {
		a.openErr = ""
		a.toast = ""
	}
	a.win.Show()
	a.win.Focus()
}

// OpenFocus opens dir when it is a repository, and otherwise just shows
// the window: a desktop launch from a home directory opens the home page
// without an error.
func (a *App) OpenFocus(dir string) {
	if a.win == nil {
		a.pending = append(a.pending, pendingOpen{dir: dir, quiet: true})
		return
	}
	if err := a.open(dir); err != nil {
		a.openErr, a.toast = "", ""
	}
	a.win.Show()
	a.win.Focus()
}

func (a *App) open(dir string) error {
	r := NewRootLazy(dir, a.settings)
	if r.repo == nil {
		err := r.openErr
		if err == "" {
			err = "打开仓库失败"
		}
		r.close()
		a.openErr = err
		return errors.New(err)
	}
	key := canonicalKey(r.repo.Root)
	if i := a.find(key); i >= 0 {
		r.close()
		a.activate(i)
		a.touchRecent(key)
		return nil
	}
	r.onTitle = a.syncTitle
	r.openTab = a.OpenAndFocus
	r.pickRepo = a.chooseRepo
	r.loadOpen = &a.loadOpen
	r.loadMenu = a.loadPopover
	r.quit = mygo.App.Quit
	a.tabs = append(a.tabs, &Tab{key: key, root: r})
	a.openErr = ""
	a.touchRecent(key)
	a.activate(len(a.tabs) - 1)
	return nil
}

const recentLimit = 20

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
}

// recent lists repositories that still exist, newest first.
func (a *App) recent() []string {
	if a.settings == nil {
		return nil
	}
	var out []string
	for _, path := range a.settings.Config().Recent {
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}

func canonicalKey(path string) string {
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return path
}

func (a *App) find(key string) int {
	for i, t := range a.tabs {
		if t.key == key {
			return i
		}
	}
	return -1
}

// activate shows tab i, taking the layout ratios shared by all tabs.
func (a *App) activate(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	a.loadOpen = false
	a.active = i
	r := a.tabs[i].root
	r.loadLayout()
	if a.win != nil && r.win == nil {
		r.Attach(a.win)
	}
	a.syncTitle()
}

func (a *App) closeAt(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	wasActive := a.active == i
	a.tabs[i].root.close()
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	switch {
	case len(a.tabs) == 0:
		a.active = -1
		a.syncTitle()
	case wasActive:
		a.active = min(i, len(a.tabs)-1)
		a.activate(a.active)
	case a.active > i:
		a.active--
		a.syncTitle()
	default:
		a.syncTitle()
	}
}

func (a *App) syncTitle() {
	if a.win == nil {
		return
	}
	if a.active < 0 || a.active >= len(a.tabs) {
		a.win.SetTitle("Git-K")
		return
	}
	a.win.SetTitle(repoTitle(a.tabs[a.active].root))
}

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
		if !open[canonicalKey(path)] {
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

// shortcuts handles window-level tab keys; the active tab handles its own.
func (a *App) shortcuts(c *ui.Context) {
	if c.Shortcut(ui.Ctrl, ui.KeyQ) {
		mygo.App.Quit()
		return
	}
	if c.Shortcut(ui.Ctrl, ui.KeyW) {
		a.closeAt(a.active)
		c.Invalidate()
		return
	}
	if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab) {
		a.cycle(-1)
		c.Invalidate()
		return
	}
	if c.Shortcut(ui.Ctrl, ui.KeyTab) {
		a.cycle(1)
		c.Invalidate()
	}
}

func (a *App) cycle(d int) {
	if len(a.tabs) < 2 {
		return
	}
	i := (a.active + d + len(a.tabs)) % len(a.tabs)
	a.activate(i)
}
