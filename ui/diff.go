package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sync"

	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

type diffMode uint8

const (
	diffNone diffMode = iota
	diffPatch
	diffRaw
	diffBinary
	diffEmpty
	diffError
)

// groupFileDiff is one file of a group's aggregated diff, rendered as the
// single-file view renders it.
type groupFileDiff struct {
	path string
	mode diffMode
	diff editor.DiffState
	raw  *editor.Buffer
	view editor.State
}

// fileStat is one file of the group's diffstat.
type fileStat struct {
	path           string
	added, deleted int
}

func (a *root) resetDiff() {
	a.diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
	a.raw.Reset("")
	a.diffMode = diffNone
	a.diffErr = ""
	a.diffPath = ""
	a.diffAdded, a.diffDeleted = 0, 0
}

// syncDiff asks for the current row's patch when it is not the one shown,
// or for every row's patch when a group chip is chosen.
func (a *root) syncDiff() {
	if a.repo == nil {
		return
	}
	if len(a.groupDiffs) > 0 {
		a.syncGroupDiff()
		return
	}
	row, ok := a.currentRow()
	if !ok {
		if a.diffKey != "" || a.diffMode != diffNone {
			a.diffKey = ""
			a.resetDiff()
		}
		return
	}
	key := fmt.Sprintf("%s\x00%d", a.status.DiffKey(row), a.repo.FileStamp(row))
	if key == a.diffKey {
		return
	}
	a.diffKey = key
	a.diffGen++
	gen := a.diffGen
	repo := a.repo
	a.diffPath = row.Path
	a.diffGroup = row.Group
	a.diffLoading = true
	a.diffErr = ""
	a.diffMode = diffNone

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if a.win == nil {
		patch, path, err := repo.Diff(ctx, row)
		cancel()
		a.applyDiff(gen, key, patch, path, err)
		return
	}
	go func() {
		defer cancel()
		patch, path, err := repo.Diff(ctx, row)
		a.win.Update(func() { a.applyDiff(gen, key, patch, path, err) })
	}()
}

// syncGroupDiff loads each file's patch and the group's diffstat; the
// diff pane stacks them as the single-file view shows them.
func (a *root) syncGroupDiff() {
	rows := append([]gitx.Row(nil), a.groupDiffs...)
	key := "group\x00" + rows[0].Group.String()
	for _, row := range rows {
		key += fmt.Sprintf("\x00%s\x00%d", row.Key, a.repo.FileStamp(row))
	}
	if key == a.diffKey {
		return
	}
	a.diffKey = key
	a.diffGen++
	gen := a.diffGen
	repo := a.repo
	a.diffPath = ""
	a.diffGroup = rows[0].Group
	a.diffLoading = true
	a.diffErr = ""
	a.diffMode = diffNone

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if a.win == nil {
		files, stat, added, deleted := loadGroupDiff(ctx, repo, rows)
		cancel()
		a.applyGroupDiff(gen, key, files, stat, added, deleted)
		return
	}
	go func() {
		defer cancel()
		files, stat, added, deleted := loadGroupDiff(ctx, repo, rows)
		a.win.Update(func() { a.applyGroupDiff(gen, key, files, stat, added, deleted) })
	}()
}

func loadGroupDiff(ctx context.Context, repo *gitx.Repo, rows []gitx.Row) (files []groupFileDiff, stat string, added, deleted int) {
	// Each file is a git process; do a few at once so a group does not
	// take one process's latency per file. Results keep the list's order.
	type result struct {
		file groupFileDiff
		stat *fileStat
	}
	results := make([]result, len(rows))
	sem := make(chan struct{}, min(4, len(rows)))
	var wg sync.WaitGroup
	for i, row := range rows {
		wg.Add(1)
		go func(i int, row gitx.Row) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			f, st, ok := loadGroupFile(ctx, repo, row)
			if ok {
				results[i] = result{file: f, stat: st}
			}
		}(i, row)
	}
	wg.Wait()

	var stats []fileStat
	for _, r := range results {
		if r.file.path == "" && r.file.mode == diffNone {
			continue
		}
		files = append(files, r.file)
		if r.stat != nil {
			stats = append(stats, *r.stat)
			added += r.stat.added
			deleted += r.stat.deleted
		}
	}
	return files, formatDiffStat(stats), added, deleted
}

func loadGroupFile(ctx context.Context, repo *gitx.Repo, row gitx.Row) (groupFileDiff, *fileStat, bool) {
	patch, path, err := repo.Diff(ctx, row)
	if path == "" {
		path = row.Path
	}
	f := groupFileDiff{path: path}
	var st *fileStat
	switch {
	case err != nil:
		f.mode = diffError
		f.raw = editor.NewBuffer(err.Error())
	case patch == "":
		return groupFileDiff{}, nil, false
	default:
		patches, perr := editor.ParsePatch(patch)
		switch {
		case perr == nil && len(patches) > 0 && !patches[0].Binary && len(patches[0].Hunks) > 0:
			f.mode = diffPatch
			f.diff = editor.DiffState{Layout: editor.DiffUnified, FitContent: true}
			f.diff.SetPatch(patches[0])
			add, del := f.diff.Stats()
			st = &fileStat{path: path, added: add, deleted: del}
		default:
			f.mode = diffRaw
			f.raw = editor.NewBuffer(patch)
			add, del := countPatchLines(patch)
			st = &fileStat{path: path, added: add, deleted: del}
		}
	}
	if f.raw != nil {
		f.view = editor.State{
			Buffer:      f.raw,
			ReadOnly:    true,
			SoftWrap:    true,
			LineNumbers: editor.LineNumbersOn,
			NoFolding:   true,
			FitContent:  true,
		}
	}
	return f, st, true
}

