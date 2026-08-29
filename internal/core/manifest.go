package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"orchestrator/internal/registry"
)

type Engine struct {
	Cache       map[string]*Manifest
	PluginsDir  string // куда ставятся плагины из реестра (дефолт "plugins")
	RegistrySrc string // источник реестра (дефолт: registry.DefaultSource())
}

func NewEngine() *Engine {
	return &Engine{
		Cache:      make(map[string]*Manifest),
		PluginsDir: "plugins",
	}
}

func IsBuiltin(ref string) bool {
	return strings.HasPrefix(ref, "core/")
}

func (e *Engine) LoadManifest(ref string) (*Manifest, error) {
	if IsBuiltin(ref) {
		switch ref {
		case "core/human_gate":
			return &Manifest{ID: "core/human_gate", Version: PlatformAPI}, nil
		}
		return nil, fmt.Errorf("неизвестный встроенный модуль: %s", ref)
	}
	if e.Cache == nil {
		e.Cache = make(map[string]*Manifest)
	}
	if m, ok := e.Cache[ref]; ok {
		return m, nil
	}
	// v0.16: голое имя (или имя@версия) — это имя из реестра;
	// локальные пути проходят как раньше.
	dir, err := registry.RefToDir(ref, e.PluginsDir)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
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
	m.Dir = dir
	e.Cache[ref] = &m
	return &m, nil
}
