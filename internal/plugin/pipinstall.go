package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// pinPattern — pip-спецификация обязана быть точным пином package==version.
// Без нижней границы: GUI ставит ровно то, что объявлено в манифесте, и
// ничего другого. Произвольные строки сюда попасть не могут — вызывающий
// передаёт только m.Runtime.Requires из проверенного манифеста.
var pinPattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]+==[^;\s]+$`)

// PipInstall ставит зависимости плагина через интерпретатор, которым плагины
// и запускаются (pythonInterpreter, с кэшем). Возвращает объединённый вывод
// pip для показа в интерфейсе.
func PipInstall(specs []string) (string, error) {
	for _, s := range specs {
		if !pinPattern.MatchString(strings.TrimSpace(s)) {
			return "", fmt.Errorf("отклонено: %q — ожидается package==version", s)
		}
	}
	py, err := pythonInterpreter()
	if err != nil {
		return "", fmt.Errorf("нет интерпретатора: %w", err)
	}
	args := append([]string{"-m", "pip", "install", "--disable-pip-version-check"}, specs...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, py, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("pip install: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
