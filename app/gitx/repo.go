// Package gitx runs git for git-k: one process per query or change, with a
// fixed environment, and parsing that keeps git's own semantics.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Repo is an opened working tree.
type Repo struct {
	// Root is the top of the working tree, absolute.
	Root string
	// GitDir is the repository's git directory, absolute; the one of a
	// linked worktree when Root is one.
	GitDir string
	// Name is the directory name of Root.
	Name string
	// DraftPath is where the commit message draft is kept.
	DraftPath string
}

// Open finds the repository that contains dir. An empty dir means the
// current directory. Bare repositories are rejected.
func Open(dir string) (*Repo, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	out, err := runGit(abs, nil, "rev-parse", "--show-toplevel", "--absolute-git-dir", "--is-bare-repository")
	if err != nil {
		return nil, fmt.Errorf("%s 不是 Git 仓库: %w", abs, err)
	}
	lines := splitLines(out)
	if len(lines) < 3 {
		return nil, errors.New("git rev-parse 输出不完整")
	}
	if lines[2] == "true" {
		return nil, errors.New("暂不支持裸仓库")
	}
	root, err := filepath.Abs(lines[0])
	if err != nil {
		return nil, err
	}
	gitDir, err := filepath.Abs(lines[1])
	if err != nil {
		return nil, err
	}
	return &Repo{
		Root:      root,
		GitDir:    gitDir,
		Name:      filepath.Base(root),
		DraftPath: filepath.Join(gitDir, "GIT_K_MSG"),
	}, nil
}

// Result is one git process's outcome. ExitCode is 0 for a successful
// command; Diff reads code 1 as differences.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Err      error
}

// Err returns an error when the process could not start or exited nonzero.
func (r Result) Err2() error {
	if r.Err != nil {
		return r.Err
	}
	if r.ExitCode != 0 {
		return &CommandError{Code: r.ExitCode, Stderr: strings.TrimSpace(string(r.Stderr))}
	}
	return nil
}

// CommandError is a git command that ran and failed.
type CommandError struct {
	Code   int
	Stderr string
}

func (e *CommandError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("git 退出码 %d", e.Code)
	}
	return e.Stderr
}

// gitEnv pins the environment: no optional locks (status is a read), no
// prompts, literal pathspecs so a file name is never a pattern, and a
// stable locale for parsing.
func gitEnv() []string {
	env := os.Environ()
	env = append(env,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_LITERAL_PATHSPECS=1",
		"LC_ALL=C",
	)
	return env
}

func runGit(dir string, stdin []byte, args ...string) ([]byte, error) {
	res := run(dir, stdin, args...)
	if err := res.Err2(); err != nil {
		return res.Stdout, err
	}
	return res.Stdout, nil
}

func run(dir string, stdin []byte, args ...string) Result {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	res := Result{Stdout: out.Bytes(), Stderr: errOut.Bytes()}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			res.ExitCode = exit.ExitCode()
		} else {
			res.Err = err
		}
	}
	return res
}

func (r *Repo) run(ctx context.Context, stdin []byte, args ...string) Result {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Root
	cmd.Env = gitEnv()
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	res := Result{Stdout: out.Bytes(), Stderr: errOut.Bytes()}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			res.ExitCode = exit.ExitCode()
		} else {
			res.Err = err
		}
	}
	return res
}

func splitLines(b []byte) []string {
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// ShortOID is the first seven characters of an object ID.
func ShortOID(oid string) string {
	if len(oid) > 7 {
		return oid[:7]
	}
	return oid
}

// parseAheadBehind reads the "# branch.ab +1 -2" line's numbers.
func parseAheadBehind(s string) (ahead, behind int) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0, 0
	}
	ahead, _ = strconv.Atoi(strings.TrimPrefix(f[0], "+"))
	behind, _ = strconv.Atoi(strings.TrimPrefix(f[1], "-"))
	return ahead, behind
}
