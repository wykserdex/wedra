//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris || illumos || aix

package plugin

import (
	"os/exec"
	"syscall"
)

// v0.23: процесс-группа — таймаут убивает плагин вместе с дочерними.
// (Unix: Setpgid + kill(-pid).)
func prepareProcessGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func attachProcessGroup(cmd *exec.Cmd) error { return nil }

func cleanupProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
