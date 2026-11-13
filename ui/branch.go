package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

// paneKind is the repository tab's secondary view.
type paneKind uint8

const (
	paneWorkspace paneKind = iota
	paneBranches
)

// commitFile is one file of the selected commit.
type commitFile struct {
	code    byte
	path    string
	oldPath string
}

// graphRow is the lane layout of one commit: the column of its dot, and
// the lanes above and below it.
type graphRow struct {
	col    int
	before []string
	after  []string
	// edges are the lanes of the commit's parents below it, for drawing
	// the lines into a merge.
	edges []int
	// afterNew marks lanes that this commit created below it; their line
	// starts where a merge curve reaches the row's bottom.
	afterNew []bool
	// merge is true for a commit with more than one parent, drawn as a
	// diamond.
	merge bool
}

// branchState is the branch tab's history, selection and diff cache.
type branchState struct {
	branches   []string
	main       string
	mainPicked bool
	loading    bool
	err        string
	commits    []gitx.Commit
	graph      []graphRow
	sel        int
	list       ui.ListState

	hash         string
	body         string
	msgBuf       *editor.Buffer
	msgView      editor.State
	files        []commitFile
	filesErr     string
	filesLoading bool
	fileSel      int
	fileList     ui.ListState

	diff        editor.DiffState
	raw         *editor.Buffer
	rawView     editor.State
	mode        diffMode
	diffErr     string
	diffPath    string
	diffLoading bool
	diffAdded   int
	diffDeleted int
	diffKey     string

	logGen    uint64
	detailGen uint64
	diffGen   uint64
}

func newBranchState() branchState {
	b := branchState{sel: -1, fileSel: -1}
	b.msgBuf = editor.NewBuffer("")
	b.msgView = editor.State{
		Buffer:      b.msgBuf,
		ReadOnly:    true,
		SoftWrap:    true,
		LineNumbers: editor.LineNumbersOff,
		NoFolding:   true,
		FitContent:  true,
	}
	b.raw = editor.NewBuffer("")
	b.rawView = editor.State{
		Buffer:      b.raw,
		ReadOnly:    true,
		SoftWrap:    true,
		LineNumbers: editor.LineNumbersOn,
		NoFolding:   true,
		FitContent:  true,
	}
	b.diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
	return b
}

// setPane switches the workspace/branch view, loading history the first
// time the branch view opens.
func (a *root) setPane(k paneKind) {
	if a.pane == k {
		return
	}
	a.pane = k
	if k == paneBranches {
		a.loadLog()
	}
}

// resetBranch forgets the history when another repository is opened.
func (a *root) resetBranch() {
	// Keep the generations so results of the previous repository cannot
	// match a new request.
	logGen, detailGen, diffGen := a.branch.logGen, a.branch.detailGen, a.branch.diffGen
	a.pane = paneWorkspace
	a.branch = newBranchState()
	a.branch.logGen, a.branch.detailGen, a.branch.diffGen = logGen, detailGen, diffGen
}

