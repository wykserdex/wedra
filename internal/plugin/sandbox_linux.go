//go:build linux

package plugin

import (
	"fmt"
	"io"
	"os/exec"
	"sync"

	"wedra/internal/pipeline"
)

// Linux backend: bubblewrap. Требует непривилегированных user namespaces.
//
// Модель: файловая система хоста доступна только на чтение (`--ro-bind / /`),
// каталог плагина тоже read-only — плагин не может себя модифицировать и закрепиться
// на диске. PID/IPC/UTS-пространства отдельные, /tmp — свежий tmpfs, HOME
// перенаправлен в /tmp. Сеть изолируется, если плагин не объявил
// permissions.network (точечный egress-фильтр в bwrap невозможен: при
// объявленной сети namespace общий, это осознанный компромисс).

var (
	linuxProbeOnce sync.Once
	linuxProbeOK   bool
)

func sandboxBackend() (string, bool) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		return "", false
	}
	return "bwrap", true
}

func sandboxLauncher() (string, bool) {
	p, err := exec.LookPath("bwrap")
	if err != nil {
		return "", false
	}
	return p, true
}

// sandboxUsable — может ли хост реально создать песочницу. Наличия bwrap в PATH
// недостаточно: user namespaces могут быть запрещены политикой ядра или
// ограничением CI-раннера (bwrap падает на RTM_NEWADDR при --unshare-net).
// Проба выполняется один раз на процесс и кэшируется.
func sandboxUsable() bool {
	launcher, ok := sandboxLauncher()
	if !ok {
		return false
	}
	linuxProbeOnce.Do(func() {
		c := exec.Command(launcher, "--unshare-all", "--ro-bind", "/", "/",
			"--proc", "/proc", "--dev", "/dev", "/bin/true")
		c.Stdout, c.Stderr = io.Discard, io.Discard
		linuxProbeOK = c.Run() == nil
	})
	return linuxProbeOK
}

func sandboxArgs(m *pipeline.Manifest, argv []string) (string, []string, error) {
	launcher, ok := sandboxLauncher()
	if !ok {
		return "", nil, fmt.Errorf("%w: bwrap (bubblewrap) не найден в PATH — установите пакет bubblewrap", ErrSandboxUnsupported)
	}
	if !sandboxUsable() {
		return "", nil, fmt.Errorf("%w: bwrap есть, но хост не разрешает user namespaces (--unshare-all) — песочницу собрать нельзя", ErrSandboxUnsupported)
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
		"--chdir", resolveSandboxPath(m.Dir),
	}
	if !declaresNetwork(m) {
		args = append(args, "--unshare-net")
	}
	args = append(args, "--")
	args = append(args, argv...)
	return launcher, args, nil
}
