package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/pipeline"
	"orchestrator/internal/registry"
)

type Engine struct {
	mu         sync.Mutex
	Cache      map[string]*pipeline.Manifest
	PluginsDir string // дефолт "plugins"
}

func NewEngine() *Engine {
	return &Engine{
		Cache:      make(map[string]*pipeline.Manifest),
		PluginsDir: "plugins",
	}
}

func IsBuiltin(ref string) bool { return strings.HasPrefix(ref, "core/") }

func (e *Engine) LoadManifest(ref string) (*pipeline.Manifest, error) {
	if IsBuiltin(ref) {
		switch ref {
		case "core/human_gate":
			return &pipeline.Manifest{ID: "core/human_gate", Version: pipeline.PlatformAPI}, nil
		}
		return nil, fmt.Errorf("неизвестный встроенный модуль: %s", ref)
	}
	e.mu.Lock()
	if e.Cache == nil {
		e.Cache = make(map[string]*pipeline.Manifest)
	}
	if m, ok := e.Cache[ref]; ok {
		e.mu.Unlock()
		return m, nil
	}
	e.mu.Unlock()
	// v0.16: голое имя (или имя@версия) — реестр; пути — как раньше.
	dir, err := registry.RefToDir(ref, e.PluginsDir)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return nil, fmt.Errorf("плагин %q: не читается plugin.yaml: %w", ref, err)
	}
	var m pipeline.Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("плагин %q: некорректный манифест: %w", ref, err)
	}
	if m.ID == "" {
		return nil, fmt.Errorf("плагин %q: в манифесте нет id", ref)
	}
	m.Dir = dir
	e.mu.Lock()
	e.Cache[ref] = &m
	e.mu.Unlock()
	return &m, nil
}
