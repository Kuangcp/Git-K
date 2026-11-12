package ui

import (
	"context"
	"time"

	"git-k/app/gitx"
)

func (a *root) unborn() bool {
	oid := a.status.HeadOID
	return oid == "" || oid == "(initial)"
}

func (a *root) selectedRows() []gitx.Row {
	if a.chosen.Len() > 0 {
		want := make(map[string]struct{}, a.chosen.Len())
		for k := range a.chosen.All() {
			want[k] = struct{}{}
		}
		var rows []gitx.Row
		for _, e := range a.entries {
			if e.header {
				continue
			}
			if _, ok := want[e.row.Key]; ok {
				rows = append(rows, e.row)
			}
		}
		return rows
	}
	if row, ok := a.currentRow(); ok {
		return []gitx.Row{row}
	}
	return nil
}

// actionRows returns the rows a context menu action applies to: all the
// chosen rows when the row clicked is one of them, else that row alone.
func (a *root) actionRows(row gitx.Row) []gitx.Row {
	if a.chosen.Len() > 0 && a.chosen.Has(row.Key) {
		return a.selectedRows()
	}
	return []gitx.Row{row}
}

// untrackedPaths returns the paths of the untracked rows among rows, in
// order and once each.
// modifiedPaths returns the paths of the modified rows among rows.
func modifiedPaths(rows []gitx.Row) []string {
	seen := make(map[string]struct{}, len(rows))
	var paths []string
	for _, row := range rows {
		if row.Group != gitx.Modified {
			continue
		}
		if _, ok := seen[row.Path]; ok {
			continue
		}
		seen[row.Path] = struct{}{}
		paths = append(paths, row.Path)
	}
	return paths
}

func untrackedPaths(rows []gitx.Row) []string {
	seen := make(map[string]struct{}, len(rows))
	var paths []string
	for _, row := range rows {
		if row.Group != gitx.Untracked {
			continue
		}
		if _, ok := seen[row.Path]; ok {
			continue
		}
		seen[row.Path] = struct{}{}
		paths = append(paths, row.Path)
	}
	return paths
}

func (a *root) stageSelected() {
	if a.busy || a.repo == nil {
		return
	}
	rows := a.selectedRows()
	if len(rows) == 0 {
		return
	}
	a.changeIndex(rows, false)
}

func (a *root) unstageSelected() {
	if a.busy || a.repo == nil {
		return
	}
	rows := a.selectedRows()
	if len(rows) == 0 {
		return
	}
	a.changeIndex(rows, true)
}

func (a *root) stageAll() {
	if a.busy || a.repo == nil {
		return
	}
	a.setBusy(true)
	repo := a.repo
	unborn := a.unborn()
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		err := repo.Stage(ctx, nil, unborn)
		cancel()
		a.update(func() {
			a.setBusy(false)
			a.afterIndexChange(err)
		})
	})
}

func (a *root) unstageAll() {
	if a.busy || a.repo == nil {
		return
	}
	a.setBusy(true)
	repo := a.repo
	unborn := a.unborn()
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		err := repo.Unstage(ctx, nil, unborn)
		cancel()
		a.update(func() {
			a.setBusy(false)
			a.afterIndexChange(err)
		})
	})
}

// changeIndex stages or unstages each selected row by its group: a staged
// row is unstaged, every other row is staged.
func (a *root) changeIndex(rows []gitx.Row, forceUnstage bool) {
	var toStage, toUnstage []string
	seen := map[string]struct{}{}
	add := func(dst *[]string, path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		*dst = append(*dst, path)
	}
	for _, row := range rows {
		if forceUnstage || row.Group == gitx.Staged {
			add(&toUnstage, row.Path)
		} else {
			add(&toStage, row.Path)
		}
	}
	a.setBusy(true)
	repo := a.repo
	unborn := a.unborn()
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var err error
		if len(toStage) > 0 {
			err = repo.Stage(ctx, toStage, unborn)
		}
		if err == nil && len(toUnstage) > 0 {
			err = repo.Unstage(ctx, toUnstage, unborn)
		}
		a.update(func() {
			a.setBusy(false)
			a.afterIndexChange(err)
		})
	})
}

func (a *root) afterIndexChange(err error) {
	if err != nil {
		a.setNote("操作失败：" + err.Error())
		return
	}
	a.setNote("")
	a.diffKey = ""
	a.refreshStatus()
}
