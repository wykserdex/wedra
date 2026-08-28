package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const PlatformAPI = "0.1"

type Port struct {
	From     string `yaml:"from"`
	Type     string `yaml:"type"`
	Format   string `yaml:"format"`
	Optional bool   `yaml:"optional"`
}

type Runtime struct {
	Type     string   `yaml:"type"` // python | binary | ...
	Entry    string   `yaml:"entry"`
	Requires []string `yaml:"requires"`
}

type Permissions struct {
	Network    []map[string]interface{} `yaml:"network"`
	Filesystem string                   `yaml:"filesystem"` // none | workspace | paths:[...] (L0: декларативно)
	Secrets    []string                 `yaml:"secrets"`
}

type Manifest struct {
	ID          string          `yaml:"id"`
	Version     string          `yaml:"version"`
	PlatformAPI string          `yaml:"platform_api"`
	Description string          `yaml:"description"` // метаданные маркетплейса
	Author      string          `yaml:"author"`
	Runtime     Runtime         `yaml:"runtime"`
	Input       map[string]Port `yaml:"input"`
	Output      map[string]Port `yaml:"output"`
	Permissions Permissions     `yaml:"permissions"`

	Dir string `yaml:"-"`
}

// Engine резолвит ссылки на плагины и кэширует манифесты.
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
