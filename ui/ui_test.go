package ui

import (
	"os"
	"os/exec"
	"path/filepath"
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
