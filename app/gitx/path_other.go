//go:build !windows

package gitx

func nativePath(p string) string { return p }
