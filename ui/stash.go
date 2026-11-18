package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

// stashState is the stash tab's list, selection and diff.
type stashState struct {
	entries   []gitx.Stash
	sel       int
	preview   bool
	list      ui.ListState
	loading   bool
	err       string
	actionErr string
	busy      bool

	files       []groupFileDiff
	fileSel     int
	allFiles    bool
	fileList    ui.ListState
	stat        *editor.Buffer
	statView    editor.State
	diffLoading bool
	diffErr     string
	diffKey     string
	diffAdded   int
	diffDeleted int

	dropAsk bool
	dropRef string

	loadGen uint64
	diffGen uint64
}

func newStashState() stashState {
	s := stashState{sel: -1, fileSel: -1}
	s.stat = editor.NewBuffer("")
	s.statView = editor.State{
		Buffer:      s.stat,
		ReadOnly:    true,
		LineNumbers: editor.LineNumbersOff,
		NoFolding:   true,
		FitContent:  true,
	}
	return s
}

// resetStash forgets the list when another repository is opened, keeping
// the generations so old results cannot land in the new repository.
func (a *root) resetStash() {
	loadGen, diffGen := a.stash.loadGen, a.stash.diffGen
	a.stash = newStashState()
	a.stash.loadGen, a.stash.diffGen = loadGen, diffGen
}

// loadStashes reads git stash list, keeping the chosen entry when it is
// still there.
func (a *root) loadStashes() {
	if a.repo == nil {
		return
	}
	s := &a.stash
	s.loadGen++
	gen := s.loadGen
	s.loading = true
	s.err = ""
	repo := a.repo
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if a.win == nil {
		stashes, err := repo.Stashes(ctx)
		cancel()
		a.applyStashes(gen, stashes, err)
		return
	}
	go func() {
		defer cancel()
		stashes, err := repo.Stashes(ctx)
		a.win.Update(func() { a.applyStashes(gen, stashes, err) })
	}()
}

func (a *root) applyStashes(gen uint64, stashes []gitx.Stash, err error) {
	s := &a.stash
	if gen != s.loadGen {
		return
	}
	s.loading = false
	if err != nil {
		s.err = "读取 Stash 失败：" + err.Error()
		return
	}
	prevRef := ""
	if s.sel >= 0 && s.sel < len(s.entries) {
		prevRef = s.entries[s.sel].Ref
	}
	wasEmpty := len(s.entries) == 0
	wasPreview := s.preview
	s.entries = stashes
	s.sel = -1
	for i, st := range stashes {
		if st.Ref == prevRef {
			s.sel = i
			break
		}
	}
	switch {
	case len(stashes) == 0:
		s.preview = false
		s.resetDiff()
	case s.sel < 0:
		// First load shows the newest stash; a vanished stash leaves the
		// next one chosen without forcing a preview the user had closed.
		s.sel = 0
		s.preview = wasEmpty || wasPreview
	default:
		// The chosen stash is still there: keep the user's preview state.
	}
}

// syncStashDiff asks for the chosen stash's patch at the start of a frame,
// before the diff elements are built.
func (a *root) syncStashDiff() {
	s := &a.stash
	if a.pane != paneStashes || !s.preview || s.sel < 0 || s.sel >= len(s.entries) {
		return
	}
	st := s.entries[s.sel]
	key := st.Ref + "\x00" + st.Hash
	if key == s.diffKey {
		return
	}
	s.diffKey = key
	s.diffGen++
	gen := s.diffGen
	s.diffLoading = true
	s.diffErr = ""
	s.files = nil
	s.stat.Reset("")
	s.diffAdded, s.diffDeleted = 0, 0
	repo := a.repo
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if a.win == nil {
		patch, err := repo.StashDiff(ctx, st.Ref)
		cancel()
		a.applyStashDiff(gen, key, patch, err)
		return
	}
	go func() {
		defer cancel()
		patch, err := repo.StashDiff(ctx, st.Ref)
		a.win.Update(func() { a.applyStashDiff(gen, key, patch, err) })
	}()
}

func (a *root) applyStashDiff(gen uint64, key, patch string, err error) {
	s := &a.stash
	if gen != s.diffGen || key != s.diffKey {
		return
	}
	s.diffLoading = false
	s.diffErr = ""
	s.files = nil
	s.stat.Reset("")
	s.diffAdded, s.diffDeleted = 0, 0
	if err != nil {
		s.diffErr = err.Error()
		return
	}
	files, stat, added, deleted := groupFilesFromPatch(patch)
	s.files = files
	s.stat.Reset(stat)
	s.diffAdded, s.diffDeleted = added, deleted
	s.fileSel = -1
	s.allFiles = false
	if len(files) > 0 {
		s.fileSel = 0
	}
}

