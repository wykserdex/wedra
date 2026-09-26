package plugin

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"wedra/internal/pipeline"
)

// ErrSandboxUnsupported — на этой платформе/хосте нет рабочей изоляции.
// Fail-closed: ядро предпочитает отказать в запуске, чем выполнить untrusted-плагин
// без песочницы.
var ErrSandboxUnsupported = errors.New("os-изоляция внешнего кода недоступна")

// sandboxScratch — доступный плагину каталог записи внутри песочницы.
func sandboxScratch() string { return os.TempDir() }

// declaresNetwork — плагин объявил сетевые разрешения.
func declaresNetwork(m *pipeline.Manifest) bool { return m != nil && len(m.Permissions.Network) > 0 }

// sandboxCommand — обёртка для запуска внешнего кода. Launcher и его аргументы
// собираются платформенной реализацией sandboxArgs (bubblewrap на Linux,
// sandbox-exec на macOS); платформы без изолятора возвращают ErrSandboxUnsupported,
// и процесс не создаётся вовсе.
func sandboxCommand(ctx context.Context, argv []string, m *pipeline.Manifest) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, errors.New("пустая команда плагина")
	}
	launcher, args, err := sandboxArgs(m, argv)
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, launcher, args...), nil
}

// sandboxBackendName — человекочитаемое имя изолятора (для логов и ошибок).
func sandboxBackendName() string {
	if name, ok := sandboxBackend(); ok {
		return name
	}
	return "нет"
}

// resolveSandboxPath — путь для правил песочницы. Обязательно резолвим symlink'и:
// на macOS $TMPDIR лежит под /var -> /private/var, а sandbox-exec матчится по
// уже резолвнутому пути, из-за чего нерезолвнутый путь не попадает в allow-правило
// и запись честного плагина запрещается.
func resolveSandboxPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}