// loadLog reads HEAD and the local branches into the graph.
func (a *root) loadLog() {
	if a.repo == nil {
		return
	}
	b := &a.branch
	b.logGen++
	gen := b.logGen
	b.loading = true
	b.err = ""
	repo := a.repo
	cur := a.status.Branch
	main := b.main
	if !b.mainPicked || main == "" {
		main = cur
		if main == "" {
			main = gitx.AllBranches
		}
	}
	b.main = main
	fallback := append([]string(nil), b.branches...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	load := func() ([]string, string, []gitx.Commit, error) {
		branches, berr := repo.Branches(ctx)
		if berr != nil {
			branches = fallback
		} else if main != gitx.AllBranches && !containsString(branches, main) {
			// The chosen main line is gone: fall back to the current
			// branch, or ALL when it is gone too.
			if containsString(branches, cur) {
				main = cur
			} else {
				main = gitx.AllBranches
			}
		}
		commits, err := repo.Log(ctx, 300, main)
		return branches, main, commits, err
	}
	if a.win == nil {
		branches, chosen, commits, err := load()
		cancel()
		a.applyLog(gen, branches, chosen, commits, err)
		return
	}
	go func() {
		defer cancel()
		branches, chosen, commits, err := load()
		a.win.Update(func() { a.applyLog(gen, branches, chosen, commits, err) })
	}()
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (a *root) applyLog(gen uint64, branches []string, main string, commits []gitx.Commit, err error) {
	b := &a.branch
	if gen != b.logGen {
		return
	}
	b.loading = false
	b.branches = branches
	b.main = main
	if err != nil {
		b.err = err.Error()
		b.commits = nil
		b.graph = nil
		b.sel = -1
		return
	}
	b.err = ""
	b.commits = commits
	b.graph = layoutGraph(commits)
	if len(commits) == 0 {
		b.sel = -1
		a.resetCommit()
		return
	}
	// Keep the chosen commit across a refresh when it is still there.
	idx := 0
	for i, c := range commits {
		if c.Hash == b.hash {
			idx = i
			break
		}
	}
	b.sel = idx
	a.selectCommit(idx)
}

// selectCommit shows commit i's message and files.
func (a *root) selectCommit(i int) {
	b := &a.branch
	if i < 0 || i >= len(b.commits) {
		a.resetCommit()
		b.sel = -1
		return
	}
	b.sel = i
	c := b.commits[i]
	if c.Hash == b.hash {
		return
	}
	b.hash = c.Hash
	b.detailGen++
	gen := b.detailGen
	b.filesLoading = true
	b.filesErr = ""
	b.files = nil
	b.fileSel = -1
	b.body = ""
	b.msgBuf.Reset("")
	a.resetCommitDiff()
	parent := ""
	if len(c.Parents) > 0 {
		parent = c.Parents[0]
	}
	repo := a.repo
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if a.win == nil {
		msg, merr := repo.CommitText(ctx, c.Hash)
		files, ferr := repo.CommitFiles(ctx, c.Hash, parent)
		cancel()
		a.applyCommit(gen, c.Hash, msg, files, merr, ferr)
		return
	}
	go func() {
		defer cancel()
		msg, merr := repo.CommitText(ctx, c.Hash)
		files, ferr := repo.CommitFiles(ctx, c.Hash, parent)
		a.win.Update(func() { a.applyCommit(gen, c.Hash, msg, files, merr, ferr) })
	}()
}

func (a *root) applyCommit(gen uint64, hash, msg string, files []gitx.CommitFile, merr, ferr error) {
	b := &a.branch
	if gen != b.detailGen || hash != b.hash {
		return
	}
	b.filesLoading = false
	if merr != nil {
		b.filesErr = merr.Error()
		return
	}
	b.body = commitBody(msg)
	b.msgBuf.Reset(b.body)
	if ferr != nil {
		b.filesErr = ferr.Error()
		b.files = nil
		return
	}
	b.files = make([]commitFile, len(files))
	for i, f := range files {
		b.files[i] = commitFile{code: f.Code, path: f.Path, oldPath: f.OldPath}
	}
}

// resetCommit clears the commit detail and its diff.
func (a *root) resetCommit() {
	b := &a.branch
	b.hash = ""
	b.detailGen++
	b.body = ""
	b.msgBuf.Reset("")
	b.files = nil
	b.filesErr = ""
	b.filesLoading = false
	b.fileSel = -1
	a.resetCommitDiff()
}

// commitBody drops the subject line, which the header already shows.
func commitBody(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return strings.TrimSpace(msg[i+1:])
	}
	return ""
}

// selectCommitFile loads the diff of the selected file in the selected
// commit against its first parent.
func (a *root) selectCommitFile(i int) {
	b := &a.branch
	if i < 0 || i >= len(b.files) || b.sel < 0 || b.sel >= len(b.commits) {
		b.fileSel = -1
		a.resetCommitDiff()
		return
	}
	b.fileSel = i
	f := b.files[i]
	c := b.commits[b.sel]
	parent := ""
	if len(c.Parents) > 0 {
		parent = c.Parents[0]
	}
	key := c.Hash + "\x00" + f.path
	if key == b.diffKey {
		return
	}
	b.diffKey = key
	b.diffGen++
	gen := b.diffGen
	b.diffPath = f.path
	b.diffLoading = true
	b.mode = diffNone
	b.diffErr = ""
	b.raw.Reset("")
	b.diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
	b.diffAdded, b.diffDeleted = 0, 0
	repo := a.repo
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if a.win == nil {
		patch, err := repo.CommitDiff(ctx, c.Hash, parent, f.path)
		cancel()
		a.applyCommitDiff(gen, key, patch, err)
		return
	}
	go func() {
		defer cancel()
		patch, err := repo.CommitDiff(ctx, c.Hash, parent, f.path)
		a.win.Update(func() { a.applyCommitDiff(gen, key, patch, err) })
	}()
}

func (a *root) applyCommitDiff(gen uint64, key, patch string, err error) {
	b := &a.branch
	if gen != b.diffGen || key != b.diffKey {
		return
	}
	b.diffLoading = false
	applyPatch(&b.mode, &b.diff, b.raw, &b.diffAdded, &b.diffDeleted, patch, err)
	if b.mode == diffError {
		b.diffErr = err.Error()
	}
}

func (a *root) resetCommitDiff() {
	b := &a.branch
	b.diffGen++
	b.diffKey = ""
	b.diffPath = ""
	b.diffLoading = false
	b.diffErr = ""
	b.mode = diffNone
	b.raw.Reset("")
	b.diff = editor.DiffState{Layout: editor.DiffUnified, KeepScrollbars: true}
	b.diffAdded, b.diffDeleted = 0, 0
}

// branchPane is the branch tab: graph on the left, commit detail and
// files top right, diff bottom right.
func (a *root) branchPane(c *ui.Context) {
	t := c.Theme()
	ui.SplitPercent(c, &a.branchGraphPercent, func() {
		ui.Column(c).Fill().BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
			a.commitGraph(c)
		})
	}, func() {
		ui.SplitVerticalPercent(c, &a.branchDetailPercent, func() {
			a.commitDetail(c)
		}, func() {
			a.commitDiffPane(c)
		}).Fill()
	}).Fill()
}

