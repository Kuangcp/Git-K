package gitx

import (
	"context"
	"errors"
)

// RemoveFile deletes an untracked file from the working tree, as its
// context menu does. It goes through git clean, which refuses paths git
// tracks or ignores: a stale status cannot delete a file git manages.
func (r *Repo) RemoveFile(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("路径为空")
	}
	res := r.run(ctx, nil, "clean", "-q", "-f", "--", path)
	return res.Err2()
}
