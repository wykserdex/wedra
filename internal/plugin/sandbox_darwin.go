//go:build darwin

package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"

	"wedra/internal/pipeline"
)

// macOS backend: sandbox-exec (входит в состав ОС, но помечен deprecated
// самой Apple — это компромисс, а не рекомендация).
//
// Модель: чтение хоста не ограничивается (песочница закрывает запись), запись
// разрешена только в каталоге плагина и в scratch. Каталог плагина на запись
// открыт намеренно: sandbox-exec не умеет делать read-only bind-mount, а
// запретить запись в нём нельзя без полного deny file-write*, который сломал бы
// Python. Плагин, которому нужна read-only ФС, должен объявлять
// permissions.filesystem: none и не писать — это проверяется тестом.
// Сеть изолируется, если permissions.network не объявлен.

func sandboxBackend() (string, bool) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return "", false
	}
	return "sandbox-exec", true
}

func platformSandbox(ctx context.Context, argv []string, m *pipeline.Manifest) (*exec.Cmd, error) {
	exe, ok := sandboxBackend()
	if !ok {
		return nil, fmt.Errorf("%w: sandbox-exec не найден в PATH", ErrSandboxUnsupported)
	}
	dir, err := absPluginDir(m)
	if err != nil {
		return nil, err
	}

	// Правила применяются по порядку, последнее совпадение выигрывает:
	// deny file-write* затем точечные allow на каталоги плагина и scratch.
	profile := "(version 1)(allow default)(deny file-write*)" +
		"(allow file-write* (subpath " + strconv.Quote(dir) + "))" +
		"(allow file-write* (subpath " + strconv.Quote(sandboxScratch()) + "))"
	if !declaresNetwork(m) {
		profile += "(deny network*)"
	}

	// sandbox-exec не принимает "--": команда плагина идёт сразу после профиля
	// (это абсолютный путь к интерпретатору, поэтому не начинается с "-").
	args := append([]string{"-p", profile}, argv...)
	return exec.CommandContext(ctx, exe, args...), nil
}
