package gitx

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Stash is one entry of git stash list.
type Stash struct {
	// Ref is "stash@{0}", the name git accepts for pop and drop.
	Ref string
	// Hash is the stash commit.
	Hash string
	// Base is the commit the stash was made on: the stash commit's first
	// parent, which is what the UI shows as its commit.
	Base string
	// When is when the stash was made.
	When time.Time
	// Message is git's subject, such as
	// "WIP on main: 1a2b3c4 the last commit".
	Message string
}

// Stashes lists the stashes, newest first.
func (r *Repo) Stashes(ctx context.Context) ([]Stash, error) {
	res := r.run(ctx, nil,
		"-c", "color.ui=false",
		"stash", "list",
		"--format=%gd%x00%H%x00%P%x00%ct%x00%s")
	if err := res.Err2(); err != nil {
		return nil, err
	}
	out := strings.TrimRight(string(res.Stdout), "\n")
	if out == "" {
		return nil, nil
	}
	var stashes []Stash
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\x00", 5)
		if len(f) != 5 || f[0] == "" {
			continue
		}
		st := Stash{Ref: f[0], Hash: f[1], Message: f[4]}
		if parents := strings.Fields(f[2]); len(parents) > 0 {
			st.Base = parents[0]
		}
		if sec, err := strconv.ParseInt(f[3], 10, 64); err == nil {
			st.When = time.Unix(sec, 0)
		}
		stashes = append(stashes, st)
	}
	return stashes, nil
}

// StashDiff returns the patch a stash applies to its base, including the
// untracked files of a stash made with -u.
func (r *Repo) StashDiff(ctx context.Context, ref string) (string, error) {
	args := []string{"-c", "color.ui=false", "stash", "show", "-p", "--no-color", "--include-untracked", ref}
	res := r.run(ctx, nil, args...)
	if err := res.Err2(); err != nil {
		// Old git before --include-untracked: show the tracked files.
		if strings.Contains(strings.ToLower(string(res.Stderr)), "unknown option") ||
			strings.Contains(strings.ToLower(string(res.Stderr)), "unrecognized option") {
			res = r.run(ctx, nil, "-c", "color.ui=false", "stash", "show", "-p", "--no-color", ref)
		}
	}
	if err := res.Err2(); err != nil {
		return "", err
	}
	return string(res.Stdout), nil
}

// StashPop applies the stash and drops it when the application succeeds,
// as git stash pop does.
func (r *Repo) StashPop(ctx context.Context, ref string) (string, error) {
	return r.stashAction(ctx, "pop", ref)
}

// StashDrop removes the stash without applying it.
func (r *Repo) StashDrop(ctx context.Context, ref string) (string, error) {
	return r.stashAction(ctx, "drop", ref)
}

func (r *Repo) stashAction(ctx context.Context, action, ref string) (string, error) {
	if !strings.HasPrefix(ref, "stash@{") || !strings.HasSuffix(ref, "}") {
		return "", errors.New("无效的 Stash 引用：" + ref)
	}
	res := r.run(ctx, nil, "stash", action, ref)
	out := strings.TrimSpace(string(res.Stdout))
	if err := res.Err2(); err != nil {
		detail := trimmed(res.Stderr)
		if detail == "" {
			detail = out
		}
		if detail == "" {
			detail = err.Error()
		}
		return out, errors.New(detail)
	}
	return out, nil
}
