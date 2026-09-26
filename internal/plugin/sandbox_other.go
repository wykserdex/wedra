//go:build !linux && !darwin

package plugin

import (
	"fmt"
	"runtime"

	"wedra/internal/pipeline"
)

// Windows и прочие платформы: поддерживаемого изолятора нет. Fail-closed —
// untrusted-код на таких хостах не запускается, а не запускается без песочницы.
// Следующий шаг для Windows: AppContainer/Job Objects (отдельный инкремент).

func sandboxBackend() (string, bool) { return "", false }

func sandboxLauncher() (string, bool) { return "", false }

func sandboxUsable() bool { return false }

func sandboxArgs(m *pipeline.Manifest, argv []string, scratch string) (string, []string, error) {
	// Сообщение намеренно говорит, что --allow-untrusted-plugins не поможет:
	// оператор не должен искать решение в флагах, когда изолятора нет вовсе.
	return "", nil, fmt.Errorf("%w: на %s изоляция внешнего кода не реализована "+
		"(нужен bwrap на Linux или sandbox-exec на macOS); флаг --allow-untrusted-plugins "+
		"не обходит песочницу, поэтому внешний код на этой платформе запустить нельзя",
		ErrSandboxUnsupported, runtime.GOOS)
}
