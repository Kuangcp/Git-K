package gitx

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Log reads HEAD and local branches, and commit details come from git.
func TestHistory(t *testing.T) {
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "first")
	write(t, dir, "a.txt", "two\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "second")

	ctx := context.Background()
	commits, err := repo.Log(ctx, 10, AllBranches)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("commits: %d", len(commits))
	}
	if commits[0].Subject != "second" || commits[1].Subject != "first" {
		t.Fatalf("order: %q %q", commits[0].Subject, commits[1].Subject)
	}
	if len(commits[0].Parents) != 1 || commits[0].Parents[0] != commits[1].Hash {
		t.Fatalf("parents: %+v", commits[0].Parents)
	}
	if commits[0].Author != "Test" || commits[0].Time.IsZero() {
		t.Fatalf("author/time: %+v", commits[0])
	}
	if !strings.Contains(commits[0].Refs, "main") {
		t.Fatalf("refs: %q", commits[0].Refs)
	}

	msg, err := repo.CommitText(ctx, commits[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "second" {
		t.Fatalf("message: %q", msg)
	}

	files, err := repo.CommitFiles(ctx, commits[0].Hash, commits[0].Parents[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "a.txt" || files[0].Code != 'M' {
		t.Fatalf("files: %+v", files)
	}

	patch, err := repo.CommitDiff(ctx, commits[0].Hash, commits[0].Parents[0], "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "+two") {
		t.Fatalf("patch: %q", patch)
	}
}

// A root commit diffs with git show, and an empty repository logs nothing.
func TestHistoryRootAndUnborn(t *testing.T) {
	repo, dir := newRepo(t)
	ctx := context.Background()
	empty, err := repo.Log(ctx, 10, AllBranches)
	if err != nil || len(empty) != 0 {
		t.Fatalf("unborn log: %v %v", empty, err)
	}
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "root")
	commits, err := repo.Log(ctx, 10, AllBranches)
	if err != nil || len(commits) != 1 {
		t.Fatalf("root log: %v %v", commits, err)
	}
	patch, err := repo.CommitDiff(ctx, commits[0].Hash, "", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "+one") {
		t.Fatalf("root patch: %q", patch)
	}
}

// A merge reads against its first parent, never a combined diff.
func TestHistoryMergeFirstParent(t *testing.T) {
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "base")

	git(t, dir, "checkout", "-q", "-b", "side")
	write(t, dir, "b.txt", "side\n")
	git(t, dir, "add", "b.txt")
	git(t, dir, "commit", "-q", "-m", "side")

	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "c.txt", "main\n")
	git(t, dir, "add", "c.txt")
	git(t, dir, "commit", "-q", "-m", "main work")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")

	ctx := context.Background()
	commits, err := repo.Log(ctx, 10, AllBranches)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits[0].Parents) != 2 {
		t.Fatalf("merge parents: %+v", commits[0].Parents)
	}
	files, err := repo.CommitFiles(ctx, commits[0].Hash, commits[0].Parents[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "b.txt" || files[0].Code != 'A' {
		t.Fatalf("merge files: %+v", files)
	}
	patch, err := repo.CommitDiff(ctx, commits[0].Hash, commits[0].Parents[0], "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "+side") {
		t.Fatalf("merge patch: %q", patch)
	}
}

// Log follows the chosen main line; ALL unions the local branches.
func TestHistoryBranchFilter(t *testing.T) {
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "checkout", "-q", "-b", "side")
	write(t, dir, "b.txt", "side\n")
	git(t, dir, "add", "b.txt")
	git(t, dir, "commit", "-q", "-m", "side work")
	git(t, dir, "checkout", "-q", "main")

	ctx := context.Background()
	branches, err := repo.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 || branches[0] != "main" || branches[1] != "side" {
		t.Fatalf("branches: %v", branches)
	}
	main, err := repo.Log(ctx, 10, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(main) != 1 || main[0].Subject != "base" {
		t.Fatalf("main log: %+v", main)
	}
	all, err := repo.Log(ctx, 10, AllBranches)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all log: %+v", all)
	}
}

// A merge in progress starts from git's MERGE_MSG, cleaned of comments,
// and committing it makes a merge commit.
func TestPendingMergeMessage(t *testing.T) {
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "base\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "checkout", "-q", "-b", "side")
	write(t, dir, "a.txt", "side\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "side")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "a.txt", "main\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "main")

	cmd := exec.Command("git", "merge", "side")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("merge succeeded without a conflict: %s", out)
	}
	msg := repo.PendingMessage()
	if !strings.Contains(msg, "Merge branch 'side'") {
		t.Fatalf("merge message: %q", msg)
	}
	if strings.Contains(msg, "# Conflicts") {
		t.Fatalf("comments kept: %q", msg)
	}
	write(t, dir, "a.txt", "resolved\n")
	git(t, dir, "add", "a.txt")
	if _, err := repo.Commit(context.Background(), msg, false); err != nil {
		t.Fatal(err)
	}
	commits, err := repo.Log(context.Background(), 10, AllBranches)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits[0].Parents) != 2 {
		t.Fatalf("commit is not a merge: %+v", commits[0].Parents)
	}
}
