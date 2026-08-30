package api

// v0.25: редактор пайплайнов — парсинг/сериализация через Go (JS тонкий,
// корректность YAML живёт в ядре, а не в браузере).
//
// Модель редактора (EditorDoc) — подмножество схемы: id/plugin/pos/bind/
// on_error/timeout + у гейта form/actions/on_reject + input-дефолты.
// Позиции узлов хранятся в YAML как `pos: [x, y]` — лоадер ядра это поле
// игнорирует (yaml.v3 без KnownFields), редактор читает обратно.
// Всё, чего редактор не управляет (when, foreach, parallel_group, retry,
// secrets, network, type-объявления в input) — выносится в unsupported:
// сохранение такого пайплайна из редактора запрещено (данные не теряются).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"orchestrator/internal/pipeline"

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
}

type editorDoc struct {
	Name        string        `json:"name"`
	Input       []editorInput `json:"input"`
	Steps       []editorStep  `json:"steps"`
	Unsupported []string      `json:"unsupported"`
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
		Name:        pf.Pipeline.Name,
		Input:       []editorInput{},
		Steps:       []editorStep{},
		Unsupported: []string{},
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
		// поля, которых редактор v0.25 не управляет
		if st.When.IsSet() {
			doc.Unsupported = append(doc.Unsupported, st.ID+": when")
		}
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
	pf := pipeline.PipelineFile{
		FormatVersion: "0.1",
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
		out.Pipeline.Steps = append(out.Pipeline.Steps, os)
	}
	raw, err := yaml.Marshal(&out)
	if err != nil {
		http.Error(w, "yaml: "+err.Error(), 500)
		return
	}
	text := "# создан в редакторе orchestrator (v0.25); pos: — позиции узлов (ядро игнорирует)\n" + string(raw)
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
