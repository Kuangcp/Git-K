package gitx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Group is the section of the status view a row belongs to. A file with
// both staged and unstaged changes is a row in each group.
type Group uint8

const (
	// Staged rows are the changes in the index.
	Staged Group = iota
	// Unmerged rows are files with conflicts.
	Unmerged
	// Modified rows are unstaged changes in the working tree.
	Modified
	// Untracked rows are files git does not track yet.
	Untracked
)

// String is the group's label in the UI.
func (g Group) String() string {
	switch g {
	case Staged:
		return "已暂存"
	case Unmerged:
		return "冲突"
	case Modified:
		return "已修改"
	case Untracked:
		return "未跟踪"
	}
	return "未知"
}

// Row is one file in one group.
type Row struct {
	// Key is unique among rows: a file in two groups has two keys.
	Key string
	// Group is the section the row shows in.
	Group Group
	// Path is relative to Repo.Root, with forward slashes.
	Path string
	// OldPath is where a rename or copy came from, when git says.
	OldPath string
	// X and Y are the porcelain v2 status codes: X the index's, Y the
	// working tree's.
	X, Y byte
	// Code is the letter the UI shows: X for a staged row, Y for a
	// modified row, '?' or 'U'.
	Code byte
}

// Status is one git status snapshot.
type Status struct {
	Branch   string
	Upstream string
	HeadOID  string
	Ahead    int
	Behind   int
	Detached bool
	Rows     []Row
	// IndexMTime changes when the index does, to key diff caches.
	IndexMTime time.Time
}

// Status reads the repository in one git process: branch, upstream,
// ahead/behind, and every changed path, with -z so unusual names are
// exact.
func (r *Repo) Status(ctx context.Context) (Status, error) {
	res := r.run(ctx, nil,
		"-c", "core.quotepath=false",
		"status", "--porcelain=v2", "-z", "--branch",
		"--untracked-files=all", "--ignore-submodules=none")
	if err := res.Err2(); err != nil {
		return Status{}, err
	}
	st := parseStatus(res.Stdout)
	if fi, err := os.Stat(filepath.Join(r.GitDir, "index")); err == nil {
		st.IndexMTime = fi.ModTime()
	}
	return st, nil
}

// parseStatus reads the NUL-separated porcelain v2 records.
func parseStatus(buf []byte) Status {
	var st Status
	recs := bytesFields(buf)
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if len(rec) == 0 {
			continue
		}
		switch rec[0] {
		case '#':
			parseBranchHeader(&st, string(rec))
		case '1':
			f := strings.SplitN(string(rec), " ", 9)
			if len(f) != 9 {
				continue
			}
			st.addXY(f[1], f[8], "")
		case '2':
			f := strings.SplitN(string(rec), " ", 10)
			if len(f) != 10 {
				continue
			}
			old := ""
			if i+1 < len(recs) {
				old = string(recs[i+1])
				i++
			}
			st.addXY(f[1], f[9], old)
		case 'u':
			f := strings.SplitN(string(rec), " ", 11)
			if len(f) != 11 {
				continue
			}
			path := f[10]
			st.Rows = append(st.Rows, Row{
				Key:   rowKey(Unmerged, path),
				Group: Unmerged,
				Path:  path,
				X:     f[1][0],
				Y:     f[1][1],
				Code:  'U',
			})
		case '?':
			path := strings.TrimPrefix(string(rec), "? ")
			st.Rows = append(st.Rows, Row{
				Key:   rowKey(Untracked, path),
				Group: Untracked,
				Path:  path,
				Code:  '?',
			})
		case '!':
			// Ignored files are not shown.
		}
	}
	sortRows(st.Rows)
	return st
}

func (st *Status) addXY(xy, path, old string) {
	if len(xy) < 2 {
		return
	}
	x, y := xy[0], xy[1]
	if x != '.' && x != ' ' {
		st.Rows = append(st.Rows, Row{
			Key:     rowKey(Staged, path),
			Group:   Staged,
			Path:    path,
			OldPath: old,
			X:       x,
			Y:       y,
			Code:    x,
		})
	}
	if y != '.' && y != ' ' {
		st.Rows = append(st.Rows, Row{
			Key:     rowKey(Modified, path),
			Group:   Modified,
			Path:    path,
			OldPath: old,
			X:       x,
			Y:       y,
			Code:    y,
		})
	}
}

func parseBranchHeader(st *Status, line string) {
	const p = "# branch."
	if !strings.HasPrefix(line, p) {
		return
	}
	rest := strings.TrimPrefix(line, p)
	switch {
	case strings.HasPrefix(rest, "oid "):
		st.HeadOID = strings.TrimPrefix(rest, "oid ")
	case strings.HasPrefix(rest, "head "):
		head := strings.TrimPrefix(rest, "head ")
		if head == "(detached)" {
			st.Detached = true
		} else {
			st.Branch = head
		}
	case strings.HasPrefix(rest, "upstream "):
		st.Upstream = strings.TrimPrefix(rest, "upstream ")
	case strings.HasPrefix(rest, "ab "):
		st.Ahead, st.Behind = parseAheadBehind(strings.TrimPrefix(rest, "ab "))
	}
}

func rowKey(g Group, path string) string {
	return strconv.Itoa(int(g)) + "\x00" + path
}

func sortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Path < b.Path
	})
}

// bytesFields splits buf at NUL without copying the records.
func bytesFields(buf []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range buf {
		if b == 0 {
			out = append(out, buf[start:i])
			start = i + 1
		}
	}
	if start < len(buf) {
		out = append(out, buf[start:])
	}
	return out
}

// DisplayPath replaces characters a single line cannot show.
func DisplayPath(path string) string {
	return strings.NewReplacer("\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(path)
}

// Count returns how many rows are in group g.
func (st Status) Count(g Group) int {
	n := 0
	for _, row := range st.Rows {
		if row.Group == g {
			n++
		}
	}
	return n
}

// Total returns the number of rows.
func (st Status) Total() int { return len(st.Rows) }

// HasStaged reports whether the index has changes to commit.
func (st Status) HasStaged() bool { return st.Count(Staged) > 0 }

// DiffKey identifies what a row's diff shows, for caching.
func (st Status) DiffKey(row Row) string {
	head := st.HeadOID
	if head == "" {
		head = "unborn"
	}
	return fmt.Sprintf("%d\x00%s\x00%s\x00%d", row.Group, row.Path, head, st.IndexMTime.UnixNano())
}
