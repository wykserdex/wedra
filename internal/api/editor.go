package api

// v0.25: редактор пайплайнов — парсинг/сериализация через Go (JS тонкий,
// корректность YAML живёт в ядре, а не в браузере).
//
// Модель редактора (EditorDoc) — подмножество схемы: id/plugin/pos/bind/
// on_error/timeout + у гейта form/actions/on_reject + input-дефолты.
// Позиции узлов хранятся в YAML как `pos: [x, y]` — лоадер ядра это поле
// игнорирует (yaml.v3 без KnownFields), редактор читает обратно.
// v0.27: when — под управлением редактора (path/op/value, 10 операторов ядра).
// Всё, чего редактор не управляет (foreach, parallel_group, after_foreach,
// retry, secrets, network, type-объявления в input) — выносится в unsupported:
// сохранение такого пайплайна из редактора запрещено (данные не теряются).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"wedra/internal/pipeline"

	"gopkg.in/yaml.v3"
)

type editorInput struct {
	Name    string `json:"name"`
	Default string `json:"default"`
}

type editorFormField struct {
	Field    string `json:"field"`
	Editable bool   `json:"editable"`
}

// editorWhen — v0.27: условие шага (ядро: internal/pipeline/when.go,
// операторы WhenOps). Value — как ввёл пользователь (строка из UI),
// на выходе коэрсируется для числовых операторов (см. serialize).
type editorWhen struct {
	Path  string      `json:"path"`
	Op    string      `json:"op"`
	Value interface{} `json:"value,omitempty"`
}

type editorStep struct {
	ID       string            `json:"id"`
	Plugin   string            `json:"plugin"`
	Pos      [2]int            `json:"pos"`
	OnError  string            `json:"on_error"`
	Timeout  string            `json:"timeout"`
	Bind     map[string]string `json:"bind"`
	Form     []editorFormField `json:"form"`
	Actions  []string          `json:"actions"`
	OnReject string            `json:"on_reject"`
	When     *editorWhen       `json:"when,omitempty"`
}

type editorDoc struct {
	Name          string        `json:"name"`
	FormatVersion string        `json:"format_version"` // v0.26a: сохраняется из исходника (пусто = новое → 0.2)
	Input         []editorInput `json:"input"`
	Steps         []editorStep  `json:"steps"`
	Unsupported   []string      `json:"unsupported"`
}

// editorPosFile — теневой разбор только под позиции (ядро pos не знает).
type editorPosFile struct {
	Pipeline struct {
		Steps []struct {
			ID  string `yaml:"id"`
			Pos [2]int `yaml:"pos"`
		} `yaml:"steps"`
	} `yaml:"pipeline"`
}

