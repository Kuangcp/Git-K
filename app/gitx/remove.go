package gitx

import (
	"context"
	"errors"
)

// RemoveFiles deletes untracked files from the working tree, as their
// context menu does, in one git process. It goes through git clean, which
// removes only paths git neither tracks nor ignores, so a stale status
// cannot delete a file git manages.
func (r *Repo) RemoveFiles(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return errors.New("没有要删除的路径")
	}
	args := []string{"clean", "-q", "-f", "--"}
	args = append(args, paths...)
	return r.run(ctx, nil, args...).Err2()
}
