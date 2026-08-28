package pipeline

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
	Foreach     string                 `yaml:"foreach"`
	ForeachItem string                 `yaml:"foreach_item"`
	ItemType    string                 `yaml:"item_type"`
	ItemFormat  string                 `yaml:"item_format"`
	Steps       []Step                 `yaml:"steps"`
}

type Step struct {
	ID      string            `yaml:"id"`
	Plugin  string            `yaml:"plugin"`
	OnError string            `yaml:"on_error"`
	Retry   *Retry            `yaml:"retry"`
	Timeout Duration          `yaml:"timeout"`
	Bind    map[string]string `yaml:"bind"`

	// v0.12: after_foreach — шаг выполняется один раз после foreach, а не per-item
	AfterForeach bool `yaml:"after_foreach"`

	// core/human_gate
	Form     []FormField `yaml:"form"`
	Actions  []string    `yaml:"actions"`
	OnReject string      `yaml:"on_reject"`
}

type Retry struct {
	Attempts int      `yaml:"attempts"`
	Delay    Duration `yaml:"delay"`
	Backoff  string   `yaml:"backoff"`
}

type FormField struct {
	Field    string `yaml:"field"`
	Editable bool   `yaml:"editable"`
	Type     string `yaml:"type"`
	Format   string `yaml:"format"`
}

// Manifest — контракт плагина
type Port struct {
	From     string `yaml:"from"`
	Type     string `yaml:"type"`
	Format   string `yaml:"format"`
	Optional bool   `yaml:"optional"`
}

type Runtime struct {
	Type     string   `yaml:"type"`
	Entry    string   `yaml:"entry"`
	Requires []string `yaml:"requires"`
}

type Permissions struct {
	Network    []map[string]interface{} `yaml:"network"`
	Filesystem string                   `yaml:"filesystem"`
	Secrets    []string                 `yaml:"secrets"`
}

type Manifest struct {
	ID          string          `yaml:"id"`
	Version     string          `yaml:"version"`
	PlatformAPI string          `yaml:"platform_api"`
	Description string          `yaml:"description"`
	Author      string          `yaml:"author"`
	Runtime     Runtime         `yaml:"runtime"`
	Input       map[string]Port `yaml:"input"`
	Output      map[string]Port `yaml:"output"`
	Permissions Permissions     `yaml:"permissions"`

	Dir string `yaml:"-"`
}

const PlatformAPI = "0.1"

// portSource — источник данных порта: bind шага приоритетнее дефолтного from
func PortSource(portName string, port Port, st *Step) string {
	if st != nil && st.Bind != nil {
		if p, ok := st.Bind[portName]; ok && p != "" {
			return p
		}
	}
	return port.From
}