// resetDiff clears the stash preview's diff.
func (s *stashState) resetDiff() {
	s.diffGen++
	s.diffKey = ""
	s.diffLoading = false
	s.diffErr = ""
	s.files = nil
	s.fileSel = -1
	s.allFiles = false
	s.stat.Reset("")
	s.diffAdded, s.diffDeleted = 0, 0
}

// clearStashPreview leaves the stash diff for the workspace's. The cached
// diff stays, so reopening the same stash shows it at once.
func (a *root) clearStashPreview() {
	a.stash.preview = false
}

// stashPane is the Stash tab's left top: the stash list replacing the
// commit panel; the chosen file's diff shows below it.
func (a *root) stashPane(c *ui.Context) {
	t := c.Theme()
	s := &a.stash
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		a.paneTitle(c, "Stash", fmt.Sprintf("%d 个", len(s.entries)))
		if s.actionErr != "" {
			ui.Text(c, s.actionErr).FontSize(12).TextColor(t.Danger).SingleLine().
				Padding(12, 12).Tooltip(s.actionErr)
		}
		switch {
		case s.err != "":
			ui.Text(c, s.err).FontSize(12).TextColor(t.Danger).Padding(12)
			return
		case s.loading && len(s.entries) == 0:
			ui.Text(c, "正在读取 Stash…").FontSize(12).TextColor(t.TextMuted).Padding(12)
			return
		case len(s.entries) == 0:
			ui.Text(c, "没有 Stash").FontSize(12).TextColor(t.TextMuted).Padding(12)
			return
		}
		s.list.Key = func(i int) any { return s.entries[i].Ref }
		s.list.Label = func(i int) string { return s.entries[i].Message }
		ui.List(c, &s.list, len(s.entries), func(i int) {
			a.stashRow(c, i)
		}).Grow(1).MinHeight(0).Padding(0, 4)
		if a.note != "" {
			ui.Text(c, a.note).FontSize(11).TextColor(t.Success).SingleLine().
				Padding(8, 12).Tooltip(a.note)
		}
	})
}

func (a *root) stashRow(c *ui.Context, i int) {
	t := c.Theme()
	s := &a.stash
	st := s.entries[i]
	row := ui.ButtonBase(c).FillWidth().MinWidth(0).Height(46).PaddingX(12).Gap(8).
		AlignItems(ui.Center).Label(stashSubject(st.Message))
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("Pop").Disabled(s.busy).Chosen() {
			a.popStash(st.Ref)
		}
		if m.Item("Drop").Danger().Disabled(s.busy).Chosen() {
			s.dropRef = st.Ref
			s.dropAsk = true
			c.Invalidate()
		}
		m.Separator()
		if m.Item("复制 Stash 名").Chosen() {
			mygo.Clipboard.WriteText(st.Ref)
		}
		if st.Base != "" && m.Item("复制 Commit ID").Chosen() {
			mygo.Clipboard.WriteText(commitID(st.Base))
		}
	})
	row.Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, st.Ref).FontSize(11).Bold().TextColor(t.Accent).SingleLine()
				if st.Base != "" {
					ui.Text(c, shortHash(st.Base)).FontSize(10).TextColor(t.TextMuted).
						Font("monospace").SingleLine().Tooltip(st.Base)
				}
				ui.Text(c, "").Grow(1)
				if !st.When.IsZero() {
					ui.Text(c, st.When.Format("2006-01-02 15:04")).FontSize(10).
						TextColor(t.TextMuted).SingleLine()
				}
			})
			ui.Text(c, stashSubject(st.Message)).FontSize(11).TextColor(t.TextMuted).
				SingleLine().Tooltip(st.Message)
		})
	})
	if i == s.sel && s.preview {
		row = row.Background(t.SurfaceHover).Radius(t.Radius).
			BorderWidth(0, 0, 0, 2).BorderColor(t.Accent)
	} else if row.Hovered() {
		row = row.Background(t.SurfaceHover).Radius(t.Radius)
	}
	if row.Clicked() {
		s.sel = i
		s.preview = true
		c.Invalidate()
	}
}

// stashSubject shows the stash message without git's "WIP on branch:"
// prefix, keeping the base commit's own subject.
func stashSubject(msg string) string {
	if i := strings.Index(msg, ": "); i >= 0 &&
		(strings.HasPrefix(msg, "WIP on ") || strings.HasPrefix(msg, "On ")) {
		rest := msg[i+2:]
		// Drop the short base hash that follows the branch name.
		if j := strings.IndexByte(rest, ' '); j == 7 {
			return rest[j+1:]
		}
		return rest
	}
	return msg
}

