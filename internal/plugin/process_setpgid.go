//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris || illumos || aix

package plugin

import (
	"os/exec"
	"syscall"
)

// v0.23: процесс-группа — таймаут убивает плагин вместе с дочерними.
// (Unix: Setpgid + kill(-pid).)
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
