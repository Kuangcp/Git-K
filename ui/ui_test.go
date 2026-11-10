package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"git-k/app/gitx"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The three panes show up, the untracked file is diffed, staging moves it
// to the staged group, and the commit button creates a commit.
func TestStatusDiffCommit(t *testing.T) {
	dir := newRepo(t)
	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()

	for _, want := range []string{"状态", "提交摘要", "a.txt", "未跟踪"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	if r.diffMode != diffPatch || r.diffAdded == 0 {
		t.Fatalf("diff not loaded: mode=%v added=%d err=%q", r.diffMode, r.diffAdded, r.diffErr)
	}

	r.stageSelected()
	tt.Frame()
	if !tt.HasText("已暂存 1") {
		t.Fatalf("staged count missing: %v", tt.Texts())
	}
	if r.status.Count(gitx.Staged) != 1 {
		t.Fatalf("status after stage: %+v", r.status.Rows)
	}

	r.summary = "test: add a.txt"
	r.body.Reset("body")
	tt.Frame()
	if err := tt.Click("提交"); err != nil {
		t.Fatalf("click commit: %v", err)
	}
	tt.Frame()
	if r.commitErr != "" {
		t.Fatalf("commit error: %s", r.commitErr)
	}
	if r.status.Total() != 0 {
		t.Fatalf("status after commit: %+v", r.status.Rows)
	}
	tt.Frame()
	if r.diffMode != diffNone || r.diffPath != "" {
		t.Fatalf("stale diff after commit: mode=%v path=%q", r.diffMode, r.diffPath)
	}
	if got := git(t, dir, "log", "-1", "--format=%B"); got != "test: add a.txt\n\nbody\n\n" && got != "test: add a.txt\n\nbody\n" {
		t.Fatalf("commit message: %q", got)
	}
}

// A second click on an already chosen row does not reload the same diff.
func TestDiffCacheKey(t *testing.T) {
	dir := newRepo(t)
	r := NewRoot(dir)
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()
	key := r.diffKey
	tt.Frame()
	if r.diffKey != key {
		t.Fatalf("diff key changed without a status change: %q -> %q", key, r.diffKey)
	}
}

// Editing the selected file again must reload its patch even though the
// index did not change.
func TestDiffReloadsOnWorktreeEdit(t *testing.T) {
	dir := newRepo(t)
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRoot(dir)
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()
	if r.diffAdded != 1 {
		t.Fatalf("first diff: added=%d err=%q", r.diffAdded, r.diffErr)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\none\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.refreshStatus()
	tt.Frame()
	if r.diffAdded != 2 {
		t.Fatalf("diff did not reload: added=%d err=%q", r.diffAdded, r.diffErr)
	}
}

// Clicking a row chooses it and loads that file's diff.
func TestClickRowSelectsDiff(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRoot(dir)
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()
	if r.diffPath != "a.txt" {
		t.Fatalf("initial diff path: %q", r.diffPath)
	}
	if err := tt.Click("b.txt"); err != nil {
		t.Fatalf("click b.txt: %v", err)
	}
	tt.Frame()
	if r.diffPath != "b.txt" {
		t.Fatalf("diff path after click: %q", r.diffPath)
	}
	if r.diffAdded != 1 || r.diffMode != diffPatch {
		t.Fatalf("b.txt diff: mode=%v added=%d err=%q", r.diffMode, r.diffAdded, r.diffErr)
	}
}

// Double-clicking an unstaged file stages it, and double-clicking it in
// the staged group unstages it.
func TestDoubleClickTogglesStage(t *testing.T) {
	dir := newRepo(t)
	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()

	double := func(code string) {
		t.Helper()
		rect, ok := tt.Find(code)
		if !ok {
			t.Fatalf("status code %q missing: %v", code, tt.Texts())
		}
		x, y := rect.X+rect.W/2, rect.Y+rect.H/2
		tt.ClickAt(x, y)
		tt.ClickAt(x, y)
		tt.Frame()
	}

	double("?")
	if got := r.status.Count(gitx.Staged); got != 1 {
		t.Fatalf("after double click: staged=%d rows=%+v", got, r.status.Rows)
	}
	if !tt.HasText("已暂存") {
		t.Fatalf("staged group missing: %v", tt.Texts())
	}

	double("A")
	if got := r.status.Count(gitx.Staged); got != 0 {
		t.Fatalf("after second double click: staged=%d rows=%+v", got, r.status.Rows)
	}
	if got := r.status.Count(gitx.Modified) + r.status.Count(gitx.Untracked); got != 1 {
		t.Fatalf("file did not return unstaged: rows=%+v", r.status.Rows)
	}
}

// A staged modification or rename reads green like a staged addition;
// the same M in the working tree keeps the warning color.
func TestStagedCodeColors(t *testing.T) {
	th := ui.LightTheme()
	if got := codeColor(th, gitx.Staged, 'M'); got != th.Success {
		t.Fatalf("staged M color: %v, want success %v", got, th.Success)
	}
	if got := codeColor(th, gitx.Staged, 'R'); got != th.Success {
		t.Fatalf("staged R color: %v, want success %v", got, th.Success)
	}
	if got := codeColor(th, gitx.Modified, 'M'); got != th.Warning {
		t.Fatalf("modified M color: %v, want warning %v", got, th.Warning)
	}
	if got := codeColor(th, gitx.Staged, 'D'); got != th.Danger {
		t.Fatalf("staged D color: %v, want danger %v", got, th.Danger)
	}
}

// The context menu of an untracked file deletes it only after the alert
// is answered, and canceling leaves the file alone.
func TestDeleteUntrackedFileAsksFirst(t *testing.T) {
	dir := newRepo(t)
	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()

	ask := func() {
		t.Helper()
		rect, ok := tt.Find("?")
		if !ok {
			t.Fatalf("untracked code missing: %v", tt.Texts())
		}
		tt.RightClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
		if err := tt.ChooseMenuItem("删除文件…"); err != nil {
			t.Fatalf("choose delete: %v", err)
		}
		tt.Frame()
		if !tt.HasText("删除文件？") {
			t.Fatalf("confirmation missing: %v", tt.Texts())
		}
	}

	ask()
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("click cancel: %v", err)
	}
	tt.Frame()
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatalf("cancel removed the file: %v", err)
	}
	if got := r.status.Total(); got != 1 {
		t.Fatalf("after cancel: %d changes, want 1", got)
	}

	ask()
	if err := tt.Click("删除"); err != nil {
		t.Fatalf("click delete: %v", err)
	}
	tt.Frame()
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("file still exists after delete: %v", err)
	}
	if got := r.status.Total(); got != 0 {
		t.Fatalf("after delete: %d changes, want 0", got)
	}
}

