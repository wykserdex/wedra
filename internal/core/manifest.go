package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Engine резолвит ссылки на плагины и кэширует манифесты.
// Теперь живёт и в internal/plugin, здесь копия для совместимости тестов (engineWith использует cache)
type Engine struct {
	cache map[string]*Manifest
}

func NewEngine() *Engine {
	return &Engine{cache: map[string]*Manifest{}}
}

func IsBuiltin(ref string) bool { return strings.HasPrefix(ref, "core/") }

func (e *Engine) LoadManifest(ref string) (*Manifest, error) {
	if IsBuiltin(ref) {
		switch ref {
		case "core/human_gate":
			return &Manifest{ID: "core/human_gate", Version: PlatformAPI}, nil
		}
		return nil, fmt.Errorf("неизвестный встроенный модуль: %s", ref)
	}
	if m, ok := e.cache[ref]; ok {
		return m, nil
	}
	raw, err := os.ReadFile(filepath.Join(ref, "plugin.yaml"))
	if err != nil {
		return nil, fmt.Errorf("плагин %q: не читается plugin.yaml: %w", ref, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("плагин %q: некорректный манифест: %w", ref, err)
	}
	if m.ID == "" {
		return nil, fmt.Errorf("плагин %q: в манифесте нет id", ref)
	}
	m.Dir = ref
	e.cache[ref] = &m
	return &m, nil
}