// stashStatusPane lists the chosen stash's changed files, like the
// workspace's status panel lists the worktree's.
func (a *root) stashStatusPane(c *ui.Context) {
	t := c.Theme()
	s := &a.stash
	ui.Column(c).Fill().Children(func() {
		a.paneTitle(c, "Stash 文件", fmt.Sprintf("%d 个文件", len(s.files)))
		switch {
		case s.diffErr != "":
			ui.Text(c, s.diffErr).FontSize(12).TextColor(t.Danger).Padding(12)
		case s.diffLoading && len(s.files) == 0:
			ui.Text(c, "正在读取文件…").FontSize(12).TextColor(t.TextMuted).Padding(12)
		case len(s.files) == 0:
			ui.Text(c, "这个 Stash 没有文件").FontSize(12).TextColor(t.TextMuted).Padding(12)
		default:
			a.stashAllRow(c)
			s.fileList.Selected = &s.fileSel
			s.fileList.Key = func(i int) any { return s.files[i].path }
			s.fileList.Label = func(i int) string { return gitx.DisplayPath(s.files[i].path) }
			list := ui.List(c, &s.fileList, len(s.files), func(i int) {
				a.stashFileRow(c, i)
			}).Grow(1).MinHeight(0).Padding(0, 4)
			if list.Changed() {
				s.allFiles = false
				c.Invalidate()
			}
		}
	})
}

// stashAllRow is the aggregate row above the file list: how many files
// were added, modified or deleted, and a click shows every diff.
func (a *root) stashAllRow(c *ui.Context) {
	t := c.Theme()
	s := &a.stash
	added, modified, deleted := stashFileCounts(s.files)
	bar := ui.ButtonBase(c).FillWidth().Height(30).PaddingX(12).Gap(10).
		AlignItems(ui.Center).Label(fmt.Sprintf("全部 %d 个文件", len(s.files)))
	bar.Children(func() {
		title := ui.Text(c, fmt.Sprintf("全部 %d 个文件", len(s.files))).FontSize(11).Bold()
		if s.allFiles {
			title.TextColor(t.Accent)
		} else {
			title.TextColor(t.TextMuted)
		}
		if added > 0 {
			ui.Text(c, fmt.Sprintf("新增 %d", added)).FontSize(11).TextColor(t.Success)
		}
		if modified > 0 {
			ui.Text(c, fmt.Sprintf("修改 %d", modified)).FontSize(11).TextColor(t.Warning)
		}
		if deleted > 0 {
			ui.Text(c, fmt.Sprintf("删除 %d", deleted)).FontSize(11).TextColor(t.Danger)
		}
	})
	switch {
	case s.allFiles:
		bar = bar.Background(t.SurfaceHover).BorderWidth(0, 0, 0, 2).BorderColor(t.Accent)
	case bar.Hovered():
		bar = bar.Background(t.SurfaceHover)
	}
	if bar.Clicked() {
		s.allFiles = true
		s.fileSel = -1
		c.Invalidate()
	}
}

// stashFileCounts groups the patch's files by their status letter; a
// rename or type change counts as a modification.
func stashFileCounts(files []groupFileDiff) (added, modified, deleted int) {
	for _, f := range files {
		switch f.code {
		case 'A':
			added++
		case 'D':
			deleted++
		default:
			modified++
		}
	}
	return added, modified, deleted
}

func (a *root) stashFileRow(c *ui.Context, i int) {
	s := &a.stash
	f := s.files[i]
	code := f.code
	if code == 0 {
		code = 'M'
	}
	row := ui.Row(c).Height(28).PaddingX(12).Gap(8).AlignItems(ui.Center)
	row.Children(func() {
		fileRowContent(c, code, gitx.Modified, f.path, f.oldPath)
	})
}

