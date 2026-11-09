package gitx

import (
	"context"
	"errors"
	"os"
	"strings"
)

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
