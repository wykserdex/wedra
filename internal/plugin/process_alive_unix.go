//go:build !windows

package plugin

// Проверка «процесс жив» без указанного сигнала (kill pid 0).

import "syscall"

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
