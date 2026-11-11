package ui

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

var heavyDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"venv":         true,
	"target":       true,
	"dist":         true,
	"build":        true,
}

func (a *root) startWatch() {
	if a.repo == nil || a.win == nil || a.watchOn {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.watchCancel = cancel
	a.watchOn = true
	root, gitDir := a.repo.Root, a.repo.GitDir
	go a.watchTree(ctx, root, true)
	go a.watchTree(ctx, gitDir, false)
}

func (a *root) stopWatch() {
	if a.watchCancel != nil {
		a.watchCancel()
		a.watchCancel = nil
	}
	a.watchOn = false
}

// close detaches a tab: stop watching and drop results still in flight.
func (a *root) close() {
	a.closed = true
	a.stopWatch()
}

func (a *root) watchTree(ctx context.Context, dir string, worktree bool) {
	opts := mygo.FileWatchOptions{
		Recursive: true,
		HintsOnly: true,
		Debounce:  150 * time.Millisecond,
	}
	if worktree {
		opts.Ignore = func(path string, isDir bool) bool {
			if path == ".git" {
				return true
			}
			base := filepath.Base(path)
			return isDir && (heavyDirs[base] || strings.HasPrefix(path, ".git/"))
		}
	} else {
		opts.Ignore = func(path string, isDir bool) bool {
			if !isDir {
				return false
			}
			switch path {
			case "objects", "logs", "lfs", "hooks", "modules":
				return true
			}
			return strings.HasPrefix(path, "objects/") || strings.HasPrefix(path, "logs/")
		}
	}
	w, err := mygo.WatchFiles(ctx, dir, opts)
	if err != nil {
		return
	}
	defer w.Close()
	for {
		if _, err := w.Next(ctx); err != nil {
			return
		}
		win := a.win
		if win == nil {
			return
		}
		win.Update(func() { a.refreshStatus() })
	}
}
