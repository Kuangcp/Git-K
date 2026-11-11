//go:build windows

package gitx

import (
	"os/exec"
	"syscall"
)

// createNoWindow keeps Git from opening a console window for each call:
// without it, Windows shows a black window per git process and the extra
// console setup slows every status and diff.
const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
