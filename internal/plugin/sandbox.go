package plugin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"wedra/internal/pipeline"
)

// ErrSandboxUnsupported — на этой платформе/хосте нет изолятора внешнего кода.
// Fail-closed: ядро предпочитает отказать в запуске, чем выполнить untrusted-плагин
// без изоляции.
var ErrSandboxUnsupported = errors.New("os-изоляция внешнего кода недоступна")

// sandboxScratch — доступный плагину каталог записи внутри песочницы.
func sandboxScratch() string { return os.TempDir() }

// declaresNetwork — плагин объявил сетевые разрешения.
func declaresNetwork(m *pipeline.Manifest) bool { return m != nil && len(m.Permissions.Network) > 0 }

// sandboxCommand — оборачивает команду плагина в изолятор. Конкретная реализация
// — в sandbox_linux.go (bubblewrap), sandbox_darwin.go (sandbox-exec) и
// sandbox_other.go (отказ). Ни одна из них не ослабляет fail-closed: если
// изолятор недоступен, возвращается ErrSandboxUnsupported и процесс не создаётся.
func sandboxCommand(ctx context.Context, argv []string, m *pipeline.Manifest) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, errors.New("пустая команда плагина")
	}
	return platformSandbox(ctx, argv, m)
}

// sandboxBackendName — человекочитаемое имя активного изолятора (для логов и ошибок).
func sandboxBackendName() string {
	if name, ok := sandboxBackend(); ok {
		return name
	}
	return "нет"
}

// absPluginDir — каталог плагина для правил песочницы.
func absPluginDir(m *pipeline.Manifest) (string, error) {
	dir, err := filepath.Abs(m.Dir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(dir), nil
}
