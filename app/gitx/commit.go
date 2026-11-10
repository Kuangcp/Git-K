package gitx

import (
	"context"
	"errors"
	"os"
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
