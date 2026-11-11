package ui

import (
	"errors"
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
	win        *mygo.Window
	tabs       []*Tab
	active     int
	settings   *settings.Store
	openErr    string
	toast      string
	icon       ui.ImageSource
	loadOpen   bool
	recentList []string
	dirty      bool
	// pending holds launch directories that arrived before the window.
	pending []pendingOpen
}

type pendingOpen struct {
	dir   string
	quiet bool
}

// NewApp creates an app with no tabs.
func NewApp(st *settings.Store) *App {
	return &App{settings: st, active: -1, dirty: true}
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