// formatDiffStat writes the summary git diff --stat would print: each
// file's changes, then the totals.
func formatDiffStat(stats []fileStat) string {
	if len(stats) == 0 {
		return ""
	}
	maxPath, maxCount := 0, 1
	var added, deleted int
	for _, st := range stats {
		maxPath = max(maxPath, len([]rune(gitx.DisplayPath(st.path))))
		maxCount = max(maxCount, len(strconv.Itoa(st.added+st.deleted)))
		added += st.added
		deleted += st.deleted
	}
	var b strings.Builder
	for _, st := range stats {
		path := []rune(gitx.DisplayPath(st.path))
		b.WriteByte(' ')
		b.WriteString(string(path))
		b.WriteString(strings.Repeat(" ", maxPath-len(path)))
		fmt.Fprintf(&b, " | %*d ", maxCount, st.added+st.deleted)
		add, del := st.added, st.deleted
		if total := add + del; total > 50 {
			// Git scales a long bar to the terminal; keep it bounded.
			if add > 0 {
				add = max(1, add*50/total)
			}
			if del > 0 {
				del = max(1, del*50/total)
			}
		}
		b.WriteString(strings.Repeat("+", add))
		b.WriteString(strings.Repeat("-", del))
		b.WriteByte('\n')
	}
	n := len(stats)
	b.WriteByte(' ')
	if n == 1 {
		b.WriteString("1 file changed")
	} else {
		fmt.Fprintf(&b, "%d files changed", n)
	}
	var parts []string
	if added > 0 {
		parts = append(parts, fmt.Sprintf("%d insertion%s(+)", added, plural(added)))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deletion%s(-)", deleted, plural(deleted)))
	}
	if len(parts) > 0 {
		b.WriteString(", ")
		b.WriteString(strings.Join(parts, ", "))
	}
	b.WriteByte('\n')
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (a *root) applyGroupDiff(gen uint64, key string, files []groupFileDiff, stat string, added, deleted int) {
	if gen != a.diffGen || key != a.diffKey {
		return
	}
	a.diffLoading = false
	a.diffPath = ""
	a.diffAdded, a.diffDeleted = added, deleted
	// The group pane does not show these; release a single file's buffers.
	a.raw.Reset("")
	a.diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
	a.groupFiles = files
	a.groupStat.Reset(stat)
	if len(files) == 0 {
		a.diffMode = diffEmpty
		return
	}
	a.diffMode = diffRaw
}

func (a *root) applyDiff(gen uint64, key, patch, path string, err error) {
	if gen != a.diffGen || key != a.diffKey {
		return
	}
	a.diffLoading = false
	a.diffPath = path
	// Release the buffers of the view being replaced before taking the new
	// patch, so switching files does not stack the two.
	applyPatch(&a.diffMode, &a.diff, a.raw, &a.diffAdded, &a.diffDeleted, patch, err)
	if err != nil {
		a.diffErr = err.Error()
	}
}

// applyPatch installs a patch into a diff view, picking how to show it.
// Both the workspace and the branch view use it, so a patch renders the
// same way in either.
func applyPatch(mode *diffMode, diff *editor.DiffState, raw *editor.Buffer, added, deleted *int, patch string, err error) {
	*mode = diffNone
	*added, *deleted = 0, 0
	// Release the old buffers before taking the new one.
	raw.Reset("")
	*diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
	if err != nil {
		*mode = diffError
		return
	}
	if patch == "" {
		*mode = diffEmpty
		return
	}
	patches, perr := editor.ParsePatch(patch)
	if perr != nil || len(patches) == 0 {
		raw.Reset(patch)
		*mode = diffRaw
		*added, *deleted = countPatchLines(patch)
		return
	}
	p := patches[0]
	if p.Binary {
		*mode = diffBinary
		*added, *deleted = countPatchLines(patch)
		return
	}
	if len(p.Hunks) == 0 {
		raw.Reset(patch)
		*mode = diffRaw
		return
	}
	diff.SetPatch(p)
	*mode = diffPatch
	*added, *deleted = diff.Stats()
}

// countPatchLines counts added and removed lines for a patch the viewer
// cannot parse.
func countPatchLines(patch string) (added, deleted int) {
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			deleted++
		}
	}
	return added, deleted
}

// groupActive returns the group whose files are shown together, if any.
func (a *root) groupActive() (gitx.Group, bool) {
	if len(a.groupDiffs) == 0 {
		return 0, false
	}
	return a.groupDiffs[0].Group, true
}

// selectGroup shows every file of g in the diff pane, or a single row
// again when its chip is clicked a second time.
func (a *root) selectGroup(g gitx.Group) {
	if active, ok := a.groupActive(); ok && active == g {
		a.clearGroup()
		if a.groupPrevCur >= 0 && a.groupPrevCur < len(a.entries) {
			a.cur = a.groupPrevCur
		}
	} else {
		if len(a.groupDiffs) == 0 {
			a.groupPrevCur = a.cur
		}
		var rows []gitx.Row
		for _, e := range a.entries {
			if !e.header && e.group == g {
				rows = append(rows, e.row)
			}
		}
		a.groupDiffs = rows
		// 焦点在汇总行：清掉文件行的当前行与多选高亮，之后点任意
		// 文件都会改变 cur，从而退出汇总。
		a.cur = -1
		a.chosen.Clear()
	}
	a.diffKey = ""
}

// clearGroup leaves the group's aggregated diff for a file row.
func (a *root) clearGroup() {
	if len(a.groupDiffs) == 0 && (a.groupStat == nil || a.groupStat.LineCount() == 0) {
		return
	}
	a.groupDiffs = nil
	a.groupFiles = nil
	if a.groupStat != nil {
		a.groupStat.Reset("")
	}
	a.diffKey = ""
}