func (a *root) commitGraph(c *ui.Context) {
	t := c.Theme()
	b := &a.branch
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		a.paneTitle(c, "提交图", fmt.Sprintf("%d 个提交", len(b.commits)))
		ui.Row(c).Height(34).PaddingX(8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "主线").FontSize(11).TextColor(t.TextMuted)
			opts := append([]string(nil), b.branches...)
			opts = append(opts, gitx.AllBranches)
			if b.main != "" && !containsString(opts, b.main) {
				opts = append([]string{b.main}, opts...)
			}
			if ui.Select(c, &b.main, opts).MinWidth(160).Changed() {
				b.mainPicked = true
				a.loadLog()
				c.Invalidate()
			}
		})
		switch {
		case b.err != "":
			ui.Text(c, b.err).FontSize(12).TextColor(t.Danger).Padding(12)
		case len(b.commits) == 0 && b.loading:
			ui.Text(c, "正在读取提交…").FontSize(12).TextColor(t.TextMuted).Padding(12)
		case len(b.commits) == 0:
			ui.Text(c, "没有可显示的提交").FontSize(12).TextColor(t.TextMuted).Padding(12)
		default:
			b.list.Selected = &b.sel
			b.list.Key = func(i int) any { return b.commits[i].Hash }
			b.list.Label = func(i int) string { return b.commits[i].Subject }
			list := ui.List(c, &b.list, len(b.commits), func(i int) {
				a.graphRow(c, i)
			}).Grow(1).Padding(0, 4)
			if list.Changed() {
				a.selectCommit(b.sel)
			}
		}
	})
}

func (a *root) graphRow(c *ui.Context, i int) {
	t := c.Theme()
	b := &a.branch
	cm := b.commits[i]
	row := ui.Row(c).Height(30).PaddingX(8).Gap(8).AlignItems(ui.Center)
	row.Children(func() {
		ui.Box(c).Width(graphWidth()).Height(30).Clip().
			Draw(func(p *ui.Painter, r ui.Rect) { drawLanes(p, r, b.graph[i], t) })
		ui.Text(c, cm.Subject).FontSize(12).SingleLine().Grow(1)
		if cm.Refs != "" {
			ui.Text(c, refLabel(cm.Refs)).FontSize(10).TextColor(t.Accent).SingleLine().
				MaxWidth(120).Tooltip(cm.Refs)
		}
		ui.Text(c, shortHash(cm.Hash)).FontSize(10).TextColor(t.TextMuted)
	})
}

// graphWidth leaves room for the lane lines and a little air.
func graphWidth() float32 { return 8 + float32(maxLanes)*12 }

const maxLanes = 6

