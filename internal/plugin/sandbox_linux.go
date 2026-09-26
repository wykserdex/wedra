//go:build linux

package plugin

import (
	"context"
	"fmt"
	"os/exec"

	"wedra/internal/pipeline"
)

// Linux backend: bubblewrap. Требует непривилегированных user namespaces
// (на GitHub runners и в обычных дистрибутивах включены по умолчанию).
//
// Модель: файловая система хоста доступна только на чтение (`--ro-bind / /`),
// каталог плагина тоже read-only — плагин не может себя модифицировать и закрепиться
// на диске. PID/IPC/UTS-пространства отдельные, /tmp — свежий tmpfs, HOME
// перенаправлен в /tmp. Сеть изолируется, если плагин не объявил
// permissions.network (точечный egress-фильтр в bwrap невозможен: при
// объявленной сети namespace общий, это осознанный компромисс).

func sandboxBackend() (string, bool) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		return "", false
	}
	return "bwrap", true
}

func platformSandbox(ctx context.Context, argv []string, m *pipeline.Manifest) (*exec.Cmd, error) {
	bwrap, ok := sandboxBackend()
	if !ok {
		return nil, fmt.Errorf("%w: bwrap (bubblewrap) не найден в PATH — установите пакет bubblewrap", ErrSandboxUnsupported)
	}
	dir, err := absPluginDir(m)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--die-with-parent",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--unshare-cgroup-try",
		"--ro-bind", "/", "/",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--setenv", "HOME", "/tmp",
		"--setenv", "TMPDIR", "/tmp",
		"--chdir", dir,
	}
	if !declaresNetwork(m) {
		args = append(args, "--unshare-net")
	}
	args = append(args, "--")
	args = append(args, argv...)
	return exec.CommandContext(ctx, bwrap, args...), nil
}
