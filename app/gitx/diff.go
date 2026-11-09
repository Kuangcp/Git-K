package gitx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// MaxPatchSize is the largest patch git-k parses; bigger files show a
// notice instead of freezing the UI.
const MaxPatchSize = 8 << 20

// Diff returns the unified patch for row, exactly as git prints it, and
// the path it is for. An unchanged row is not an error: the patch is
// empty.
func (r *Repo) Diff(ctx context.Context, row Row) (patch string, path string, err error) {
	path = row.Path
	var res Result
	switch row.Group {
	case Staged:
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"diff", "--cached", "--no-ext-diff", "--no-color", "-M", "--", row.Path)
	case Modified:
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"diff", "--no-ext-diff", "--no-color", "-M", "--", row.Path)
	case Untracked:
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"diff", "--no-index", "--no-color", "--", os.DevNull, row.Path)
	default:
		// Combined diffs of a conflict use a syntax ParsePatch does not
		// read; the UI falls back to the raw text.
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"diff", "--no-ext-diff", "--no-color", "--cc", "--", row.Path)
	}
	// git diff exits 1 when there are differences, which is the normal
	// case.
	if res.Err != nil {
		return "", path, res.Err
	}
	if res.ExitCode != 0 && res.ExitCode != 1 {
		return "", path, &CommandError{Code: res.ExitCode, Stderr: trimmed(res.Stderr)}
	}
	if len(res.Stdout) > MaxPatchSize {
		return "", path, ErrPatchTooLarge
	}
	return string(res.Stdout), path, nil
}

// ErrPatchTooLarge is returned for a patch beyond MaxPatchSize.
var ErrPatchTooLarge = fmt.Errorf("补丁超过 %d MiB，已跳过渲染", MaxPatchSize>>20)

func trimmed(b []byte) string {
	s := string(b)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// FileStamp is the working tree modification stamp of row's path, 0 when
// it does not exist. It keys the diff cache so an edit that leaves the
// index alone still reloads the patch.
func (r *Repo) FileStamp(row Row) int64 {
	fi, err := os.Stat(filepath.Join(r.Root, filepath.FromSlash(row.Path)))
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}
