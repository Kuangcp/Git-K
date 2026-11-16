package ui

import (
	"context"
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
	win      *mygo.Window
	repo     *gitx.Repo
	keys     *bindings
	openTab  func(string)
	pickRepo func()
	loadOpen *bool
	loadMenu func(*ui.Context, ui.Element)
	closed   bool

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
	noteUntil  time.Time

	deletePaths []string
	deleteAsk   bool
	revertPaths []string
	revertAsk   bool

	summary   string
	body      *editor.Buffer
	bodyView  editor.State
	amendMode bool
	draftSeq  atomic.Uint64
	draftTxt  string

	pane   paneKind
	branch branchState

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

	sidePercent         float32
	commitPercent       float32
	branchGraphPercent  float32
	branchDetailPercent float32
	settings            *settings.Store
	savedLayout         settings.Layout
	deferStatus         bool

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
		sidePercent:         48,
		commitPercent:       44,
		branchGraphPercent:  38,
		branchDetailPercent: 52,
		cur:                 -1,
		branch:              newBranchState(),
		settings:            st,
	}
	a.loadLayout()
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
	a.refreshStatus()
}

// View builds one frame.
func (a *root) View(c *ui.Context) {
	a.expireNote()
	a.maybeSaveDraft()
	if a.repo == nil {
		a.noRepo(c)
		return
	}
	// 帧首同步 diff：DiffState 被视图元素按指针引用，构建后再重置会让
	// 正在绘制的编辑器读到零值。
	curBefore := a.cur
	if a.pane == paneWorkspace {
		a.syncDiff()
	}
	ui.Column(c).Fill().Children(func() {
		a.topBar(c)
		if a.pane == paneBranches {
			a.branchPane(c)
		} else {
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
		}
		a.footer(c)
	})
	a.deleteDialog(c)
	a.revertDialog(c)
	a.shortcuts(c)
	if a.pane == paneWorkspace && a.cur != curBefore {
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
	l := settings.Layout{
		SidePercent:         clampPercent(a.sidePercent),
		CommitPercent:       clampPercent(a.commitPercent),
		BranchGraphPercent:  clampPercent(a.branchGraphPercent),
		BranchDetailPercent: clampPercent(a.branchDetailPercent),
	}
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
	if a.closed {
		return
	}
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

// loadLayout takes the shared split ratios, as another tab may have moved
// them since this tab was built.
func (a *root) loadLayout() {
	if a.settings == nil {
		return
	}
	l := a.settings.Config().Layout
	a.sidePercent = layoutPercent(l.SidePercent, a.sidePercent)
	a.commitPercent = layoutPercent(l.CommitPercent, a.commitPercent)
	a.branchGraphPercent = layoutPercent(l.BranchGraphPercent, a.branchGraphPercent)
	a.branchDetailPercent = layoutPercent(l.BranchDetailPercent, a.branchDetailPercent)
	a.savedLayout = l
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
	a.amendMode = false
	a.groupDiffs = nil
	a.groupFiles = nil
	a.groupStat.Reset("")
	a.groupPrevCur = -1
	a.resetBranch()
	a.deletePaths = nil
	a.deleteAsk = false
	a.revertPaths = nil
	a.revertAsk = false
	summary, body := splitMessage(repo.PendingMessage())
	a.summary = summary
	a.body.Reset(body)
	a.draftTxt = a.message()
	if !a.deferStatus {
		a.refreshStatus()
	}
	if a.win != nil {
		a.startWatch()
	}
}
