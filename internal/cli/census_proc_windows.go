//go:build windows

package cli

import (
	"os/exec"
	"strconv"
	"syscall"
)

func isWindows() bool { return true }

// Новая группа консоли, чтобы управляющие сигналы терминала не доставали
// до go test. CREATE_NEW_PROCESS_GROUP здесь единственный доступный вариант:
// Setpgid в Windows нет.
func applyProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// taskkill /T убивает дерево целиком. Убийство только родителя оставило бы
// тест-бинарник держать открытые файлы.
func killTree(pid int) {
	if pid <= 0 {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	_ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
}
