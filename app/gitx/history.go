package gitx

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Commit is one commit of the history graph.
type Commit struct {
	Hash    string
	Parents []string
	Author  string
	Time    time.Time
	// Refs is git's decoration, such as "HEAD -> main, tag: v1".
	Refs    string
	Subject string
}

// CommitFile is one file a commit changed.
type CommitFile struct {
	// Code is the name-status letter: A, M, D, R, C or T.
	Code    byte
	Path    string
	OldPath string
}

// AllBranches asks Log for every local branch instead of one main line.
const AllBranches = "ALL"

// Branches lists the local branch names.
func (r *Repo) Branches(ctx context.Context) ([]string, error) {
	res := r.run(ctx, nil,
		"-c", "color.ui=false",
		"for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err := res.Err2(); err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimRight(string(res.Stdout), "\n"), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// Log returns the recent commits reachable from branch, newest first, in
// topological order. An empty branch or AllBranches unions HEAD and every
// local branch, so the graph shows all lines of work.
func (r *Repo) Log(ctx context.Context, limit int, branch string) ([]Commit, error) {
	if limit <= 0 {
		limit = 300
	}
	// An unborn branch has no HEAD to log; an empty history is not an
	// error for the branch view.
	head := r.run(ctx, nil, "rev-parse", "--verify", "-q", "HEAD")
	if head.Err != nil {
		return nil, head.Err
	}
	if head.ExitCode != 0 {
		return nil, nil
	}
	revs := []string{"HEAD", "--branches"}
	if branch != "" && branch != AllBranches {
		revs = []string{"refs/heads/" + branch}
	}
	args := []string{
		"-c", "color.ui=false",
		"log", "--topo-order", "--max-count=" + strconv.Itoa(limit),
		"--decorate=short", "--decorate-refs=refs/heads",
		"--decorate-refs=refs/tags",
		"--format=%H%x00%P%x00%an%x00%at%x00%D%x00%s%x00",
	}
	args = append(args, revs...)
	res := r.run(ctx, nil, args...)
	if err := res.Err2(); err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(strings.TrimRight(string(res.Stdout), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) < 6 {
			continue
		}
		c := Commit{Hash: f[0], Author: f[2], Refs: f[4], Subject: f[5]}
		if f[1] != "" {
			c.Parents = strings.Fields(f[1])
		}
		if n, err := strconv.ParseInt(f[3], 10, 64); err == nil {
			c.Time = time.Unix(n, 0)
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// CommitText returns the full message of hash, without the trailing
// newline git adds.
func (r *Repo) CommitText(ctx context.Context, hash string) (string, error) {
	res := r.run(ctx, nil, "show", "-s", "--format=%B", hash)
	if err := res.Err2(); err != nil {
		return "", err
	}
	return strings.TrimRight(string(res.Stdout), "\n"), nil
}

// CommitFiles returns the files hash changed against parent, the first
// parent for a merge, or "" for a root commit.
func (r *Repo) CommitFiles(ctx context.Context, hash, parent string) ([]CommitFile, error) {
	var res Result
	if parent == "" {
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"show", "--no-ext-diff", "--name-status", "-z", "--format=",
			"--find-renames", hash, "--")
	} else {
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"diff", "--no-ext-diff", "--name-status", "-z",
			"--find-renames", parent, hash, "--")
	}
	if err := res.Err2(); err != nil {
		return nil, err
	}
	fields := strings.Split(string(res.Stdout), "\x00")
	var files []CommitFile
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" {
			continue
		}
		if i >= len(fields) {
			break
		}
		path := fields[i]
		i++
		f := CommitFile{Code: status[0], Path: path}
		if f.Code == 'R' || f.Code == 'C' {
			if i < len(fields) {
				f.OldPath, f.Path = path, fields[i]
				i++
			}
		}
		files = append(files, f)
	}
	return files, nil
}

// CommitDiff returns the patch of one file in hash against parent, the
// first parent for a normal commit and "" for a root commit. Merge
// commits use their first parent so the viewer never sees a combined
// diff.
func (r *Repo) CommitDiff(ctx context.Context, hash, parent, path string) (string, error) {
	var res Result
	if parent == "" {
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"show", "--no-ext-diff", "--no-color", "--format=", "--patch",
			"-M", hash, "--", path)
	} else {
		res = r.run(ctx, nil,
			"-c", "color.ui=false",
			"diff", "--no-ext-diff", "--no-color", "-M", parent, hash, "--", path)
	}
	if res.Err != nil {
		return "", res.Err
	}
	// git diff exits 1 when there are differences.
	if res.ExitCode != 0 && res.ExitCode != 1 {
		return "", &CommandError{Code: res.ExitCode, Stderr: trimmed(res.Stderr)}
	}
	if len(res.Stdout) > MaxPatchSize {
		return "", ErrPatchTooLarge
	}
	return string(res.Stdout), nil
}
