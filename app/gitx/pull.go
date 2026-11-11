package gitx

import (
	"context"
	"errors"
	"strings"
)

// Pull brings the current branch's upstream changes into the working
// tree and returns git's output.
func (r *Repo) Pull(ctx context.Context) (string, error) {
	res := r.run(ctx, nil, "pull")
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
