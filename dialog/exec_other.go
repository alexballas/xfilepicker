//go:build !windows

package dialog

import "os/exec"

func applyHiddenWindow(cmd *exec.Cmd) {
	// No-op on non-Windows platforms.
}
