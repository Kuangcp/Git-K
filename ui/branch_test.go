package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"git-k/app/gitx"
)

// The branch tab loads the graph, shows the first commit's files, and
// renders a file's diff with the shared diff body.
func TestBranchPane(t *testing.T) {
	dir := newRepo(t)
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "first")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRoot(dir)
	defer r.stopWatch()
	tt := ui.NewTester(r.View, 1200, 800)
	tt.Frame()
	for _, want := range []string{"工作区", "分支"} {
		if !tt.HasText(want) {
			t.Fatalf("top bar missing %q: %v", want, tt.Texts())
		}
	}

	tt.Key(ui.Ctrl, ui.Key2)
	tt.Frame()
	if r.pane != paneBranches {
		t.Fatal("Ctrl+2 did not open the branch pane")
	}
	if len(r.branch.commits) != 1 || r.branch.commits[0].Subject != "first" {
		t.Fatalf("commits: %+v", r.branch.commits)
	}
	if r.branch.main != "main" || !containsString(r.branch.branches, "main") {
		t.Fatalf("main line: %q branches=%v", r.branch.main, r.branch.branches)
	}
	if len(r.branch.files) != 1 || r.branch.files[0].path != "a.txt" {
		t.Fatalf("files: %+v", r.branch.files)
	}
	for _, want := range []string{"提交图", "first", "a.txt", "修改的文件 1"} {
		if !tt.HasText(want) {
			t.Fatalf("branch pane missing %q: %v", want, tt.Texts())
		}
	}
	if r.branch.fileSel != -1 || len(r.branch.groupFiles) != 1 || r.branch.groupAdded == 0 {
		t.Fatalf("default stacked diff: sel=%d files=%d added=%d",
			r.branch.fileSel, len(r.branch.groupFiles), r.branch.groupAdded)
	}
	if tt.HasText("全部暂存") {
		t.Fatalf("workspace staging action shown in branch pane: %v", tt.Texts())
	}

	r.selectCommitFile(0)
	tt.Frame()
	if r.branch.fileSel != 0 || r.branch.mode != diffPatch || r.branch.diffAdded == 0 {
		t.Fatalf("commit diff: sel=%d mode=%v added=%d",
			r.branch.fileSel, r.branch.mode, r.branch.diffAdded)
	}
	r.selectCommitGroup()
	tt.Frame()
	if r.branch.fileSel != -1 || len(r.branch.groupFiles) != 1 {
		t.Fatalf("back to stacked diff: sel=%d files=%d",
			r.branch.fileSel, len(r.branch.groupFiles))
	}

	tt.Key(ui.Ctrl, ui.Key1)
	tt.Frame()
	if r.pane != paneWorkspace || !tt.HasText("提交摘要") {
		t.Fatalf("Ctrl+1 did not return to the workspace")
	}
}

// layoutGraph puts a merge's parents in lanes below the merge.
func TestLayoutGraph(t *testing.T) {
	commits := []gitx.Commit{
		{Hash: "c3", Parents: []string{"c1", "c2"}},
		{Hash: "c2", Parents: []string{"c1"}},
		{Hash: "c1"},
	}
	rows := layoutGraph(commits)
	if len(rows) != 3 {
		t.Fatalf("rows: %d", len(rows))
	}
	if rows[0].col != 0 || len(rows[0].after) != 2 {
		t.Fatalf("merge row: %+v", rows[0])
	}
	if len(rows[0].edges) != 2 || rows[0].edges[0] != 0 || rows[0].edges[1] != 1 {
		t.Fatalf("merge edges: %+v", rows[0].edges)
	}
	if len(rows[0].afterNew) != 2 || rows[0].afterNew[0] || !rows[0].afterNew[1] {
		t.Fatalf("merge new lanes: %+v", rows[0].afterNew)
	}
	if !rows[0].merge || rows[1].merge {
		t.Fatalf("merge flags: %v %v", rows[0].merge, rows[1].merge)
	}
	if rows[1].afterNew[0] {
		t.Fatalf("existing lane marked new: %+v", rows[1].afterNew)
	}
	if len(rows[2].after) != 0 {
		t.Fatalf("root should close its lane: %+v", rows[2])
	}
}

// Refs split into chips, with HEAD's branch marked as current.
func TestParseRefs(t *testing.T) {
	chips := parseRefs("HEAD -> main, tag: v1, dev")
	if len(chips) != 3 {
		t.Fatalf("chips: %+v", chips)
	}
	if chips[0].name != "main" || !chips[0].current {
		t.Fatalf("head chip: %+v", chips[0])
	}
	if chips[1].name != "v1" || !chips[1].tag {
		t.Fatalf("tag chip: %+v", chips[1])
	}
	if chips[2].name != "dev" || chips[2].current || chips[2].tag {
		t.Fatalf("branch chip: %+v", chips[2])
	}
}
