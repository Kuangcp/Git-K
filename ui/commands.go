package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// shortcuts dispatches the repository actions from the keymap. The
// window actions (minimize, tabs) run in App.shortcuts.
func (a *root) shortcuts(c *ui.Context) {
	if a.keys.capturing() {
		return
	}
	if a.repo == nil {
		return
	}
	for _, act := range keyActions {
		if act.Scope != scopeRepo {
			continue
		}
		if a.pane != paneWorkspace && workspaceOnly(act.ID) {
			continue
		}
		if !a.keys.hit(c, act.ID) {
			continue
		}
		switch act.ID {
		case "workspace":
			a.setPane(paneWorkspace)
			c.Invalidate()
		case "branches":
			a.setPane(paneBranches)
			c.Invalidate()
		case "stage":
			a.stageSelected()
		case "stageAll":
			a.stageAll()
		case "unstage":
			a.unstageSelected()
		case "refresh":
			a.refreshStatus()
			if a.pane == paneBranches {
				a.loadLog()
			}
		case "amend":
			a.loadAmend()
		case "push":
			a.push()
		case "pull":
			a.pull()
		case "commit":
			a.doCommit()
		}
		return
	}
}

// workspaceOnly reports actions that change the index or commit and so
// only apply to the workspace tab.
func workspaceOnly(id string) bool {
	switch id {
	case "stage", "stageAll", "unstage", "amend", "commit":
		return true
	}
	return false
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
