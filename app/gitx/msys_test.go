package gitx

import "testing"

func TestMSYSPath(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`/d/Code/Go/git-k`, `D:\Code\Go\git-k`},
		{`/cygdrive/d/Code/Go/git-k`, `D:\Code\Go\git-k`},
		{`/d`, `D:\`},
		{`/d/`, `D:\`},
		{`/home/zk/code`, `/home/zk/code`},
		{`C:\Code\git-k`, `C:\Code\git-k`},
		{`//server/share/repo`, `//server/share/repo`},
	} {
		if got := msysPath(c.in); got != c.want {
			t.Fatalf("msysPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
