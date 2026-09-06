package api

// v0.25: редактор пайплайнов — парсинг/сериализация через Go (JS тонкий,
// корректность YAML живёт в ядре, а не в браузере).
//
// Модель редактора (EditorDoc) — подмножество схемы: id/plugin/pos/bind/
// on_error/timeout + у гейта form/actions/on_reject + input-дефолты.
// Позиции узлов хранятся в YAML как `pos: [x, y]` — лоадер ядра это поле
// игнорирует (yaml.v3 без KnownFields), редактор читает обратно.
// v0.27: when — под управлением редактора (path/op/value, 10 операторов ядра).
// v0.28: foreach/parallel_group/after_foreach на шаге + foreach/foreach_item/
// item_type/item_format на пайплайне (управляющий поток целиком).
// v0.29: retry (on_error: retry + retry{attempts, delay, backoff}).
// v0.5: secrets (pipeline.secrets — env-ключи плагинов) под управлением
// редактора; кросс-чек «плагин ↔ пайплайн» — валидатор ядра (warnings).
// v0.6: network (pipeline.network — политика allow/deny) под управлением
// редактора; кросс-чек «плагин заявил сеть + deny» — ошибка валидатора.
// Осталось вне редактора: type-объявления в input — такие пайплайны
// выносятся в unsupported: сохранение из редактора запрещено
// (данные не теряются).

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
	// v0.28: управляющий поток шага
	Foreach       string `json:"foreach"`
	ForeachItem   string `json:"foreach_item"`
	AfterForeach  bool   `json:"after_foreach"`
	ParallelGroup string `json:"parallel_group"`
	// v0.29: retry (вместе с on_error: retry)
	Retry *editorRetry `json:"retry,omitempty"`
}

// editorRetry — v0.29: политика повторов (ядро: PROTOCOL §5: retry повторяет
// таймауты и доменные ошибки с retryable: true; исчерпанный retry = stop).
type editorRetry struct {
	Attempts int    `json:"attempts"`
	Delay    string `json:"delay"`   // "5s", "100ms", … (time.ParseDuration)
	Backoff  string `json:"backoff"` // fixed | exponential
}

type editorDoc struct {
	Name          string        `json:"name"`
	FormatVersion string        `json:"format_version"` // v0.26a: сохраняется из исходника (пусто = новое → 0.2)
	Input         []editorInput `json:"input"`
	Steps         []editorStep  `json:"steps"`
	// v0.5: env-ключи, которые ядро передаст плагинам (кросс-чек с
	// permissions.secrets манифестов — валидатор, warnings)
	Secrets []string `json:"secrets"`
	// v0.6: сетевая политика — "" (allow, дефолт: плагин видит
	// WEDRA_NETWORK=allow и сам декларирует сеть в манифесте) или "deny"
	// (шаг с заявленной сетью — ошибка валидатора и раннера)
	Network     string   `json:"network"`
	Unsupported []string `json:"unsupported"`
	// v0.28: управляющий поток пайплайна (батч по массиву)
	Foreach     string `json:"foreach"`
	ForeachItem string `json:"foreach_item"`
	ItemType    string `json:"item_type"`
	ItemFormat  string `json:"item_format"`
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
		Foreach:       pf.Pipeline.Foreach,
		ForeachItem:   pf.Pipeline.ForeachItem,
		ItemType:      pf.Pipeline.ItemType,
		ItemFormat:    pf.Pipeline.ItemFormat,
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
		es.Foreach = st.Foreach
		es.ForeachItem = st.ForeachItem
		es.AfterForeach = st.AfterForeach
		es.ParallelGroup = st.ParallelGroup
		if st.Retry != nil {
			re := &editorRetry{Attempts: st.Retry.Attempts, Backoff: st.Retry.Backoff}
			if st.Retry.Delay.Duration > 0 {
				re.Delay = st.Retry.Delay.Duration.String()
			}
			es.Retry = re
		}
		doc.Steps = append(doc.Steps, es)
	}
	// v0.5: secrets под управлением редактора (was: unsupported)
	if pf.Pipeline.Secrets != nil {
		doc.Secrets = pf.Pipeline.Secrets
	} else {
		doc.Secrets = []string{}
	}
	// v0.6: network под управлением редактора (was: unsupported)
	doc.Network = pf.Pipeline.Network
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
	// v0.28: управляющий поток шага
	Foreach       string    `yaml:"foreach,omitempty"`
	ForeachItem   string    `yaml:"foreach_item,omitempty"`
	AfterForeach  bool      `yaml:"after_foreach,omitempty"`
	ParallelGroup string    `yaml:"parallel_group,omitempty"`
	Retry         *outRetry `yaml:"retry,omitempty"`
}

