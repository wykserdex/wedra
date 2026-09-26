//go:build darwin

package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"wedra/internal/pipeline"
)

// macOS backend: sandbox-exec (входит в состав ОС, но помечен deprecated
// самой Apple — это компромисс, а не рекомендация).
//
// Модель: чтение хоста не ограничивается (песочница закрывает запись), запись
// разрешена только в каталоге плагина и в scratch. Каталог плагина на запись
// открыт намеренно: sandbox-exec не умеет делать read-only bind-mount, а полный
// deny file-write* сломал бы Python. Сеть изолируется, если permissions.network
// не объявлен.

var (
	darwinProbeOnce sync.Once
	darwinProbeOK   bool
)

func sandboxBackend() (string, bool) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return "", false
	}
	return "sandbox-exec", true
}

func sandboxLauncher() (string, bool) {
	p, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return "", false
	}
	return p, true
}

// sandboxProfile — правила песочницы. Порядок важен: последнее совпадение
// выигрывает, поэтому сначала запрещаем запись, затем разрешаем её в двух
// каталогах.
func sandboxProfile(pluginDir, scratch string, network bool) string {
	profile := "(version 1)(allow default)(deny file-write*)" +
		"(allow file-write* (subpath " + strconv.Quote(pluginDir) + "))" +
		"(allow file-write* (subpath " + strconv.Quote(scratch) + "))"
	if !network {
		profile += "(deny network*)"
	}
	return profile
}

// sandboxUsable — реальная проба записи в каталог плагина под профилем.
// Именно она ловит случаи, когда профиль принят, но allow-правило не срабатывает
// (резолв путей, права каталога). Результат кэшируется на процесс.
func sandboxUsable() bool {
	launcher, ok := sandboxLauncher()
	if !ok {
		return false
	}
	darwinProbeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "wedra-sandbox-probe-")
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)
		resolved := resolveSandboxPath(dir)
		marker := filepath.Join(resolved, "probe")
		profile := sandboxProfile(resolved, resolved, true)
		c := exec.Command(launcher, "-p", profile, "/bin/sh", "-c", "printf x > "+strconv.Quote(marker))
		darwinProbeOK = c.Run() == nil
		if darwinProbeOK {
			if _, err := os.Stat(marker); err != nil {
				darwinProbeOK = false
			}
		}
	})
	return darwinProbeOK
}

func sandboxArgs(m *pipeline.Manifest, argv []string) (string, []string, error) {
	launcher, ok := sandboxLauncher()
	if !ok {
		return "", nil, fmt.Errorf("%w: sandbox-exec не найден в PATH", ErrSandboxUnsupported)
	}
	if !sandboxUsable() {
		return "", nil, fmt.Errorf("%w: sandbox-exec не смог выполнить пробную запись в каталог плагина", ErrSandboxUnsupported)
	}
	// sandbox-exec не принимает "--": команда плагина идёт сразу после профиля
	// (это абсолютный путь к интерпретатору, поэтому не начинается с "-").
	profile := sandboxProfile(resolveSandboxPath(m.Dir), resolveSandboxPath(sandboxScratch()), declaresNetwork(m))
	return launcher, append([]string{"-p", profile}, argv...), nil
}
