package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *root) shortcuts(c *ui.Context) {
	if c.Shortcut(ui.Ctrl, ui.KeyQ) {
		if a.quit != nil {
			a.quit()
		}
		return
	}
	if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyS) {
		a.stageAll()
		return
	}
	if c.Shortcut(ui.Ctrl, ui.KeyS) {
		a.stageSelected()
	}
	if c.Shortcut(ui.Ctrl, ui.KeyU) {
		a.unstageSelected()
	}
	if c.Shortcut(ui.Ctrl, ui.KeyR) || c.Shortcut(0, ui.KeyF5) {
		a.refreshStatus()
	}
	if c.Shortcut(ui.Ctrl, ui.KeyM) {
		a.loadAmend()
		return
	}
	if c.Shortcut(ui.Ctrl, ui.KeyP) {
		a.push()
		return
	}
	if c.Shortcut(ui.Ctrl, ui.KeyL) {
		a.pull()
		return
	}
	if c.Shortcut(ui.Ctrl, ui.KeyEnter) {
		a.doCommit()
	}
}

// chooseRepo asks for a directory and opens it.
func (a *root) chooseRepo() {
	if a.win == nil {
		return
	}
	win := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      win,
			Title:       "选择 Git 仓库",
			Directory:   true,
			ButtonLabel: "打开",
		})
		if err != nil || len(paths) == 0 {
			return
		}
		win.Update(func() {
			if a.openTab != nil {
				a.openTab(paths[0])
				return
			}
			a.openRepo(paths[0])
		})
	}()
}
