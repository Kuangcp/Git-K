package gitx

import (
	"context"
	"strings"
	"testing"
)

func TestStashListDiffPopDrop(t *testing.T) {
	ctx := context.Background()
	repo, dir := newRepo(t)
	write(t, dir, "a.txt", "base\n")
	if err := repo.Stage(ctx, []string{"a.txt"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(ctx, "base commit\n", false); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(git(t, dir, "rev-parse", "HEAD"))

	write(t, dir, "a.txt", "base\nworktree\n")
	write(t, dir, "new.txt", "untracked\n")
	git(t, dir, "stash", "push", "-u", "-m", "test stash")

	stashes, err := repo.Stashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stashes) != 1 {
		t.Fatalf("stashes: %+v", stashes)
	}
	st := stashes[0]
	if st.Ref != "stash@{0}" || st.Hash == "" || st.Base != base {
		t.Fatalf("stash entry: %+v, base=%s", st, base)
	}
	if !strings.Contains(st.Message, "test stash") {
		t.Fatalf("message: %q", st.Message)
	}
	if st.When.IsZero() {
		t.Fatal("stash time missing")
	}

	patch, err := repo.StashDiff(ctx, st.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "+worktree") || !strings.Contains(patch, "new.txt") ||
		!strings.Contains(patch, "+untracked") {
		t.Fatalf("stash diff missing changes:\n%s", patch)
	}

	out, err := repo.StashPop(ctx, st.Ref)
	if err != nil {
		t.Fatalf("pop: %v (%s)", err, out)
	}
	stashes, _ = repo.Stashes(ctx)
	if len(stashes) != 0 {
		t.Fatalf("stash remained after pop: %+v", stashes)
	}
	status, _ := repo.Status(ctx)
	if len(status.Rows) != 2 {
		t.Fatalf("worktree after pop: %+v", status.Rows)
	}

	// Stash again and drop it; the worktree is untouched.
	git(t, dir, "stash", "push", "-u", "-m", "drop me")
	stashes, _ = repo.Stashes(ctx)
	if len(stashes) != 1 {
		t.Fatalf("second stash: %+v", stashes)
	}
	ref := stashes[0].Ref
	if _, err := repo.StashDrop(ctx, ref); err != nil {
		t.Fatal(err)
	}
	stashes, _ = repo.Stashes(ctx)
	if len(stashes) != 0 {
		t.Fatalf("stash remained after drop: %+v", stashes)
	}
	status, _ = repo.Status(ctx)
	if len(status.Rows) != 0 {
		t.Fatalf("drop must not touch the worktree: %+v", status.Rows)
	}
}

func TestStashActionRejectsBadRef(t *testing.T) {
	repo, _ := newRepo(t)
	if _, err := repo.StashPop(context.Background(), "HEAD"); err == nil {
		t.Fatal("expected an invalid stash ref error")
	}
}
