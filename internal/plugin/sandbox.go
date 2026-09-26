package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"wedra/internal/pipeline"
)

// ErrSandboxUnsupported — на этой платформе/хосте нет рабочей изоляции.
// Fail-closed: ядро предпочитает отказать в запуске, чем выполнить untrusted-плагин
// без песочницы.
var ErrSandboxUnsupported = errors.New("os-изоляция внешнего кода недоступна")

// declaresNetwork — плагин объявил сетевые разрешения.
func declaresNetwork(m *pipeline.Manifest) bool { return m != nil && len(m.Permissions.Network) > 0 }

// sandboxCommand — обёртка для запуска внешнего кода. Launcher и его аргументы
// собираются платформенной реализацией sandboxArgs (bubblewrap на Linux,
// sandbox-exec на macOS); платформы без изолятора возвращают ErrSandboxUnsupported,
// и процесс не создаётся вовсе.
//
// Возвращает cleanup, который удаляет приватный scratch-каталог: он живёт ровно
// столько, сколько идёт процесс плагина, и на хосте после запуска не остаётся.
func sandboxCommand(ctx context.Context, argv []string, m *pipeline.Manifest) (*exec.Cmd, func(), error) {
	noop := func() {}
	if len(argv) == 0 {
		return nil, noop, errors.New("пустая команда плагина")
	}
	// Собственный каталог вместо общего os.TempDir(): он приватен этому запуску и
	// гарантированно существует до монтирования. Точка монтирования, которой нет
	// на хосте, не годится: после --ro-bind / / создать её уже нельзя, и bwrap
	// падает с "Read-only file system".
	scratch, err := os.MkdirTemp("", "wedra-sbx-")
	if err != nil {
		return nil, noop, fmt.Errorf("scratch для песочницы: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(scratch) }

	launcher, args, err := sandboxArgs(m, argv, scratch)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	return exec.CommandContext(ctx, launcher, args...), cleanup, nil
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
