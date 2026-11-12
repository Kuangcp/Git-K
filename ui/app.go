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
	win          *mygo.Window
	tabs         []*Tab
	active       int
	settings     *settings.Store
	openErr      string
	toast        string
	icon         ui.ImageSource
	loadOpen     bool
	settingsOpen bool
	settingsTab  int
	recentList   []string
	dirty        bool
	keys         *bindings
	// minimize lets tests observe the window action; nil minimizes the
	// real window.
	minimize func()
	// pending holds launch directories that arrived before the window.
	pending []pendingOpen
}

type pendingOpen struct {
	dir   string
	quiet bool
}

// NewApp creates an app with no tabs.
func NewApp(st *settings.Store) *App {
	a := &App{settings: st, active: -1, dirty: true, keys: newBindings()}
	if st != nil {
		a.keys.load(st.Config().Shortcuts)
	}
	return a
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
	a.applyTheme(c)
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c)
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
	a.settingsModal(c)
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
	// gitx.Open runs rev-parse --show-toplevel, so a launch from any depth
	// creates the tab of the repository root and dedupes against it.
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
	r.openTab = a.OpenAndFocus
	r.pickRepo = a.chooseRepo
	r.loadOpen = &a.loadOpen
	r.loadMenu = a.loadPopover
	r.keys = a.keys
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
	case wasActive:
		a.active = min(i, len(a.tabs)-1)
		a.activate(a.active)
	case a.active > i:
		a.active--
	}
}

// shortcuts handles window-level keys; the active tab handles its own.
// Recordings in the settings dialog see the keys first and suppress
// dispatch for the frame.
func (a *App) shortcuts(c *ui.Context) {
	if a.keys.capturing() {
		return
	}
	for _, act := range keyActions {
		if act.Scope != scopeWindow || !a.keys.hit(c, act.ID) {
			continue
		}
		switch act.ID {
		case "minimize":
			a.minimizeWindow()
		case "closeTab":
			a.closeAt(a.active)
			c.Invalidate()
		case "nextTab":
			a.cycle(1)
			c.Invalidate()
		case "prevTab":
			a.cycle(-1)
			c.Invalidate()
		}
		return
	}
}

// minimizeWindow minimizes the window; Ctrl+Q no longer quits the app.
func (a *App) minimizeWindow() {
	if a.minimize != nil {
		a.minimize()
		return
	}
	if a.win != nil {
		a.win.Minimize()
	}
}

func (a *App) cycle(d int) {
	if len(a.tabs) < 2 {
		return
	}
	i := (a.active + d + len(a.tabs)) % len(a.tabs)
	a.activate(i)
}