func (s *Server) handleParsePipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST yaml", 405)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	pf, err := pipeline.LoadPipelineFileFromBytes(data)
	if err != nil {
		http.Error(w, "parse: "+err.Error(), 400)
		return
	}
	pos := map[string][2]int{}
	var posFile editorPosFile
	if err := yaml.Unmarshal(data, &posFile); err == nil {
		for _, st := range posFile.Pipeline.Steps {
			pos[st.ID] = st.Pos
		}
	}
	doc := editorDoc{
		Name:          pf.Pipeline.Name,
		FormatVersion: pf.FormatVersion,
		Input:         []editorInput{},
		Steps:         []editorStep{},
		Unsupported:   []string{},
	}
	names := make([]string, 0, len(pf.Pipeline.Input))
	for n := range pf.Pipeline.Input {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		v := pf.Pipeline.Input[n]
		switch v.(type) {
		case map[string]interface{}:
			doc.Unsupported = append(doc.Unsupported, "input."+n+" (type-объявление, не значение)")
			doc.Input = append(doc.Input, editorInput{Name: n, Default: ""})
		default:
			def := ""
			if v != nil {
				def = fmt.Sprint(v)
			}
			doc.Input = append(doc.Input, editorInput{Name: n, Default: def})
		}
	}
	for _, st := range pf.Pipeline.Steps {
		es := editorStep{
			ID:      st.ID,
			Plugin:  st.Plugin,
			Pos:     pos[st.ID],
			OnError: st.OnError,
			Bind:    map[string]string{},
		}
		if es.OnError == "" {
			es.OnError = "stop"
		}
		if st.Timeout.Duration > 0 {
			es.Timeout = st.Timeout.Duration.String()
		}
		for k, v := range st.Bind {
			es.Bind[k] = v
		}
		for _, f := range st.Form {
			es.Form = append(es.Form, editorFormField{Field: f.Field, Editable: f.Editable})
		}
		es.Actions = st.Actions
		es.OnReject = st.OnReject
		if st.When.IsSet() {
			es.When = &editorWhen{Path: st.When.Path, Op: st.When.Op, Value: st.When.Value}
		}
		// поля, которых редактор v0.27 не управляет
		if st.Foreach != "" {
			doc.Unsupported = append(doc.Unsupported, st.ID+": foreach")
		}
		if st.ParallelGroup != "" {
			doc.Unsupported = append(doc.Unsupported, st.ID+": parallel_group")
		}
		if st.AfterForeach {
			doc.Unsupported = append(doc.Unsupported, st.ID+": after_foreach")
		}
		if st.Retry != nil {
			doc.Unsupported = append(doc.Unsupported, st.ID+": retry")
		}
		doc.Steps = append(doc.Steps, es)
	}
	if pf.Pipeline.Foreach != "" {
		doc.Unsupported = append(doc.Unsupported, "pipeline.foreach")
	}
	if pf.Pipeline.Secrets != nil {
		doc.Unsupported = append(doc.Unsupported, "pipeline.secrets")
	}
	if pf.Pipeline.Network != "" {
		doc.Unsupported = append(doc.Unsupported, "pipeline.network")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doc)
}

// outStep — теневой вывод: pos в YAML, остальное — ровно по схеме.
type outStep struct {
	ID       string            `yaml:"id"`
	Plugin   string            `yaml:"plugin"`
	Pos      [2]int            `yaml:"pos"`
	Bind     map[string]string `yaml:"bind,omitempty"`
	OnError  string            `yaml:"on_error,omitempty"`
	Timeout  string            `yaml:"timeout,omitempty"`
	Form     []editorFormField `yaml:"form,omitempty"`
	Actions  []string          `yaml:"actions,omitempty"`
	OnReject string            `yaml:"on_reject,omitempty"`
	When     *outWhen          `yaml:"when,omitempty"`
}

type outWhen struct {
	Path  string      `yaml:"path"`
	Op    string      `yaml:"op"`
	Value interface{} `yaml:"value,omitempty"`
}

type outFile struct {
	FormatVersion string `yaml:"format_version"`
	Pipeline      struct {
		Name  string         `yaml:"name"`
		Input map[string]any `yaml:"input,omitempty"`
		Steps []outStep      `yaml:"steps"`
	} `yaml:"pipeline"`
}

