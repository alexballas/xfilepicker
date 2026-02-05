//go:build windows

package dialog

import (
	"os/exec"
	"syscall"
)

func applyHiddenWindow(cmd *exec.Cmd) {
	// Prevents a console window from flashing when launching ffmpeg on Windows.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
