package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"wedra/internal/pipeline"
	"wedra/internal/registry"
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

func IsBuiltin(ref string) bool {
	return pipeline.IsBuiltin(ref)
}

func IsBuiltinNamespace(ref string) bool {
	return pipeline.IsBuiltinNamespace(ref)
}

func (e *Engine) LoadManifest(ref string) (*pipeline.Manifest, error) {
	if IsBuiltin(ref) {
		return &pipeline.Manifest{ID: "core/human_gate", Version: pipeline.PlatformAPI}, nil
	}
	if IsBuiltinNamespace(ref) {
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
	if err := pipeline.DecodeManifest(raw, &m); err != nil {
		return nil, fmt.Errorf("плагин %q: некорректный манифест: %w", ref, err)
	}
	m.Dir = dir
	if err := pipeline.ValidateManifest(&m); err != nil {
		return nil, fmt.Errorf("плагин %q: некорректный манифест: %w", ref, err)
	}
	e.mu.Lock()
	e.Cache[ref] = &m
	e.mu.Unlock()
	return &m, nil
}
