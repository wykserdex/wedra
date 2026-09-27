//go:build !linux

package plugin

import (
	"fmt"
	"runtime"

	"wedra/internal/pipeline"
)

// Windows, macOS и прочие платформы: поддерживаемого изолятора нет.
// Fail-closed — untrusted-код на таких хостах не запускается, а не запускается
// без песочницы. Следующий шаг для Windows: AppContainer/Job Objects (отдельный
// инкремент).
//
// macOS попадает сюда намеренно. Реализация на sandbox-exec была, но
// sandbox-exec не умеет read-only bind mount, поэтому каталог плагина приходилось
// открывать на запись — и плагин мог переписать сам себя и закрепиться на диске.
// Защита целостности на macOS была неполной, поэтому бэкенд убран в
// attic/sandbox_darwin.go.archived вместе со своим поведенческим тестом. Вернуть
// можно, но сначала нужно решить, чем закрыть эту дыру.

func sandboxBackend() (string, bool) { return "", false }

func sandboxLauncher() (string, bool) { return "", false }

func sandboxUsable() bool { return false }

func sandboxArgs(m *pipeline.Manifest, argv []string, scratch string) (string, []string, error) {
	// Сообщение намеренно говорит, что --allow-untrusted-plugins не поможет:
	// оператор не должен искать решение в флагах, когда изолятора нет вовсе.
	return "", nil, fmt.Errorf("%w: на %s изоляция внешнего кода не реализована "+
		"(нужен bwrap на Linux; бэкенд для macOS отключён — см. SECURITY.md); "+
		"флаг --allow-untrusted-plugins не обходит песочницу, поэтому внешний код "+
		"на этой платформе запустить нельзя",
		ErrSandboxUnsupported, runtime.GOOS)
}
