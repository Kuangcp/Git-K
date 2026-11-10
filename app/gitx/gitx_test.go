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

// RemoveFiles deletes untracked files in one call, while git clean leaves
// a path in the index alone.
func TestRemoveFilesUntrackedOnly(t *testing.T) {
	ctx := context.Background()
	repo, dir := newRepo(t)
	write(t, dir, "loose.txt", "hello\n")
	write(t, dir, "other.txt", "hello\n")
	if err := repo.RemoveFiles(ctx, []string{"loose.txt", "other.txt"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"loose.txt", "other.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("untracked file %s still exists: %v", name, err)
		}
	}

	write(t, dir, "tracked.txt", "keep\n")
	git(t, dir, "add", "tracked.txt")
	if err := repo.RemoveFiles(ctx, []string{"tracked.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tracked.txt")); err != nil {
		t.Fatalf("tracked file removed: %v", err)
	}
}

// LoadAmend returns the last commit's message and resets the index to
// HEAD, leaving the working tree's changes in place.
// Push sends the current branch upstream.
func TestPush(t *testing.T) {
	ctx := context.Background()
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "hello\n")
	if err := repo.Stage(ctx, []string{"a.txt"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(ctx, "first", false); err != nil {
		t.Fatal(err)
	}
	bare := t.TempDir()
	git(t, bare, "init", "--bare", "-q")
	git(t, dir, "remote", "add", "origin", bare)
	git(t, dir, "push", "-q", "-u", "origin", "main")

	write(t, dir, "a.txt", "second\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-q", "-m", "second")
	if _, err := repo.Push(ctx); err != nil {
		t.Fatal(err)
	}
	if got := git(t, bare, "log", "-1", "--format=%s", "main"); strings.TrimSpace(got) != "second" {
		t.Fatalf("pushed commit: %q", got)
	}
}

func TestLoadAmend(t *testing.T) {
	ctx := context.Background()
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "hello\n")
	if err := repo.Stage(ctx, []string{"a.txt"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(ctx, "first\n\nbody\n", false); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "b.txt", "second\n")
	if err := repo.Stage(ctx, []string{"b.txt"}, false); err != nil {
		t.Fatal(err)
	}
	msg, err := repo.LoadAmend(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if msg != "first\n\nbody" {
		t.Fatalf("message: %q", msg)
	}
	st, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Count(Staged) != 0 || st.Count(Untracked) != 1 {
		t.Fatalf("index not reset: %+v", st.Rows)
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
