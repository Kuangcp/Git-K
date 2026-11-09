package gitx

import (
	"context"
	"strings"
)

// stageArgs builds one git invocation for paths. Empty paths means all.
func stageArgs(paths []string, unstage, unborn bool) []string {
	if unstage {
		if unborn {
			args := []string{"rm", "-r", "--cached", "--ignore-unmatch"}
			if len(paths) == 0 {
				return append(args, ".")
			}
			return append(append(args, "--"), paths...)
		}
		args := []string{"restore", "--staged"}
		if len(paths) == 0 {
			return append(args, ".")
		}
		return append(append(args, "--"), paths...)
	}
	args := []string{"add", "-A"}
	if len(paths) == 0 {
		return args
	}
	return append(append(args, "--"), paths...)
}

// Stage adds paths (all when none) to the index.
func (r *Repo) Stage(ctx context.Context, paths []string, unborn bool) error {
	res := r.run(ctx, nil, stageArgs(paths, false, unborn)...)
	if err := res.Err2(); err != nil {
		return err
	}
	return nil
}

// Unstage restores paths (all when none) in the index from HEAD. An
// unborn branch has no HEAD, so the paths are removed from the index.
func (r *Repo) Unstage(ctx context.Context, paths []string, unborn bool) error {
	res := r.run(ctx, nil, stageArgs(paths, true, unborn)...)
	if err := res.Err2(); err != nil {
		return err
	}
	return nil
}

// Unborn reports whether HEAD does not exist yet.
func (r *Repo) Unborn(ctx context.Context) bool {
	res := r.run(ctx, nil, "rev-parse", "--verify", "--quiet", "HEAD")
	return res.ExitCode != 0
}

// CommitMessage returns the draft message saved for the repository.
func (r *Repo) CommitMessage() string {
	b, err := readFile(r.DraftPath)
	if err != nil {
		return ""
	}
	return b
}

// SaveCommitMessage writes the draft message; an empty message removes
// the draft.
func (r *Repo) SaveCommitMessage(msg string) error {
	if strings.TrimSpace(msg) == "" {
		return removeFile(r.DraftPath)
	}
	return writeFile(r.DraftPath, msg)
}
