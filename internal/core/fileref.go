package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
			continue
		}
		pluginAbs, err := filepath.Abs(m.Dir)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(pluginAbs, v)); err == nil {
			continue
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

func fileRefWarningsForRun(m *Manifest, st *Step, input map[string]interface{}) []string {
	root, _ := os.Getwd()
	return fileRefWarnings(m, st, input, root)
}

func formatHintForLog(w string) string {
	return strings.ReplaceAll(w, "\n", " ")
}
