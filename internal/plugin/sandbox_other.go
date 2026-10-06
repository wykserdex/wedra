//go:build !linux

//lint:file-ignore U1000,SA4023 Платформенный файл: на этой платформе sandboxArgs
// всегда отказывает, поэтому проверки «сравнение всегда истинно» и «функция не
// используется» — артефакты того, что линтер разбирает платформенную сборку.
// В сборке с изолятором (Linux) обе функции живут в sandbox_linux.go, и там
// sandboxArgs возвращает nil-ошибку штатно.

package plugin

import (
	"fmt"
	"runtime"

	"github.com/wykserdex/wedra/internal/pipeline"
)

// Windows, macOS и прочие платформы: поддерживаемого изолятора нет.
// Fail-closed — untrusted-код на таких хостах не запускается, а не запускается
// без песочницы.
//
// AppContainer на Windows — рабочий путь: не-elevated процесс создаёт профиль
// через CreateAppContainerProfile и запускает потомка с
// PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES. Измерено на Windows 10 19041:
// интерпретатор стартует внутри контейнера, каталог плагина читается, но не
// пишется и не переписывается, сеть закрыта. Прежние записи в SECURITY.md о
// непреодолимом барьере целостности/SACL не подтвердились: метка не является
// запретом, когда DACL явно выдан контейнерному SID.
//
// Job Objects — НЕ часть этой ступени и не граница безопасности:
// JobObjectSecurityLimitInformation удалён начиная с Windows Vista,
// JOB_OBJECT_SET_SECURITY_ATTRIBUTES не поддерживается. Job objects дают
// лимиты CPU, памяти и числа процессов и гарантированное завершение дерева
// процессов. Защиту от DoS и уборку — да, изоляцию — нет.
//
// macOS попадает сюда намеренно. Реализация на sandbox-exec была, но
// sandbox-exec не умеет read-only bind mount, поэтому каталог плагина приходилось
// открывать на запись — и плагин мог переписать сам себя и закрепиться на диске.
// Защита целостности на macOS была неполной, поэтому бэкенд вместе с
// поведенческим тестом вынесен из дерева в историю git (коммит 9d17120).
// Вернуть можно, но сначала нужно решить, чем закрыть эту дыру.

func sandboxBackend() (string, bool) { return "", false }

func sandboxLauncher() (string, bool) { return "", false }

func sandboxUsable() bool { return false }

func sandboxArgs(m *pipeline.Manifest, argv []string, scratch string) (string, []string, sandboxNetwork, error) {
	// Сообщение намеренно говорит, что --allow-untrusted-plugins не поможет:
	// оператор не должен искать решение в флагах, когда изолятора нет вовсе.
	return "", nil, nil, fmt.Errorf("%w: на %s изоляция внешнего кода не реализована "+
		"(нужен bwrap на Linux; бэкенд для macOS отключён — см. SECURITY.md); "+
		"флаг --allow-untrusted-plugins не обходит песочницу, поэтому внешний код "+
		"на этой платформе запустить нельзя",
		ErrSandboxUnsupported, runtime.GOOS)
}
