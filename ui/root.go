package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

// listEntry is one row of the status list: a section header or a file.
type listEntry struct {
	header bool
	group  gitx.Group
	title  string
	count  int
	row    gitx.Row
	key    string
}

// root is the state of the window. The view is a function of it, built on
// the main thread; git runs on goroutines and reports back through
// Window.Update.
type root struct {
	win  *mygo.Window
	repo *gitx.Repo

	status    gitx.Status
	entries   []listEntry
	list      ui.ListState
	cur       int
	chosen    ui.Selection[string]
	loaded    bool
	openErr   string
	statusErr string

	busy       bool
	commitBusy bool
	commitErr  string
	note       string

	msg      *editor.Buffer
	msgView  editor.State
	amend    bool
	draftSeq atomic.Uint64
	draftTxt string

	diff        editor.DiffState
	diffKey     string
	diffPath    string
	diffGroup   gitx.Group
	diffLoading bool
	diffMode    diffMode
	diffErr     string
	diffAdded   int
	diffDeleted int

	raw     *editor.Buffer
	rawView editor.State

	sidePercent float32
	diffPercent float32

	statusGen uint64
	diffGen   uint64

	watchCancel context.CancelFunc
	watchOn     bool
}

type diffMode uint8

const (
	diffNone diffMode = iota
	diffPatch
	diffRaw
	diffBinary
	diffEmpty
	diffError
)

// NewRoot opens the repository at dir, reads its status once, and returns
// the window state. An empty dir means the current directory.
func NewRoot(dir string) *root {
	a := &root{
		sidePercent: 38,
		diffPercent: 46,
		cur:         -1,
	}
	a.msg = editor.NewBuffer("")
	a.msgView = editor.State{
		Buffer:      a.msg,
		Placeholder: "提交信息\n\n第一行是摘要，空行后写正文。",
		SoftWrap:    true,
		LineNumbers: editor.LineNumbersOff,
		NoFolding:   true,
	}
	a.raw = editor.NewBuffer("")
	a.rawView = editor.State{
		Buffer:      a.raw,
		ReadOnly:    true,
		SoftWrap:    true,
		LineNumbers: editor.LineNumbersOn,
		NoFolding:   true,
	}
	a.diff = editor.DiffState{
		Layout:         editor.DiffUnified,
		KeepScrollbars: true,
	}
	a.list.Selected = &a.cur
	a.list.Selection = &a.chosen
	a.openRepo(dir)
	return a
}

// Attach gives the root its window, starts watching the repository, and
// refreshes the status.
func (a *root) Attach(win *mygo.Window) {
	a.win = win
	a.updateTitle()
	a.refreshStatus()
	a.startWatch()
}

// View builds one frame.
func (a *root) View(c *ui.Context) {
	a.maybeSaveDraft()
	if a.repo == nil {
		a.noRepo(c)
		return
	}
	ui.Column(c).Fill().Children(func() {
		a.topBar(c)
		ui.SplitVerticalPercent(c, &a.diffPercent, func() {
			ui.SplitPercent(c, &a.sidePercent, func() {
				a.statusPane(c)
			}, func() {
				a.commitPane(c)
			}).Fill()
		}, func() {
			a.diffPane(c)
		}).Fill()
		a.footer(c)
	})
	a.syncDiff()
	a.shortcuts(c)
}

