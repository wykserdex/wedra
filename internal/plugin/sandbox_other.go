//go:build !linux && !darwin

package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"wedra/internal/pipeline"
)

// Windows и прочие платформы: поддерживаемого изолятора нет. Fail-closed —
// untrusted-код на таких хостах не запускается, а не запускается без песочницы.
// Следующий шаг для Windows: AppContainer/Job Objects (отдельный инкремент).

func sandboxBackend() (string, bool) { return "", false }

func platformSandbox(ctx context.Context, argv []string, m *pipeline.Manifest) (*exec.Cmd, error) {
	return nil, fmt.Errorf("%w: для %s нет поддерживаемого изолятора (нужен bwrap на Linux или sandbox-exec на macOS)", ErrSandboxUnsupported, runtime.GOOS)
}
