//go:build windows || js

package plugin

import "os/exec"

// v0.23: под Windows process group не ставим — честное ограничение:
// командная группа/Job Object не реализуется в первом срезе; CommandContext
// всё равно режет прямой процесс. Дочерние под Windows — в бэклог.
func setProcGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {}