// A context menu action applies to every chosen row when the row clicked
// is one of them.
func TestContextMenuStagesSelection(t *testing.T) {
	dir := newRepo(t)
	for _, name := range []string{"b.txt", "z.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()

	first, ok := tt.Find("?")
	if !ok {
		t.Fatalf("untracked code missing: %v", tt.Texts())
	}
	z, ok := tt.Find("z.txt")
	if !ok {
		t.Fatalf("z.txt missing: %v", tt.Texts())
	}
	tt.ClickAt(first.X+first.W/2, first.Y+first.H/2)
	if err := tt.ClickWith(ui.Shift, "z.txt"); err != nil {
		t.Fatalf("extend selection: %v", err)
	}
	tt.RightClickAt(z.X+z.W/2, z.Y+z.H/2)
	if err := tt.ChooseMenuItem("暂存"); err != nil {
		t.Fatalf("choose stage: %v", err)
	}
	tt.Frame()
	if got := r.status.Count(gitx.Staged); got != 3 {
		t.Fatalf("staged %d rows, want 3: %+v", got, r.status.Rows)
	}
	if got := r.status.Count(gitx.Untracked); got != 0 {
		t.Fatalf("untracked %d rows, want 0", got)
	}
}

func TestContextMenuDeletesSelection(t *testing.T) {
	dir := newRepo(t)
	for _, name := range []string{"b.txt", "z.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()

	first, ok := tt.Find("?")
	if !ok {
		t.Fatalf("untracked code missing: %v", tt.Texts())
	}
	z, ok := tt.Find("z.txt")
	if !ok {
		t.Fatalf("z.txt missing: %v", tt.Texts())
	}
	tt.ClickAt(first.X+first.W/2, first.Y+first.H/2)
	if err := tt.ClickWith(ui.Shift, "z.txt"); err != nil {
		t.Fatalf("extend selection: %v", err)
	}
	tt.RightClickAt(z.X+z.W/2, z.Y+z.H/2)
	if err := tt.ChooseMenuItem("删除 3 个文件…"); err != nil {
		t.Fatalf("choose delete: %v", err)
	}
	tt.Frame()
	if !tt.HasText("3 个未跟踪的文件") {
		t.Fatalf("confirmation missing the count: %v", tt.Texts())
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatalf("click delete: %v", err)
	}
	tt.Frame()
	for _, name := range []string{"a.txt", "b.txt", "z.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", name, err)
		}
	}
	if got := r.status.Total(); got != 0 {
		t.Fatalf("after delete: %d changes, want 0", got)
	}
}

