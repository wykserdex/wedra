package plugin

// v0.23: надёжность запуска плагина (лимит вывода, process-group kill).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"wedra/internal/pipeline"
)

func requirePythonT(t *testing.T) {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	t.Skip("python не найден — пропускаю")
}

func fixtureManifest(t *testing.T, name string) *pipeline.Manifest {
	t.Helper()
	dir, _ := filepath.Abs(filepath.Join("..", "core", "testdata", "plugins", name))
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var m pipeline.Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Dir = dir
	return &m
}

// Гигантский stdout (17MB) — не «всю память», а честная ошибка протокола.
func TestExecStdoutCap(t *testing.T) {
	requirePythonT(t)
	m := fixtureManifest(t, "chatter")
	res := Exec(m, []byte("{}"), 30*time.Second)
	if !res.Platform || res.ErrCode != "protocol_violation" {
		t.Fatalf("ожидался protocol_violation, got platform=%v err=%q msg=%q", res.Platform, res.ErrCode, res.ErrMsg)
	}
	if len(res.ErrMsg) == 0 || !strings.Contains(res.ErrMsg, "лимит") {
		t.Fatalf("сообщение не про лимит: %q", res.ErrMsg)
	}
}

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
	// до 5 секунд ждём смерти дочернего (group kill должен быть мгновенным)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// os.Kill(pid, 0) — проверка существования без сигнала
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			return // умер — отлично
		}
		if err != nil && err != syscall.EPERM {
			return // тоже не жив
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("дочерний процесс %d жив после таймаута — process-group kill не сработал", pid)
}

func parsePID(s string, out *int) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			continue
		}
		n = n*10 + int(c-'0')
	}
	*out = n
	return n, nil
}
