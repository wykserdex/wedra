package core

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Duration — обёртка над time.Duration для YAML-строк вида "10s", "250ms".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

type PipelineFile struct {
	FormatVersion string   `yaml:"format_version"`
	Pipeline      Pipeline `yaml:"pipeline"`
}

type Pipeline struct {
	Name        string                 `yaml:"name"`
	Input       map[string]interface{} `yaml:"input"`
	Foreach     string                 `yaml:"foreach"`      // путь к массиву, напр. "input.emails"
	ForeachItem string                 `yaml:"foreach_item"` // ключ текущего элемента, дефолт "item"
	ItemType    string                 `yaml:"item_type"`    // объявление типа элемента для статической валидации
	ItemFormat  string                 `yaml:"item_format"`  // напр. email; без этого порт с format: email не срезолвится
	Steps       []Step                 `yaml:"steps"`
}

type Step struct {
	ID      string            `yaml:"id"`
	Plugin  string            `yaml:"plugin"`   // путь к папке плагина или core/human_gate
	OnError string            `yaml:"on_error"` // stop | skip | retry (дефолт stop)
	Retry   *Retry            `yaml:"retry"`
	Timeout Duration          `yaml:"timeout"`
	Bind    map[string]string `yaml:"bind"`     // v0.2: порт → путь в контексте, перекрывает from манифеста

	// core/human_gate
	Form     []FormField `yaml:"form"`
	Actions  []string    `yaml:"actions"`
	OnReject string      `yaml:"on_reject"` // stop (дефолт) | continue
}

// portSource — источник данных порта: bind шага приоритетнее дефолтного
// from манифеста (PROTOCOL.md §10, контракт v0.2).
func portSource(portName string, port Port, st *Step) string {
	if st != nil && st.Bind != nil {
		if p, ok := st.Bind[portName]; ok && p != "" {
			return p
		}
	}
	return port.From
}

type Retry struct {
	Attempts int      `yaml:"attempts"`
	Delay    Duration `yaml:"delay"`
	Backoff  string   `yaml:"backoff"` // fixed | exponential
}

type FormField struct {
	Field    string `yaml:"field"`
	Editable bool   `yaml:"editable"`
	Type     string `yaml:"type"`
	Format   string `yaml:"format"`
}