// Clicking the group bar shows every file of the group, and clicking
// the active chip, or a file row, returns to one file.
func TestGroupHeaderShowsAllDiffs(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()
	bRow, ok := tt.Find("b.txt")
	if !ok {
		t.Fatalf("b.txt missing: %v", tt.Texts())
	}

	if err := tt.Click("未跟踪 2"); err != nil {
		t.Fatalf("click group chip: %v", err)
	}
	tt.Frame()
	if len(r.groupDiffs) != 2 {
		t.Fatalf("group rows: %d, want 2", len(r.groupDiffs))
	}
	if r.cur != -1 || r.chosen.Len() != 0 {
		t.Fatalf("file selection kept: cur=%d chosen=%d", r.cur, r.chosen.Len())
	}
	if len(r.groupFiles) != 2 {
		t.Fatalf("group files: %d, want 2", len(r.groupFiles))
	}
	for i, want := range []string{"a.txt", "b.txt"} {
		f := r.groupFiles[i]
		if f.path != want || f.mode != diffPatch {
			t.Fatalf("group file %d: path=%q mode=%v", i, f.path, f.mode)
		}
	}
	stat := r.groupStat.String()
	for _, want := range []string{"a.txt", "b.txt", "2 files changed", "2 insertions(+)"} {
		if !strings.Contains(stat, want) {
			t.Fatalf("group stat missing %q:\n%s", want, stat)
		}
	}
	if !tt.HasText("2 个文件") {
		t.Fatalf("group count missing: %v", tt.Texts())
	}

	// A status refresh keeps the group chosen.
	r.refreshStatus()
	tt.Frame()
	if len(r.groupDiffs) != 2 || len(r.groupFiles) != 2 {
		t.Fatalf("group lost after refresh: rows=%d files=%d", len(r.groupDiffs), len(r.groupFiles))
	}

	// Click the group row again to deselect
	if err := tt.Click("未跟踪 2"); err != nil {
		t.Fatalf("click group chip again: %v", err)
	}
	tt.Frame()
	if len(r.groupDiffs) != 0 || r.diffPath != "a.txt" || r.diffMode != diffPatch {
		t.Fatalf("after closing group: rows=%d path=%q mode=%v", len(r.groupDiffs), r.diffPath, r.diffMode)
	}

	// Reopen group and click a different file row to leave it
	if err := tt.Click("未跟踪 2"); err != nil {
		t.Fatalf("click group chip: %v", err)
	}
	tt.Frame()
	tt.ClickAt(bRow.X+bRow.W/2, bRow.Y+bRow.H/2)
	tt.Frame()
	if len(r.groupDiffs) != 0 || r.diffPath != "b.txt" {
		t.Fatalf("after clicking file: rows=%d path=%q", len(r.groupDiffs), r.diffPath)
	}
}

// Ctrl+Q asks the application to quit.
func TestQuitShortcut(t *testing.T) {
	dir := newRepo(t)
	r := NewRoot(dir)
	defer r.stopWatch()
	quit := false
	r.quit = func() { quit = true }
	tt := ui.NewTester(r.View, 1100, 800)
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyQ)
	if !quit {
		t.Fatal("Ctrl+Q did not quit")
	}
}

func TestMessageComposition(t *testing.T) {
	a := &root{summary: "feat: summary ", body: nil}
	a.body = editor.NewBuffer("line one\nline two\n")
	if got, want := a.message(), "feat: summary\n\nline one\nline two\n"; got != want {
		t.Fatalf("message: %q, want %q", got, want)
	}
	a.body.Reset("")
	if got := a.message(); got != "feat: summary" {
		t.Fatalf("summary only: %q", got)
	}
	sum, body := splitMessage("feat: summary\n\nline one\nline two\n")
	if sum != "feat: summary" || body != "line one\nline two" {
		t.Fatalf("split: %q / %q", sum, body)
	}
	sum, body = splitMessage("only summary\n")
	if sum != "only summary" || body != "" {
		t.Fatalf("split one line: %q / %q", sum, body)
	}
}

// git-cola's arrangement: commit above diff on the left, status on the right.
func TestLayoutLeftColumn(t *testing.T) {
	dir := newRepo(t)
	r := NewRoot(dir)
	tt := ui.NewTester(r.View, 1200, 800)
	tt.Frame()
	summary, ok := tt.Find("提交摘要")
	if !ok {
		t.Fatalf("summary field missing: %v", tt.Texts())
	}
	status, ok := tt.Find("状态")
	if !ok {
		t.Fatalf("status title missing: %v", tt.Texts())
	}
	diff, ok := tt.Find("差异")
	if !ok {
		t.Fatalf("diff title missing: %v", tt.Texts())
	}
	if summary.X >= status.X {
		t.Fatalf("commit must be left of status: summary=%v status=%v", summary, status)
	}
	if diff.Y <= summary.Y {
		t.Fatalf("diff must be below commit: summary=%v diff=%v", summary, diff)
	}
	if diff.X >= status.X {
		t.Fatalf("diff must be left of status: diff=%v status=%v", diff, status)
	}
}
