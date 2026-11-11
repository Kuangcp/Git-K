package gitx

import "strings"

// msysPath rewrites an absolute MSYS2/Cygwin path to a Windows one:
// /d/Code/Go/git-k and /cygdrive/d/Code/Go/git-k become
// D:\Code\Go\git-k. Other paths are returned unchanged. It runs only on
// Windows; keeping it here makes it testable anywhere.
func msysPath(p string) string {
	if strings.HasPrefix(p, "/cygdrive/") {
		rest := p[len("/cygdrive/"):]
		if len(rest) >= 1 && driveLetter(rest[0]) && (len(rest) == 1 || rest[1] == '/') {
			return windowsPath(rest)
		}
		return p
	}
	if len(p) >= 2 && p[0] == '/' && driveLetter(p[1]) && (len(p) == 2 || p[2] == '/') {
		return windowsPath(p[1:])
	}
	return p
}

func driveLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// windowsPath turns d/Code/Go into D:\Code\Go, or d into D:\.
func windowsPath(rest string) string {
	drive := strings.ToUpper(rest[:1])
	tail := strings.TrimPrefix(rest[1:], "/")
	if tail == "" {
		return drive + `:\`
	}
	return drive + `:\` + strings.ReplaceAll(tail, "/", `\`)
}
