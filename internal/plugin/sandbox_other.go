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

func sandboxArgs(m *pipeline.Manifest, argv []string) (string, []string, error) {
	return "", nil, fmt.Errorf("%w: для %s нет поддерживаемого изолятора (нужен bwrap на Linux или sandbox-exec на macOS)", ErrSandboxUnsupported, runtime.GOOS)
}
