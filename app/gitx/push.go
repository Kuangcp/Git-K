package gitx

import (
	"context"
	"errors"
	"strings"
)

// Push sends the current branch to its configured upstream and returns
// git's output.
func (r *Repo) Push(ctx context.Context) (string, error) {
	res := r.run(ctx, nil, "push")
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