// update runs fn on the main thread, or at once in a test with no window.
func (a *root) update(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// background runs fn off the main thread, or at once in a test.
func (a *root) background(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	go fn()
}

func (a *root) updateTitle() {
	if a.win == nil || a.repo == nil {
		return
	}
	branch := a.status.Branch
	if branch == "" {
		branch = "(无分支)"
	}
	if a.status.Detached {
		branch = "detached"
	}
	a.win.SetTitle(fmt.Sprintf("%s — %s — git-k", a.repo.Name, branch))
}

func (a *root) openRepo(dir string) {
	a.stopWatch()
	a.draftSeq.Add(1)
	a.statusGen++
	a.diffGen++
	repo, err := gitx.Open(dir)
	if err != nil {
		a.repo = nil
		a.openErr = err.Error()
		return
	}
	a.repo = repo
	a.openErr = ""
	a.statusErr = ""
	a.status = gitx.Status{}
	a.entries = nil
	a.cur = -1
	a.chosen.Clear()
	a.loaded = false
	a.diffKey = ""
	a.resetDiff()
	a.commitErr = ""
	a.note = ""
	a.msg.Reset(repo.CommitMessage())
	a.draftTxt = a.msg.String()
	a.refreshStatus()
	a.updateTitle()
	if a.win != nil {
		a.startWatch()
	}
}

func (a *root) resetDiff() {
	a.diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
	a.raw.Reset("")
	a.diffMode = diffNone
	a.diffErr = ""
	a.diffPath = ""
	a.diffAdded, a.diffDeleted = 0, 0
}

// ---- status refresh -------------------------------------------------------

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
	a.pruneChosen()
	a.cur = -1
	for i, e := range a.entries {
		if !e.header && e.key == prev {
			a.cur = i
			break
		}
	}
	if a.cur < 0 {
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

// ---- selection and diff ---------------------------------------------------

// syncDiff asks for the current row's patch when it is not the one shown.
func (a *root) syncDiff() {
	if a.repo == nil {
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

func (a *root) applyDiff(gen uint64, key, patch, path string, err error) {
	if gen != a.diffGen || key != a.diffKey {
		return
	}
	a.diffLoading = false
	a.diffPath = path
	if err != nil {
		a.diffMode = diffError
		a.diffErr = err.Error()
		return
	}
	if patch == "" {
		a.diffMode = diffEmpty
		a.diffAdded, a.diffDeleted = 0, 0
		return
	}
	patches, perr := editor.ParsePatch(patch)
	if perr != nil || len(patches) == 0 {
		a.raw.Reset(patch)
		a.diffMode = diffRaw
		a.diffAdded, a.diffDeleted = countPatchLines(patch)
		return
	}
	p := patches[0]
	if p.Binary {
		a.diffMode = diffBinary
		a.diffAdded, a.diffDeleted = countPatchLines(patch)
		return
	}
	if len(p.Hunks) == 0 {
		a.raw.Reset(patch)
		a.diffMode = diffRaw
		a.diffAdded, a.diffDeleted = 0, 0
		return
	}
	a.diff.SetPatch(p)
	a.diffMode = diffPatch
	a.diffAdded, a.diffDeleted = a.diff.Stats()
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

// ---- git actions ----------------------------------------------------------

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
		a.note = "操作失败：" + err.Error()
		return
	}
	a.note = ""
	a.diffKey = ""
	a.refreshStatus()
}

func (a *root) setBusy(busy bool) {
	a.busy = busy
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *root) doCommit() {
	if a.commitBusy || a.repo == nil {
		return
	}
	msg := a.msg.String()
	if strings.TrimSpace(msg) == "" {
		a.commitErr = "请填写提交信息"
		return
	}
	if !a.status.HasStaged() && !a.amend {
		a.commitErr = "没有已暂存的改动"
		return
	}
	a.commitBusy = true
	a.commitErr = ""
	repo := a.repo
	amend := a.amend
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		out, err := repo.Commit(ctx, msg, amend)
		cancel()
		a.update(func() {
			a.commitBusy = false
			if err != nil {
				a.commitErr = err.Error()
				return
			}
			a.commitErr = ""
			a.note = "提交成功"
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "[") {
					a.note = "已提交 " + line
					break
				}
			}
			a.draftSeq.Add(1)
			_ = repo.SaveCommitMessage("")
			a.msg.Reset("")
			a.draftTxt = ""
			a.amend = false
			a.diffKey = ""
			a.refreshStatus()
		})
	})
}

func (a *root) maybeSaveDraft() {
	if a.repo == nil {
		return
	}
	s := a.msg.String()
	if s == a.draftTxt {
		return
	}
	a.commitErr = ""
	a.note = ""
	a.draftTxt = s
	if a.win == nil {
		return
	}
	repo := a.repo
	seq := a.draftSeq.Add(1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		if a.draftSeq.Load() != seq {
			return
		}
		_ = repo.SaveCommitMessage(s)
	}()
}

// ---- watching -------------------------------------------------------------

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

var heavyDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"venv":         true,
	"target":       true,
	"dist":         true,
	"build":        true,
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

// ---- shortcuts ------------------------------------------------------------

func (a *root) shortcuts(c *ui.Context) {
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
		win.Update(func() { a.openRepo(paths[0]) })
	}()
}
