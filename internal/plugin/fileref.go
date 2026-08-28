package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"orchestrator/internal/pipeline"
)

type FileRefManifest = pipeline.Manifest

// FileRefWarnings — проверка относительных file_ref до запуска (вынесено из core/fileref.go)
func FileRefWarnings(m *pipeline.Manifest, input map[string]interface{}) []string {
	var warns []string
	for name, port := range m.Input {
		if port.Format != "file_ref" {
			continue
		}
		v, ok := input[name]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if filepath.IsAbs(s) {
			continue
		}
		// относительный путь — проверяем от cwd плагина
		if _, err := os.Stat(filepath.Join(m.Dir, s)); err != nil {
			// есть от корня репозитория?
			if _, err2 := os.Stat(s); err2 == nil {
				warns = append(warns, fmt.Sprintf("file_ref %s=%q не найден от %s, но есть от корня — используйте путь от корня или скопируйте файл", name, s, m.Dir))
			} else {
				warns = append(warns, fmt.Sprintf("file_ref %s=%q не найден (cwd плагина %s)", name, s, m.Dir))
			}
		}
	}
	return warns
}
