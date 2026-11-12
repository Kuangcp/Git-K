package gitx

import (
	"context"
	"errors"
)

// RevertFiles discards the working tree changes of paths, restoring them
// from the index. The index is left alone, so a file that also has staged
// changes keeps them.
func (r *Repo) RevertFiles(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return errors.New("没有要撤销的路径")
	}
	args := []string{"restore", "--"}
	args = append(args, paths...)
	return r.run(ctx, nil, args...).Err2()
}