// drawLanes paints the vertical lanes and the commit's dot.
func drawLanes(p *ui.Painter, r ui.Rect, g graphRow, t *ui.Theme) {
	const step = 12
	mid := r.Y + r.H/2
	for j := range g.before {
		if j >= maxLanes {
			break
		}
		x := r.X + 6 + float32(j)*step
		p.Fill(ui.Rect{X: x - 0.75, Y: r.Y, W: 1.5, H: r.H / 2}, laneColor(t, j), 0)
	}
	for j := range g.after {
		if j >= maxLanes {
			break
		}
		// A lane created by this merge starts where its curve arrives at
		// the row's bottom; drawing it from the middle would leave a
		// stub above the curve.
		if j < len(g.afterNew) && g.afterNew[j] {
			continue
		}
		x := r.X + 6 + float32(j)*step
		p.Fill(ui.Rect{X: x - 0.75, Y: mid, W: 1.5, H: r.H / 2}, laneColor(t, j), 0)
	}
	// A merge bends from the commit's dot to the lane of each parent,
	// which then runs down to the merged commit.
	dotX := r.X + 6 + float32(g.col)*step
	for _, j := range g.edges {
		if j == g.col || j >= maxLanes {
			continue
		}
		x := r.X + 6 + float32(j)*step
		var path ui.Path
		path.MoveTo(dotX, mid).
			CubeTo(dotX, mid+r.H/2, x, r.Y+r.H-r.H/2, x, r.Y+r.H).
			Cap(ui.RoundCap)
		p.StrokePath(&path, 1.5, laneColor(t, j))
	}
	if g.col < maxLanes {
		c := laneColor(t, g.col)
		if g.merge {
			const r = 4.5
			var d ui.Path
			d.MoveTo(dotX, mid-r).LineTo(dotX+r, mid).LineTo(dotX, mid+r).
				LineTo(dotX-r, mid).Close()
			p.FillPath(&d, c)
		} else {
			p.Fill(ui.Rect{X: dotX - 3, Y: mid - 3, W: 6, H: 6}, c, 3)
		}
	}
}

func laneColor(t *ui.Theme, i int) ui.Color {
	colors := [...]ui.Color{t.Accent, t.Success, t.Warning, t.Danger, t.TextMuted, t.Text}
	return colors[i%len(colors)]
}

// graphLane is one column of the graph. Its id stays with the lane as
// commits come and go, so a new parent lane can be told from a lane that
// merely changed its commit.
type graphLane struct {
	hash string
	id   int
}

// layoutGraph assigns commits to lanes by their parents in topological
// order. Each row keeps the lanes above and below it for drawing.
func layoutGraph(commits []gitx.Commit) []graphRow {
	var lanes []graphLane
	nextID := 1
	rows := make([]graphRow, len(commits))
	for i, c := range commits {
		col := indexGraphLane(lanes, c.Hash)
		if col < 0 {
			col = len(lanes)
			lanes = append(lanes, graphLane{hash: c.Hash, id: nextID})
			nextID++
		}
		before := laneHashes(lanes)
		beforeIDs := make(map[int]bool, len(lanes))
		for _, l := range lanes {
			beforeIDs[l.id] = true
		}
		if len(c.Parents) == 0 {
			lanes = append(lanes[:col], lanes[col+1:]...)
		} else {
			lanes[col].hash = c.Parents[0]
			for _, p := range c.Parents[1:] {
				if indexGraphLane(lanes, p) < 0 {
					lanes = append(lanes, graphLane{hash: p, id: nextID})
					nextID++
				}
			}
		}
		lanes = dedupeGraphLanes(lanes)
		after := laneHashes(lanes)
		afterNew := make([]bool, len(lanes))
		for j, l := range lanes {
			afterNew[j] = !beforeIDs[l.id]
		}
		var edges []int
		for _, p := range c.Parents {
			if j := indexGraphLane(lanes, p); j >= 0 {
				edges = append(edges, j)
			}
		}
		rows[i] = graphRow{
			col: col, before: before, after: after, edges: edges,
			afterNew: afterNew, merge: len(c.Parents) > 1,
		}
	}
	return rows
}

func laneHashes(lanes []graphLane) []string {
	out := make([]string, len(lanes))
	for i, l := range lanes {
		out[i] = l.hash
	}
	return out
}

// dedupeGraphLanes keeps one lane per commit: a merge whose parents are
// already tracked joins those lanes instead of drawing a second line.
func dedupeGraphLanes(lanes []graphLane) []graphLane {
	seen := make(map[string]bool, len(lanes))
	out := lanes[:0]
	for _, l := range lanes {
		if l.hash == "" || seen[l.hash] {
			continue
		}
		seen[l.hash] = true
		out = append(out, l)
	}
	return out
}

func indexGraphLane(lanes []graphLane, hash string) int {
	for i, l := range lanes {
		if l.hash == hash {
			return i
		}
	}
	return -1
}

