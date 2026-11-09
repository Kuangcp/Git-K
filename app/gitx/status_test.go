package gitx

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseStatusGroupsAndRenames(t *testing.T) {
	records := []string{
		"# branch.oid abcdef1234567890",
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +2 -1",
		"1 M. N... 100644 100644 100644 aaaa bbbb staged.go",
		"1 .M N... 100644 100644 100644 aaaa bbbb modified.go",
		"1 MM N... 100644 100644 100644 aaaa bbbb both.go",
		"2 R. N... 100644 100644 100644 aaaa bbbb R100 new name.go",
		"old name.go",
		"u UU N... 100644 100644 100644 100644 a b c conflict.go",
		"? untracked.txt",
		"! ignored.txt",
	}
	st := parseStatus([]byte(strings.Join(records, "\x00") + "\x00"))

	if st.Branch != "main" || st.Upstream != "origin/main" {
		t.Fatalf("branch: %+v", st)
	}
	if st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("ahead/behind: %d/%d", st.Ahead, st.Behind)
	}
	if st.HeadOID != "abcdef1234567890" {
		t.Fatalf("oid: %q", st.HeadOID)
	}

	type want struct {
		group   Group
		path    string
		oldPath string
		code    byte
	}
	wants := []want{
		{Staged, "both.go", "", 'M'},
		{Staged, "new name.go", "old name.go", 'R'},
		{Staged, "staged.go", "", 'M'},
		{Unmerged, "conflict.go", "", 'U'},
		{Modified, "both.go", "", 'M'},
		{Modified, "modified.go", "", 'M'},
		{Untracked, "untracked.txt", "", '?'},
	}
	if len(st.Rows) != len(wants) {
		t.Fatalf("rows: %d, want %d: %+v", len(st.Rows), len(wants), st.Rows)
	}
	for i, w := range wants {
		got := st.Rows[i]
		if got.Group != w.group || got.Path != w.path || got.OldPath != w.oldPath || got.Code != w.code {
			t.Fatalf("row %d: %+v, want %+v", i, got, w)
		}
	}
	if st.Count(Staged) != 3 || st.Count(Modified) != 2 || st.Count(Unmerged) != 1 || st.Count(Untracked) != 1 {
		t.Fatalf("counts: %+v", st)
	}
	if !st.HasStaged() || st.Total() != 7 {
		t.Fatalf("totals: %+v", st)
	}
}

func TestParseStatusUnusualNames(t *testing.T) {
	records := []string{
		"# branch.oid (initial)",
		"# branch.head main",
		"? dir/file with space.txt",
		"? tab\there.txt",
		"? line\nbreak.txt",
	}
	st := parseStatus([]byte(strings.Join(records, "\x00") + "\x00"))
	if len(st.Rows) != 3 {
		t.Fatalf("rows: %+v", st.Rows)
	}
	found := map[string]bool{}
	for _, row := range st.Rows {
		found[DisplayPath(row.Path)] = true
		if row.Key == rowKey(Untracked, "dir/file with space.txt") {
			found["key"] = true
		}
	}
	for _, want := range []string{"dir/file with space.txt", `tab\there.txt`, `line\nbreak.txt`, "key"} {
		if !found[want] {
			t.Fatalf("missing %q in %+v", want, st.Rows)
		}
	}
}

func TestStageArgs(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		unstage bool
		unborn  bool
		want    []string
	}{
		{"stage all", nil, false, false, []string{"add", "-A"}},
		{"stage paths", []string{"a.go", "b c.go"}, false, false, []string{"add", "-A", "--", "a.go", "b c.go"}},
		{"unstage all", nil, true, false, []string{"restore", "--staged", "."}},
		{"unstage paths", []string{"a.go"}, true, false, []string{"restore", "--staged", "--", "a.go"}},
		{"unborn all", nil, true, true, []string{"rm", "-r", "--cached", "--ignore-unmatch", "."}},
		{"unborn paths", []string{"a.go"}, true, true, []string{"rm", "-r", "--cached", "--ignore-unmatch", "--", "a.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stageArgs(tt.paths, tt.unstage, tt.unborn); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
