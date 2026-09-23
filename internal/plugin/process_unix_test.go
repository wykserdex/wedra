//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris || illumos || aix

package plugin

// v0.23: process-group kill — только Unix (на Windows Job Object нет).

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Таймаут убивает процесс-группу: дочерний sleep плагина не остаётся сиротой.
func TestExecTimeoutKillsProcessGroup(t *testing.T) {
	requirePythonT(t)
	m := fixtureManifest(t, "spawner")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	res := ExecWithEnv(m, []byte("{}"), time.Second, []string{"SPID_FILE=" + pidFile})
	if res.ErrCode != "timeout" {
		t.Fatalf("ожидался timeout, got %q (%q)", res.ErrCode, res.ErrMsg)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("pid дочернего не записан: %v", err)
	}
	var pid int
	if _, err := parsePID(string(raw), &pid); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			return
		}
		if err != nil && err != syscall.EPERM {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("дочерний процесс %d жив после таймаута — process-group kill не сработал", pid)
}
