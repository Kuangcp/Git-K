package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
	"git-k/app/settings"
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
	quit func()

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

	deletePaths []string
	deleteAsk   bool

	summary  string
	body     *editor.Buffer
	bodyView editor.State
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

	sidePercent   float32
	commitPercent float32
	settings      *settings.Store
	savedLayout   settings.Layout
	deferStatus   bool

	statusGen uint64
	diffGen   uint64

	watchCancel context.CancelFunc
	watchOn     bool

	// groupDiffs holds rows whose diffs are aggregated when a group row
	// is chosen. Nil means the current file (or nothing).
	groupDiffs    []gitx.Row
	groupFiles    []groupFileDiff
	groupStat     *editor.Buffer
	groupStatView editor.State
	groupPrevCur  int
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

// NewRoot opens the repository at dir, reads its status once, and returns
// the window state. An empty dir means the current directory. It keeps no
// settings, as tests want.
func NewRoot(dir string) *root { return newRoot(dir, nil, false) }

// NewRootWithSettings is NewRoot with the layout percentages loaded from
// st, whose updates it saves when a split moves.
func NewRootWithSettings(dir string, st *settings.Store) *root {
	return newRoot(dir, st, false)
}

// NewRootLazy opens the repository but leaves the first status read to
// Attach, so the window shows before git runs.
func NewRootLazy(dir string, st *settings.Store) *root {
	return newRoot(dir, st, true)
}

func newRoot(dir string, st *settings.Store, lazy bool) *root {
	a := &root{
		sidePercent:   48,
		commitPercent: 44,
		cur:           -1,
		quit:          mygo.App.Quit,
		settings:      st,
	}
	if st != nil {
		l := st.Config().Layout
		a.sidePercent = layoutPercent(l.SidePercent, a.sidePercent)
		a.commitPercent = layoutPercent(l.CommitPercent, a.commitPercent)
		a.savedLayout = l
	}
	a.body = editor.NewBuffer("")
	a.bodyView = editor.State{
		Buffer:      a.body,
		Placeholder: "详细描述…",
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
	a.groupStat = editor.NewBuffer("")
	a.groupStatView = editor.State{
		Buffer:      a.groupStat,
		ReadOnly:    true,
		LineNumbers: editor.LineNumbersOff,
		NoFolding:   true,
		FitContent:  true,
	}
	a.list.Selected = &a.cur
	a.list.Selection = &a.chosen
	a.deferStatus = lazy
	a.openRepo(dir)
	return a
}

// Attach gives the root its window, starts watching the repository, and
// refreshes the status.
func (a *root) Attach(win *mygo.Window) {
	a.win = win
	a.updateTitle()
	a.refreshStatus()
}

// View builds one frame.
func (a *root) View(c *ui.Context) {
	a.maybeSaveDraft()
	if a.repo == nil {
		a.noRepo(c)
		return
	}
	// 帧首同步 diff：DiffState 被视图元素按指针引用，构建后再重置会让
	// 正在绘制的编辑器读到零值。
	a.syncDiff()
	curBefore := a.cur
	ui.Column(c).Fill().Children(func() {
		a.topBar(c)
		ui.SplitPercent(c, &a.sidePercent, func() {
			// 左列：提交信息在上，差异在下；右列是状态。
			ui.Column(c).Fill().BorderWidth(0, 1, 0, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.SplitVerticalPercent(c, &a.commitPercent, func() {
					a.commitPane(c)
				}, func() {
					a.diffPane(c)
				}).Fill()
			})
		}, func() {
			a.statusPane(c)
		}).Fill()
		a.footer(c)
	})
	a.deleteDialog(c)
	a.shortcuts(c)
	if a.cur != curBefore {
		// 点击选中的新行在本帧才写入 cur，下一帧再同步 diff。
		c.Invalidate()
	}
	a.saveLayout()
}

// saveLayout stores the split ratios when they changed, such as while a
// divider is dragged.
func (a *root) saveLayout() {
	if a.settings == nil {
		return
	}
	l := settings.Layout{SidePercent: clampPercent(a.sidePercent), CommitPercent: clampPercent(a.commitPercent)}
	if l == a.savedLayout {
		return
	}
	a.savedLayout = l
	a.settings.Update(func(c *settings.Config) { c.Layout = l })
}

func layoutPercent(v, def float32) float32 {
	if v <= 0 || v >= 100 {
		return def
	}
	return v
}

// clampPercent keeps a saved split a real proportion: 0 and 100 are how a
// split's zero value reads, so they must not hide a pane on the next run.
func clampPercent(v float32) float32 {
	return min(max(v, 1), 99)
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
	a.groupDiffs = nil
	a.groupFiles = nil
	a.groupStat.Reset("")
	a.groupPrevCur = -1
	a.deletePaths = nil
	a.deleteAsk = false
	summary, body := splitMessage(repo.CommitMessage())
	a.summary = summary
	a.body.Reset(body)
	a.draftTxt = a.message()
	if !a.deferStatus {
		a.refreshStatus()
	}
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

// ---- selection and diff ---------------------------------------------------

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
	var stats []fileStat
	for _, row := range rows {
		patch, path, err := repo.Diff(ctx, row)
		if path == "" {
			path = row.Path
		}
		f := groupFileDiff{path: path}
		switch {
		case err != nil:
			f.mode = diffError
			f.raw = editor.NewBuffer(err.Error())
		case patch == "":
			continue
		default:
			patches, perr := editor.ParsePatch(patch)
			switch {
			case perr == nil && len(patches) > 0 && !patches[0].Binary && len(patches[0].Hunks) > 0:
				f.mode = diffPatch
				f.diff = editor.DiffState{Layout: editor.DiffUnified, FitContent: true}
				f.diff.SetPatch(patches[0])
				add, del := f.diff.Stats()
				added += add
				deleted += del
				stats = append(stats, fileStat{path: path, added: add, deleted: del})
			default:
				f.mode = diffRaw
				f.raw = editor.NewBuffer(patch)
				add, del := countPatchLines(patch)
				added += add
				deleted += del
				stats = append(stats, fileStat{path: path, added: add, deleted: del})
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
		files = append(files, f)
	}
	return files, formatDiffStat(stats), added, deleted
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
	a.raw.Reset("")
	a.diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
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
	msg := a.message()
	if strings.TrimSpace(a.summary) == "" {
		a.commitErr = "请填写提交摘要"
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
			a.summary = ""
			a.body.Reset("")
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
	s := a.message()
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

// message composes the summary and the extended description as git
// stores them.
func (a *root) message() string {
	summary := strings.TrimSpace(a.summary)
	body := strings.TrimRight(a.body.String(), "\r\n")
	if body == "" {
		return summary
	}
	return summary + "\n\n" + body + "\n"
}

// splitMessage reads a draft back into its summary and its description.
func splitMessage(s string) (summary, body string) {
	s = strings.TrimRight(s, "\r\n")
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return s[:i], strings.TrimLeft(s[i+2:], "\r\n")
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
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