func (s *Server) handleSerializePipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST editor-doc json", 405)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var doc editorDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		http.Error(w, "json: "+err.Error(), 400)
		return
	}
	if len(doc.Unsupported) > 0 {
		http.Error(w, "пайплайн содержит поля, которые редактор не управляет: "+strings.Join(doc.Unsupported, ", ")+
			" — редактируй в YAML (вкладка Пайплайны), не из редактора", 409)
		return
	}
	// документ → схема ядра
	// v0.26a: версия формата — из исходного файла; новое doc (без версии) → текущая 0.2
	fv := doc.FormatVersion
	if fv == "" {
		fv = "0.2"
	}
	pf := pipeline.PipelineFile{
		FormatVersion: fv,
		Pipeline:      pipeline.Pipeline{Name: doc.Name, Input: map[string]interface{}{}, Steps: []pipeline.Step{}},
	}
	for _, in := range doc.Input {
		if in.Name == "" {
			continue
		}
		pf.Pipeline.Input[in.Name] = in.Default
	}
	for _, st := range doc.Steps {
		step := pipeline.Step{
			ID:      st.ID,
			Plugin:  st.Plugin,
			OnError: st.OnError,
			Bind:    map[string]string{},
		}
		if step.OnError == "" {
			step.OnError = "stop"
		}
		for k, v := range st.Bind {
			if k != "" && v != "" {
				step.Bind[k] = v
			}
		}
		if st.Timeout != "" {
			d, err := time.ParseDuration(st.Timeout)
			if err != nil {
				http.Error(w, "timeout "+st.ID+": "+err.Error(), 400)
				return
			}
			step.Timeout = pipeline.Duration{Duration: d}
		}
		for _, f := range st.Form {
			step.Form = append(step.Form, pipeline.FormField{Field: f.Field, Editable: f.Editable})
		}
		step.Actions = st.Actions
		step.OnReject = st.OnReject
		if st.When != nil {
			w := pipeline.When{Path: st.When.Path, Op: st.When.Op, Value: st.When.Value}
			if w.Op == "" {
				w.Op = "truthy"
			}
			// пустая строка из UI = «значения нет» — не выводим в YAML
			if t, ok := w.Value.(string); ok && t == "" {
				w.Value = nil
			}
			// UI шлёт value строкой; числовые операторы ядра требуют число
			if w.Op == "gt" || w.Op == "gte" || w.Op == "lt" || w.Op == "lte" {
				if t, ok := w.Value.(string); ok {
					if f, err := strconv.ParseFloat(t, 64); err == nil {
						w.Value = f
					}
				}
			}
			step.When = w
		}
		pf.Pipeline.Steps = append(pf.Pipeline.Steps, step)
	}
	// схема → YAML (через теневой вывод, чтобы pos прописался)
	out := outFile{FormatVersion: pf.FormatVersion}
	out.Pipeline.Name = pf.Pipeline.Name
	out.Pipeline.Input = map[string]any{}
	for k, v := range pf.Pipeline.Input {
		out.Pipeline.Input[k] = v
	}
	for _, st := range pf.Pipeline.Steps {
		os := outStep{ID: st.ID, Plugin: st.Plugin, Pos: docStepPos(doc, st.ID), OnError: st.OnError}
		if st.Timeout.Duration > 0 {
			os.Timeout = st.Timeout.Duration.String()
		}
		for k, v := range st.Bind {
			if os.Bind == nil {
				os.Bind = map[string]string{}
			}
			os.Bind[k] = v
		}
		for _, f := range st.Form {
			os.Form = append(os.Form, editorFormField{Field: f.Field, Editable: f.Editable})
		}
		os.Actions = st.Actions
		os.OnReject = st.OnReject
		if st.When.IsSet() {
			os.When = &outWhen{Path: st.When.Path, Op: st.When.Op, Value: st.When.Value}
		}
		out.Pipeline.Steps = append(out.Pipeline.Steps, os)
	}
	raw, err := yaml.Marshal(&out)
	if err != nil {
		http.Error(w, "yaml: "+err.Error(), 500)
		return
	}
	text := "# создан в редакторе WEDRA (v0.26); pos: — позиции узлов (ядро игнорирует)\n" + string(raw)
	// честность: сгенерированный YAML обязан читаться ядром и проходить валидацию
	check, err := pipeline.LoadPipelineFileFromBytes([]byte(text))
	if err != nil {
		http.Error(w, "генерированный YAML не читается: "+err.Error(), 500)
		return
	}
	errs, warns := pipeline.Validate(check, s.Engine)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"yaml": text, "errors": errs, "warnings": warns, "ok": len(errs) == 0,
	})
}

func docStepPos(doc editorDoc, id string) [2]int {
	for _, st := range doc.Steps {
		if st.ID == id {
			return st.Pos
		}
	}
	return [2]int{}
}
