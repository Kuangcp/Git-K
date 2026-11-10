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
	win      *mygo.Window
	tabs     []*Tab
	active   int
	settings *settings.Store
	openErr  string
	toast    string
	// pending holds launch directories that arrived before the window.
	pending []string
}

// NewApp creates an app with no tabs.
func NewApp(st *settings.Store) *App {
	return &App{settings: st, active: -1}
}

// Attach gives the app its window and opens any launches that arrived
// before it was ready.
func (a *App) Attach(win *mygo.Window) {
	a.win = win
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
	for _, dir := range pending {
		a.OpenAndFocus(dir)
	}
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
		a.pending = append(a.pending, dir)
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
		return nil
	}
	r.onTitle = a.syncTitle
	r.openTab = a.OpenAndFocus
	r.pickRepo = a.chooseRepo
	r.quit = mygo.App.Quit
	a.tabs = append(a.tabs, &Tab{key: key, root: r})
	a.openErr = ""
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

func (a *App) empty(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Padding(32).Children(func() {
		ui.Text(c, "Git-K").FontSize(28).Bold()
		ui.Text(c, "把一个 Git 仓库作为标签打开").FontSize(13).TextColor(t.TextMuted)
		if a.openErr != "" {
			ui.Text(c, a.openErr).FontSize(12).TextColor(t.Danger).SingleLine().Tooltip(a.openErr)
		}
		if ui.PrimaryButton(c, "打开仓库…").Clicked() {
			a.chooseRepo()
		}
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