// stashDiffPane shows the chosen file of the chosen stash.
func (a *root) stashDiffPane(c *ui.Context) {
	t := c.Theme()
	s := &a.stash
	var st gitx.Stash
	if s.sel >= 0 && s.sel < len(s.entries) {
		st = s.entries[s.sel]
	}
	allFiles := s.allFiles && len(s.files) > 0
	var file *groupFileDiff
	if !allFiles && s.fileSel >= 0 && s.fileSel < len(s.files) {
		file = &s.files[s.fileSel]
	}
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).
		Padding(0, 0, diffBottomInset, 0).Children(func() {
		ui.Row(c).Height(34).PaddingX(12).Gap(10).AlignItems(ui.Center).
			BorderWidth(1, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "差异").FontSize(12).Bold().TextColor(t.TextMuted)
			added, deleted := s.diffAdded, s.diffDeleted
			switch {
			case allFiles:
				ui.Text(c, fmt.Sprintf("%d 个文件", len(s.files))).FontSize(12).Bold()
			case file != nil:
				ui.Text(c, gitx.DisplayPath(file.path)).FontSize(12).Bold().SingleLine()
				added, deleted = file.added, file.deleted
			case st.Ref != "":
				ui.Text(c, st.Ref).FontSize(12).Bold().SingleLine()
				if st.Base != "" {
					ui.Text(c, shortHash(st.Base)).FontSize(10).TextColor(t.TextMuted).
						Font("monospace").Tooltip(st.Base)
				}
			}
			if added > 0 {
				ui.Text(c, fmt.Sprintf("+%d", added)).FontSize(11).TextColor(t.Success)
			}
			if deleted > 0 {
				ui.Text(c, fmt.Sprintf("−%d", deleted)).FontSize(11).TextColor(t.Danger)
			}
			ui.Text(c, "").Grow(1)
			if s.diffLoading {
				ui.Text(c, "读取中…").FontSize(11).TextColor(t.TextMuted)
			}
			if ui.Button(c, "工作区差异").Height(22).PaddingX(8).Clicked() {
				a.clearStashPreview()
				c.Invalidate()
			}
		})
		switch {
		case s.diffErr != "":
			ui.Column(c).Fill().Center().Padding(16).Children(func() {
				ui.Text(c, "无法显示差异").FontSize(12).Bold()
				ui.Text(c, s.diffErr).FontSize(11).TextColor(t.Danger).SingleLine().
					Tooltip(s.diffErr)
			})
		case s.diffLoading && len(s.files) == 0:
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "正在读取差异…").FontSize(12).TextColor(t.TextMuted)
			})
		case len(s.files) == 0:
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "这个 Stash 没有可显示的差异").FontSize(12).TextColor(t.TextMuted)
			})
		case allFiles:
			groupDiffPane(c, s.files, s.stat, &s.statView)
		case file == nil:
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "选择一个文件查看差异").FontSize(12).TextColor(t.TextMuted)
			})
		default:
			switch file.mode {
			case diffPatch:
				fittedDiff(c, &file.diff)
			case diffError:
				ui.Column(c).Fill().Center().Padding(16).Children(func() {
					ui.Text(c, file.raw.String()).FontSize(11).TextColor(t.Danger).
						SingleLine().Tooltip(file.raw.String())
				})
			default:
				fittedText(c, &file.view)
			}
		}
	})
}

// popStash applies and drops a stash.
func (a *root) popStash(ref string) {
	a.stashAction(ref, false)
}

// dropStash removes a stash without applying it.
func (a *root) dropStash(ref string) {
	a.stashAction(ref, true)
}

func (a *root) stashAction(ref string, drop bool) {
	s := &a.stash
	if s.busy || a.repo == nil {
		return
	}
	s.busy = true
	s.actionErr = ""
	repo := a.repo
	a.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		var (
			out string
			err error
		)
		if drop {
			out, err = repo.StashDrop(ctx, ref)
		} else {
			out, err = repo.StashPop(ctx, ref)
		}
		cancel()
		a.update(func() {
			s.busy = false
			verb := "Pop"
			if drop {
				verb = "Drop"
			}
			if err != nil {
				s.actionErr = verb + " 失败：" + err.Error()
				a.setNote("")
			} else {
				s.actionErr = ""
				note := verb + " 成功：" + ref
				if out != "" {
					note += "（" + firstLine(out) + "）"
				}
				a.setNote(note)
			}
			// Do not keep previewing the entry that pop or drop removed.
			s.sel = -1
			s.preview = false
			s.resetDiff()
			// The status refresh reloads the stash list in the stash tab.
			a.refreshStatus()
		})
	})
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// stashDropDialog confirms dropping the stash chosen by the context menu.
func (a *root) stashDropDialog(c *ui.Context) {
	s := &a.stash
	if !s.dropAsk {
		s.dropRef = ""
		return
	}
	msg := fmt.Sprintf("将删除 %s。\n\n这个 Stash 会被移除，无法恢复。", s.dropRef)
	switch ui.AlertDialog(c, &s.dropAsk, "删除 Stash？", msg, "取消", "删除") {
	case 0, ui.AlertDismissed:
		s.dropRef = ""
	case 1:
		ref := s.dropRef
		s.dropRef = ""
		a.dropStash(ref)
	}
}