type outRetry struct {
	Attempts int    `yaml:"attempts"`
	Delay    string `yaml:"delay,omitempty"`
	Backoff  string `yaml:"backoff,omitempty"`
}

type outWhen struct {
	Path  string      `yaml:"path"`
	Op    string      `yaml:"op"`
	Value interface{} `yaml:"value,omitempty"`
}

type outFile struct {
	FormatVersion string `yaml:"format_version"`
	Pipeline      struct {
		Name        string         `yaml:"name"`
		Input       map[string]any `yaml:"input,omitempty"`
		Steps       []outStep      `yaml:"steps"`
		Secrets     []string       `yaml:"secrets,omitempty"` // v0.5
		Network     string         `yaml:"network,omitempty"` // v0.6
		Foreach     string         `yaml:"foreach,omitempty"`
		ForeachItem string         `yaml:"foreach_item,omitempty"`
		ItemType    string         `yaml:"item_type,omitempty"`
		ItemFormat  string         `yaml:"item_format,omitempty"`
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
		Pipeline: pipeline.Pipeline{
			Name: doc.Name, Input: map[string]interface{}{}, Steps: []pipeline.Step{},
			Foreach: doc.Foreach, ForeachItem: doc.ForeachItem,
			ItemType: doc.ItemType, ItemFormat: doc.ItemFormat,
		},
	}
	for _, in := range doc.Input {
		if in.Name == "" {
			continue
		}
		pf.Pipeline.Input[in.Name] = in.Default
	}
	// v0.5: secrets из doc (пустые строки UI не валиден — отбрасываем)
	for _, k := range doc.Secrets {
		if key := strings.TrimSpace(k); key != "" {
			pf.Pipeline.Secrets = append(pf.Pipeline.Secrets, key)
		}
	}
	// v0.6: network — только две политики (раннер: != "deny" = allow)
	if strings.TrimSpace(doc.Network) == "deny" {
		pf.Pipeline.Network = "deny"
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
		step.Foreach = st.Foreach
		step.ForeachItem = st.ForeachItem
		step.AfterForeach = st.AfterForeach
		step.ParallelGroup = st.ParallelGroup
		if st.Retry != nil {
			r := &pipeline.Retry{Attempts: st.Retry.Attempts, Backoff: st.Retry.Backoff}
			if st.Retry.Delay != "" {
				d, err := time.ParseDuration(st.Retry.Delay)
				if err != nil {
					http.Error(w, "retry "+st.ID+": delay: "+err.Error(), 400)
					return
				}
				r.Delay = pipeline.Duration{Duration: d}
			}
			step.Retry = r
		}
		pf.Pipeline.Steps = append(pf.Pipeline.Steps, step)
	}
	// схема → YAML (через теневой вывод, чтобы pos прописался)
	out := outFile{FormatVersion: pf.FormatVersion}
	out.Pipeline.Name = pf.Pipeline.Name
	out.Pipeline.Foreach = pf.Pipeline.Foreach
	out.Pipeline.ForeachItem = pf.Pipeline.ForeachItem
	out.Pipeline.ItemType = pf.Pipeline.ItemType
	out.Pipeline.ItemFormat = pf.Pipeline.ItemFormat
	out.Pipeline.Input = map[string]any{}
	for k, v := range pf.Pipeline.Input {
		out.Pipeline.Input[k] = v
	}
	if len(pf.Pipeline.Secrets) > 0 {
		out.Pipeline.Secrets = pf.Pipeline.Secrets
	}
	out.Pipeline.Network = pf.Pipeline.Network // omitempty: allow = поля нет
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
		os.Foreach = st.Foreach
		os.ForeachItem = st.ForeachItem
		os.AfterForeach = st.AfterForeach
		os.ParallelGroup = st.ParallelGroup
		if st.Retry != nil {
			os.Retry = &outRetry{Attempts: st.Retry.Attempts, Delay: st.Retry.Delay.Duration.String(), Backoff: st.Retry.Backoff}
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
