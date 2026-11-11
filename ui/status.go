package ui

import (
	"context"
	"time"

	"git-k/app/gitx"
)

func (a *root) refreshStatus() {
	if a.repo == nil {
		return
	}
	repo := a.repo
	a.statusGen++
	gen := a.statusGen
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if a.win == nil {
		st, err := repo.Status(ctx)
		cancel()
		a.applyStatus(gen, st, err)
		return
	}
	go func() {
		defer cancel()
		st, err := repo.Status(ctx)
		a.win.Update(func() { a.applyStatus(gen, st, err) })
	}()
}

func (a *root) applyStatus(gen uint64, st gitx.Status, err error) {
	if gen != a.statusGen {
		return
	}
	a.loaded = true
	// Watch once the first status is in, so enrolling the tree does not
	// compete with showing the window.
	a.startWatch()
	if err != nil {
		a.statusErr = "读取状态失败：" + err.Error()
		return
	}
	a.statusErr = ""
	prev := ""
	if e := a.currentEntry(); e != nil && !e.header {
		prev = e.row.Key
	}
	a.status = st
	a.entries = buildEntries(st)
	if g, ok := a.groupActive(); ok {
		var rows []gitx.Row
		for _, e := range a.entries {
			if !e.header && e.group == g {
				rows = append(rows, e.row)
			}
		}
		a.groupDiffs = rows
		a.diffKey = ""
	}
	a.pruneChosen()
	a.cur = -1
	for i, e := range a.entries {
		if !e.header && e.key == prev {
			a.cur = i
			break
		}
	}
	if a.cur < 0 && len(a.groupDiffs) == 0 {
		a.cur = firstFile(a.entries)
	}
	if a.cur < 0 {
		a.chosen.Clear()
	}
	a.updateTitle()
}

func (a *root) pruneChosen() {
	if a.chosen.Len() == 0 {
		return
	}
	valid := make(map[string]struct{}, len(a.entries))
	for _, e := range a.entries {
		if !e.header {
			valid[e.key] = struct{}{}
		}
	}
	var gone []string
	for k := range a.chosen.All() {
		if _, ok := valid[k]; !ok {
			gone = append(gone, k)
		}
	}
	for _, k := range gone {
		a.chosen.Remove(k)
	}
}

func buildEntries(st gitx.Status) []listEntry {
	groups := []gitx.Group{gitx.Staged, gitx.Unmerged, gitx.Modified, gitx.Untracked}
	var out []listEntry
	for _, g := range groups {
		n := st.Count(g)
		if n == 0 {
			continue
		}
		out = append(out, listEntry{
			header: true,
			group:  g,
			title:  g.String(),
			count:  n,
			key:    "header:" + g.String(),
		})
		for _, row := range st.Rows {
			if row.Group != g {
				continue
			}
			out = append(out, listEntry{group: g, row: row, key: row.Key})
		}
	}
	return out
}

func firstFile(entries []listEntry) int {
	for i, e := range entries {
		if !e.header {
			return i
		}
	}
	return -1
}

func (a *root) currentEntry() *listEntry {
	if a.cur < 0 || a.cur >= len(a.entries) {
		return nil
	}
	return &a.entries[a.cur]
}

func (a *root) currentRow() (gitx.Row, bool) {
	if e := a.currentEntry(); e != nil && !e.header {
		return e.row, true
	}
	return gitx.Row{}, false
}
