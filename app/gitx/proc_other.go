//go:build !windows

package gitx

import "os/exec"

func hideConsole(cmd *exec.Cmd) {}
