//go:build js

package plugin

import "os/exec"

func prepareProcessGroup(cmd *exec.Cmd) error { return nil }

func attachProcessGroup(cmd *exec.Cmd) error { return nil }

func cleanupProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
