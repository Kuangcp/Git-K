package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func newRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "user.email", "test@example.com")
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo, dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusStageDiffCommit(t *testing.T) {
	ctx := context.Background()
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "hello\n")

	st, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Rows) != 1 || st.Rows[0].Group != Untracked || st.Rows[0].Path != "a.txt" {
		t.Fatalf("initial status: %+v", st.Rows)
	}
	if !repo.Unborn(ctx) {
		t.Fatal("expected unborn HEAD")
	}
	patch, path, err := repo.Diff(ctx, st.Rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if path != "a.txt" || !strings.Contains(patch, "+hello") {
		t.Fatalf("untracked diff:\n%s", patch)
	}

	// Unstaging on an unborn branch removes the entry from the index.
	git(t, dir, "add", "a.txt")
	if err := repo.Unstage(ctx, []string{"a.txt"}, true); err != nil {
		t.Fatal(err)
	}
	st, _ = repo.Status(ctx)
	if len(st.Rows) != 1 || st.Rows[0].Group != Untracked {
		t.Fatalf("after unborn unstage: %+v", st.Rows)
	}

	if err := repo.Stage(ctx, []string{"a.txt"}, true); err != nil {
		t.Fatal(err)
	}
	st, _ = repo.Status(ctx)
	if !st.HasStaged() || st.Rows[0].Group != Staged {
		t.Fatalf("after stage: %+v", st.Rows)
	}
	patch, _, err = repo.Diff(ctx, st.Rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "new file mode") {
		t.Fatalf("staged diff:\n%s", patch)
	}

	if _, err := repo.Commit(ctx, "test: first commit\n", false); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "a.txt", "hello\nworld\n")
	st, _ = repo.Status(ctx)
	if len(st.Rows) != 1 || st.Rows[0].Group != Modified {
		t.Fatalf("after commit: %+v", st.Rows)
	}
	patch, _, err = repo.Diff(ctx, st.Rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "+world") {
		t.Fatalf("modified diff:\n%s", patch)
	}

	if _, err := repo.Commit(ctx, "test: amend\n", true); err != nil {
		t.Fatal(err)
	}
	log := git(t, dir, "log", "-1", "--format=%B")
	if !strings.Contains(log, "test: amend") {
		t.Fatalf("amend did not rewrite the message: %q", log)
	}
}

func TestDiffEscapedPathIsLiteral(t *testing.T) {
	ctx := context.Background()
	repo, dir := newRepo(t)
	write(t, dir, "*.txt", "literal\n")
	if err := repo.Stage(ctx, []string{"*.txt"}, true); err != nil {
		t.Fatal(err)
	}
	st, _ := repo.Status(ctx)
	patch, _, err := repo.Diff(ctx, st.Rows[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "literal") {
		t.Fatalf("literal pathspec failed:\n%s", patch)
	}
}

func TestDraftRoundTrip(t *testing.T) {
	repo, _ := newRepo(t)
	if got := repo.CommitMessage(); got != "" {
		t.Fatalf("empty draft: %q", got)
	}
	if err := repo.SaveCommitMessage("draft message"); err != nil {
		t.Fatal(err)
	}
	if got := repo.CommitMessage(); got != "draft message" {
		t.Fatalf("draft: %q", got)
	}
	if err := repo.SaveCommitMessage(""); err != nil {
		t.Fatal(err)
	}
	if got := repo.CommitMessage(); got != "" {
		t.Fatalf("cleared draft: %q", got)
	}
}
