package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The Stash tab lists stashes with their base commit, previews the chosen
// stash, and Pop restores the worktree.
func TestStashPaneAndPop(t *testing.T) {
	dir := newRepo(t)
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nworktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "stash", "push", "-u", "-m", "test stash")
	// 一个只在工作区、不在 stash 里的文件，用来测试状态面板的点击。
	if err := os.WriteFile(filepath.Join(dir, "unstaged.txt"), []byte("now\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1200, 800)
	tt.Frame()

	tt.Key(ui.Ctrl, ui.Key3)
	tt.Frame()
	if r.pane != paneStashes {
		t.Fatal("Ctrl+3 did not open the Stash pane")
	}
	if len(r.stash.entries) != 1 || r.stash.entries[0].Ref != "stash@{0}" {
		t.Fatalf("stashes: %+v", r.stash.entries)
	}
	if r.stash.sel != 0 || !r.stash.preview {
		t.Fatalf("first stash must preview: sel=%d preview=%v", r.stash.sel, r.stash.preview)
	}
	if len(r.stash.files) == 0 || r.stash.diffAdded == 0 {
		t.Fatalf("stash diff missing: files=%d added=%d err=%q",
			len(r.stash.files), r.stash.diffAdded, r.stash.diffErr)
	}
	base := r.stash.entries[0].Base
	for _, want := range []string{"Stash", "stash@{0}", "test stash", shortHash(base)} {
		if !tt.HasText(want) {
			t.Fatalf("stash pane missing %q: %v", want, tt.Texts())
		}
	}

	// 右侧面板列出 stash 中的文件，而不是工作区状态。
	if !tt.HasText("a.txt") || !tt.HasText("new.txt") {
		t.Fatalf("stash files missing: %v", tt.Texts())
	}
	if tt.HasText("unstaged.txt") {
		t.Fatalf("workspace status shown in stash preview: %v", tt.Texts())
	}
	if r.stash.fileSel != 0 || r.stash.allFiles {
		t.Fatalf("first stash file should be chosen: sel=%d all=%v", r.stash.fileSel, r.stash.allFiles)
	}
	for _, want := range []string{"全部 2 个文件", "新增 1", "修改 1"} {
		if !tt.HasText(want) {
			t.Fatalf("stash summary missing %q: %v", want, tt.Texts())
		}
	}
	want := -1
	for i, f := range r.stash.files {
		if f.path == "new.txt" {
			want = i
		}
	}
	if want < 0 {
		t.Fatalf("new.txt missing from stash files: %+v", r.stash.files)
	}
	if err := tt.Click("new.txt"); err != nil {
		t.Fatalf("click stash file: %v", err)
	}
	tt.Frame()
	if r.stash.fileSel != want || r.stash.allFiles || !r.stash.preview {
		t.Fatalf("stash file choice: sel=%d want=%d all=%v preview=%v",
			r.stash.fileSel, want, r.stash.allFiles, r.stash.preview)
	}

	if err := tt.Click("全部 2 个文件"); err != nil {
		t.Fatalf("click stash summary: %v", err)
	}
	tt.Frame()
	if !r.stash.allFiles || r.stash.fileSel != -1 {
		t.Fatalf("aggregate stash diff: all=%v sel=%d", r.stash.allFiles, r.stash.fileSel)
	}

	// 差异头的工作区按钮返回工作区差异和状态。
	if err := tt.Click("工作区差异"); err != nil {
		t.Fatalf("click workspace diff: %v", err)
	}
	tt.Frame()
	if r.stash.preview {
		t.Fatal("workspace diff button must leave the stash preview")
	}
	if !tt.HasText("unstaged.txt") {
		t.Fatalf("workspace status not restored: %v", tt.Texts())
	}

	r.popStash("stash@{0}")
	tt.Frame()
	if len(r.stash.entries) != 0 {
		t.Fatalf("stash remained after pop: %+v", r.stash.entries)
	}
	if r.status.Total() != 3 {
		t.Fatalf("worktree after pop: %+v", r.status.Rows)
	}
	if r.stash.preview {
		t.Fatal("preview must clear when the stash list is empty")
	}
}

// Drop needs confirmation and removes the stash without changing files.
func TestStashDropConfirm(t *testing.T) {
	dir := newRepo(t)
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nstash me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "stash", "push", "-m", "to drop")

	r := NewRoot(dir)
	tt := ui.NewTester(r.View, 1200, 800)
	tt.Frame()
	r.setPane(paneStashes)
	tt.Frame()
	if len(r.stash.entries) != 1 {
		t.Fatalf("stashes: %+v", r.stash.entries)
	}
	r.stash.dropRef = "stash@{0}"
	r.stash.dropAsk = true
	tt.Frame()
	if !tt.HasText("删除 Stash？") {
		t.Fatalf("drop confirmation missing: %v", tt.Texts())
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatalf("cancel drop: %v", err)
	}
	tt.Frame()
	if len(r.stash.entries) != 1 || r.stash.dropAsk {
		t.Fatalf("cancel must keep the stash: %+v ask=%v", r.stash.entries, r.stash.dropAsk)
	}

	r.dropStash("stash@{0}")
	tt.Frame()
	if len(r.stash.entries) != 0 {
		t.Fatalf("stash remained after drop: %+v", r.stash.entries)
	}
	if r.status.Total() != 0 {
		t.Fatalf("drop must not touch the worktree: %+v", r.status.Rows)
	}
}

func TestStashSubject(t *testing.T) {
	tests := map[string]string{
		"WIP on main: 1a2b3c4 fix the thing": "fix the thing",
		"On main: my stash":                  "my stash",
		"unrelated message":                  "unrelated message",
	}
	for in, want := range tests {
		if got := stashSubject(in); got != want {
			t.Fatalf("stashSubject(%q) = %q, want %q", in, got, want)
		}
	}
}

// The row's context menu pops the stash it was opened on.
func TestStashContextMenuPop(t *testing.T) {
	dir := newRepo(t)
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nmenu pop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "stash", "push", "-m", "menu stash")

	r := NewRoot(dir)
	tt := ui.NewTester(r.View, 1200, 800)
	tt.Frame()
	r.setPane(paneStashes)
	tt.Frame()
	if err := tt.RightClick("menu stash"); err != nil {
		t.Fatalf("right click stash: %v", err)
	}
	tt.Frame()
	menu := tt.Menu()
	if len(menu) == 0 {
		t.Fatalf("stash context menu missing: %v", tt.Texts())
	}
	if err := tt.ChooseMenuItem("Pop"); err != nil {
		t.Fatalf("choose Pop from %v: %v", menu, err)
	}
	tt.Frame()
	if len(r.stash.entries) != 0 {
		t.Fatalf("stash remained after menu pop: %+v", r.stash.entries)
	}
	if r.status.Total() == 0 {
		t.Fatal("pop from the menu did not restore the worktree")
	}
}
