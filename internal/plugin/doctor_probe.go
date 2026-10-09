package plugin

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/wykserdex/wedra/internal/common"
)

// InterpreterInfo — ответ на вопрос «чем именно будут запускаться
// python-плагины».
type InterpreterInfo struct {
	// Path — интерпретатор, который вернул бы pythonInterpreter(), то есть
	// уже прошедший пробу на заглушку Microsoft Store.
	Path string `json:"path"`
	// Version — версия, как её печатает сам интерпретатор.
	Version string `json:"version"`
}

// PythonInfo опрашивает окружение: находит интерпретатор тем же кодом, каким
// запускаются плагины, и спрашивает у него версию.
//
// Это ОПРОС, а не запуск плагина: ни один плагин не выполняется, ничего не
// ставится, сеть не используется. Интерпретатор приходится запустить — иначе
// версию не узнать, а версия нужна человеку в первую очередь.
//
// Отдельная функция, а не расширение pythonInterpreter(), чтобы правило
// «спросить того же, кем запускаем» не разъезжалось по коду с тем местом,
// где плагины реально стартуют.
func PythonInfo() (InterpreterInfo, error) {
	py, err := pythonInterpreter()
	if err != nil {
		return InterpreterInfo{}, err
	}
	out, err := runProbe(py, "import sys; print(sys.version.split()[0])")
	if err != nil {
		return InterpreterInfo{}, fmt.Errorf("интерпретатор %s найден, но версию не отдали: %w", py, err)
	}
	return InterpreterInfo{Path: py, Version: strings.TrimSpace(out)}, nil
}

// PythonModulesPresent — какие из верхнеуровневых имён импортируются
// интерпретатором.
//
// ОДИН запуск интерпретатора на весь список, а не по процессу на пакет:
// пакетов бывает десятки, а каждый python -c стоит сотни миллисекунд только на
// старте. Проверка через importlib.util.find_spec, а не через import: find_spec
// находит модуль, НЕ исполняя его, поэтому тяжёлый пакет не тянет за собой
// побочные эффекты, а отсутствие пакета не превращается в исключение.
//
// Имена приходят из манифестов, то есть из файлов, которые может править
// кто угодно, поэтому передаются через stdin, а не аргументами командной
// строки: так из них нельзя сделать фрагмент кода.
const pythonProbeScript = `import sys, json
try:
    import importlib.util as u
except Exception:
    sys.exit(3)
print(json.dumps({n: u.find_spec(n) is not None for n in json.load(sys.stdin)}))`

// PythonModulesPresent возвращает карту «имя → импортируется». Отсутствие пакета
// — не ошибка: это и есть ответ. Ошибка возвращается только когда проба сама не
// отработала (нет интерпретатора, сломанный python, нечитаемый вывод).
func PythonModulesPresent(py string, modules []string) (map[string]bool, error) {
	present := map[string]bool{}
	names := dedupeSorted(modules)
	if len(names) == 0 {
		return present, nil
	}
	if py == "" {
		return nil, fmt.Errorf("не указан интерпретатор для пробы импортов")
	}
	stdin, err := json.Marshal(names)
	if err != nil {
		return nil, fmt.Errorf("подготовить список имён: %w", err)
	}
	cmd := exec.Command(py, "-X", "utf8", "-c", pythonProbeScript)
	cmd.Stdin = strings.NewReader(string(stdin))
	cmd.WaitDelay = common.ProcWaitDelay
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("проба импортов не отработала (%s): %w", py, err)
	}
	var decoded map[string]bool
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &decoded); err != nil {
		return nil, fmt.Errorf("проба импортов вернула неразборчивый вывод: %w", err)
	}
	for name, ok := range decoded {
		present[name] = ok
	}
	return present, nil
}

// runProbe выполняет короткий код на интерпретаторе и возвращает stdout.
//
// WaitDelay здесь — из того же соображения, что и при запуске плагинов: если
// проба что-то утечёт, ждать закрытия пайпов вечно нельзя.
func runProbe(py, code string) (string, error) {
	cmd := exec.Command(py, "-X", "utf8", "-c", code)
	cmd.WaitDelay = common.ProcWaitDelay
	out, err := common.Output(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// dedupeSorted убирает повторы и сортирует: порядок проб не должен зависеть от
// порядка обхода манифестов, иначе вывод doctor дрожит между запусками.
func dedupeSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