func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// refLabel shortens git's decoration for a row: "HEAD -> main, tag: v1"
// reads "main · v1".
func refLabel(refs string) string {
	var out []string
	for _, r := range strings.Split(refs, ",") {
		r = strings.TrimSpace(r)
		r = strings.TrimPrefix(r, "HEAD -> ")
		r = strings.TrimPrefix(r, "tag: ")
		if r != "" && r != "HEAD" {
			out = append(out, r)
		}
	}
	return strings.Join(out, " · ")
}

func (a *root) commitDetail(c *ui.Context) {
	t := c.Theme()
	b := &a.branch
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		if b.sel < 0 || b.sel >= len(b.commits) {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "选择一个提交查看详情").FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		cm := b.commits[b.sel]
		ui.Column(c).Padding(12).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, shortHash(cm.Hash)).FontSize(11).Font("monospace").
					TextColor(t.Accent)
				if refs := refLabel(cm.Refs); refs != "" {
					ui.Text(c, refs).FontSize(10).TextColor(t.TextMuted).SingleLine().
						MaxWidth(200).Tooltip(cm.Refs)
				}
				ui.Text(c, "").Grow(1)
				ui.Text(c, cm.Time.Format("2006-01-02 15:04")).FontSize(10).
					TextColor(t.TextMuted)
			})
			ui.Text(c, cm.Subject).FontSize(14).Bold().SingleLine().Tooltip(cm.Subject)
			ui.Text(c, cm.Author).FontSize(11).TextColor(t.TextMuted)
		})
		if b.body != "" {
			ui.Scroll(c).Height(72).FillWidth().Children(func() {
				editor.View(c, &b.msgView).FillWidth().Padding(6, 12)
			})
		}
		ui.Row(c).Height(28).PaddingX(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "修改的文件").FontSize(11).Bold().TextColor(t.TextMuted)
			ui.Text(c, fmt.Sprintf("%d", len(b.files))).FontSize(11).
				TextColor(t.TextMuted).PaddingStart(6)
		})
		switch {
		case b.filesErr != "":
			ui.Text(c, b.filesErr).FontSize(11).TextColor(t.Danger).Padding(12).
				SingleLine().Tooltip(b.filesErr)
		case b.filesLoading && len(b.files) == 0:
			ui.Text(c, "正在读取文件…").FontSize(11).TextColor(t.TextMuted).Padding(12)
		case len(b.files) == 0:
			ui.Text(c, "没有文件变更").FontSize(11).TextColor(t.TextMuted).Padding(12)
		default:
			b.fileList.Selected = &b.fileSel
			b.fileList.Key = func(i int) any { return b.files[i].path }
			b.fileList.Label = func(i int) string { return gitx.DisplayPath(b.files[i].path) }
			list := ui.List(c, &b.fileList, len(b.files), func(i int) {
				a.commitFileRow(c, i)
			}).Grow(1).Padding(0, 4)
			if list.Changed() {
				a.selectCommitFile(b.fileSel)
			}
		}
	})
}

func (a *root) commitFileRow(c *ui.Context, i int) {
	b := &a.branch
	f := b.files[i]
	row := ui.Row(c).Height(26).PaddingX(12).Gap(8).AlignItems(ui.Center)
	row.Children(func() {
		fileRowContent(c, f.code, gitx.Modified, f.path, f.oldPath)
	})
}

func (a *root) commitDiffPane(c *ui.Context) {
	t := c.Theme()
	b := &a.branch
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		ui.Row(c).Height(34).PaddingX(12).Gap(10).AlignItems(ui.Center).
			BorderWidth(1, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "差异").FontSize(12).Bold().TextColor(t.TextMuted)
			if b.diffPath != "" {
				ui.Text(c, gitx.DisplayPath(b.diffPath)).FontSize(12).Bold().SingleLine()
			}
			if b.diffAdded > 0 {
				ui.Text(c, fmt.Sprintf("+%d", b.diffAdded)).FontSize(11).TextColor(t.Success)
			}
			if b.diffDeleted > 0 {
				ui.Text(c, fmt.Sprintf("−%d", b.diffDeleted)).FontSize(11).TextColor(t.Danger)
			}
			ui.Text(c, "").Grow(1)
			if b.diffLoading {
				ui.Text(c, "读取中…").FontSize(11).TextColor(t.TextMuted)
			}
		})
		showDiffBody(c, diffRender{
			state:    &b.diff,
			rawView:  &b.rawView,
			mode:     b.mode,
			err:      b.diffErr,
			added:    b.diffAdded,
			deleted:  b.diffDeleted,
			loading:  b.diffLoading,
			empty:    "没有可显示的差异",
			selected: b.diffPath != "",
		})
	})
}
