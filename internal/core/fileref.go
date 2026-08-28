package core

// Точка трения из M5, пакет №5 (тестер №1, наступил дважды): subprocess плагина
// стартует НЕ в корне проекта, а в директории самого плагина (cmd.Dir = manifest dir,
// PROTOCOL §1). Поэтому относительный file_ref «как я вижу путь из корня» прилетает
// плагину как not_found, хотя с точки зрения автора пайплайна всё выглядело валидно.
//
// Решение (вариант «лучше» самого тестера): собрав stdin шага, ядро заранее проверяет
// относительные пути портов с format: file_ref и предупреждает ДО запуска плагина —
// в консоль (logf) и в журнал рана (event file_ref_warning).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fileRefWarnings — чистая функция: по собранному входу шага возвращает
// предупреждения для относительных file_ref-путей, не резолвящихся от m.Dir.
// root — «корень проекта» (cwd процесса tool), нужен только для хинта в тексте.
func fileRefWarnings(m *Manifest, st *Step, input map[string]interface{}, root string) []string {
	var warns []string

	names := make([]string, 0, len(m.Input))
	for name := range m.Input {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		port := m.Input[name]
		if port.Format != "file_ref" {
			continue
		}
		v, ok := input[name].(string)
		if !ok || v == "" || filepath.IsAbs(v) {
			continue // абсолютные и пустые — зона ответственности плагина
		}
		pluginAbs, err := filepath.Abs(m.Dir)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(pluginAbs, v)); err == nil {
			continue // резолвится от cwd плагина — всё честно
		}
		msg := fmt.Sprintf(
			"шаг %q: вход %s = %q — относительный путь не найден от рабочей директории плагина (%s): "+
				"subprocess плагина стартует в его собственной директории (PROTOCOL §1)",
			st.ID, name, v, pluginAbs)
		if root != "" {
			if _, err := os.Stat(filepath.Join(root, v)); err == nil {
				msg += "; такой путь есть от КОРНЯ ПРОЕКТА — вероятно, вы имели в виду его: " +
					"укажите путь абсолютно или относительно директории плагина"
			}
		}
		warns = append(warns, msg)
	}
	return warns
}

// fileRefWarningsForRun — обёртка для раннера: root = cwd процесса tool.
func fileRefWarningsForRun(m *Manifest, st *Step, input map[string]interface{}) []string {
	root, _ := os.Getwd()
	return fileRefWarnings(m, st, input, root)
}

// formatHintForLog — то же в одну строку для журнала (поиск по journal.jsonl).
func formatHintForLog(w string) string {
	return strings.ReplaceAll(w, "\n", " ")
}
