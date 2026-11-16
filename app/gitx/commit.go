package gitx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// LoadAmend returns the last commit's message and resets the index to
// HEAD, leaving the working tree alone: amending then starts from the
// last commit's tree plus what the user stages.
func (r *Repo) LoadAmend(ctx context.Context) (string, error) {
	res := r.run(ctx, nil, "log", "-1", "--format=%B", "HEAD")
	if err := res.Err2(); err != nil {
		return "", err
	}
	msg := strings.TrimRight(string(res.Stdout), "\n")
	res = r.run(ctx, nil, "reset", "-q", "--mixed", "HEAD")
	if err := res.Err2(); err != nil {
		return "", err
	}
	return msg, nil
}

// PendingMessage is the message to start editing: git's merge message
// while a merge is in progress, unless this app saved a newer draft (the
// user edited it), else the app's draft.
func (r *Repo) PendingMessage() string {
	draft := r.CommitMessage()
	if _, err := os.Stat(filepath.Join(r.GitDir, "MERGE_HEAD")); err != nil {
		return draft
	}
	path := filepath.Join(r.GitDir, "MERGE_MSG")
	info, err := os.Stat(path)
	if err != nil {
		return draft
	}
	if draft != "" {
		if di, err := os.Stat(r.DraftPath); err == nil && di.ModTime().After(info.ModTime()) {
			return draft
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return draft
	}
	return cleanMergeMessage(string(b))
}

// cleanMergeMessage drops git's commented instructions ("# Conflicts:")
// from a merge message, as git's default cleanup does for an editor.
func cleanMergeMessage(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\n")
}

// Commit creates a commit from the index with msg. amend rewrites the
// last commit. It returns git's output, which names the new commit.
func (r *Repo) Commit(ctx context.Context, msg string, amend bool) (string, error) {
	if strings.TrimSpace(msg) == "" {
		return "", errors.New("提交信息不能为空")
	}
	args := []string{"commit", "--quiet", "-F", "-"}
	if amend {
		args = append(args, "--amend")
	}
	res := r.run(ctx, []byte(msg), args...)
	out := strings.TrimSpace(string(res.Stdout))
	if err := res.Err2(); err != nil {
		detail := strings.TrimSpace(string(res.Stderr))
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

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writeFile(path, s string) error {
	return os.WriteFile(path, []byte(s), 0o600)
}

func removeFile(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
