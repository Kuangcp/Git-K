//go:build windows

package gitx

// nativePath accepts paths from MSYS2 shells and git on Windows, which
// may arrive as /d/Code/... rather than D:\Code\...
func nativePath(p string) string { return msysPath(p) }
