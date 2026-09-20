//go:build windows

package plugin

// Проверка «процесс жив» на Windows: сигнала 0 тут нет,
// дёргаем дескриптор через OpenProcess.

import "syscall"

func pidAlive(pid int) bool {
	const processQueryInformation = 0x0400
	h, err := syscall.OpenProcess(processQueryInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}
