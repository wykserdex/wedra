package api

// v0.25 (v0.26a: format_version; v0.27: when; v0.28: foreach/parallel/
// after_foreach): round-trip parse → doc → serialize → YAML обязан читаться
// ядром и проходить валидацию; pos возвращается; unsupported блокирует
// serialize; конфликты управляющего потока — честно в errors.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wedra/internal/pipeline"
	"wedra/internal/plugin"
)

func postBytes(t *testing.T, url string, body []byte) (int, map[string]interface{}) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func fileToBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEditorParseSingleCheck(t *testing.T) {
	ts, _ := gateTestServer(t)
	code, doc := postBytes(t, ts.URL+"/api/parse/pipeline", fileToBytes(t, "../../examples/single_check.yaml"))
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, doc)
	}
	if doc["name"] != "single_check" {
		t.Fatalf("name = %v", doc["name"])
	}
	if doc["format_version"] != "0.2" {
		t.Fatalf("format_version = %v (single_check — 0.2)", doc["format_version"])
	}
	steps, _ := doc["steps"].([]interface{})
	if len(steps) != 3 {
		t.Fatalf("steps = %d", len(steps))
	}
	gate, _ := steps[2].(map[string]interface{})
	if gate["plugin"] != "core/human_gate" {
		t.Fatalf("gate plugin = %v", gate["plugin"])
	}
	actions, _ := gate["actions"].([]interface{})
	if len(actions) != 2 {
		t.Fatalf("actions = %v", gate["actions"])
	}
	form, _ := gate["form"].([]interface{})
	if len(form) != 2 {
		t.Fatalf("form = %v", gate["form"])
	}
	unsup, _ := doc["unsupported"].([]interface{})
	if len(unsup) != 0 {
		t.Fatalf("unsupported = %v (single_check — чистый для редактора)", unsup)
	}
	inputs, _ := doc["input"].([]interface{})
	if len(inputs) != 1 {
		t.Fatalf("input = %v", inputs)
	}
	if inputs[0].(map[string]interface{})["default"] != "user@mailinator.com" {
		t.Fatalf("input default = %v", inputs[0])
	}
}

func TestEditorParseUnsupported(t *testing.T) {
	ts, _ := gateTestServer(t)
	raw := []byte(`format_version: "0.1"
pipeline:
  name: tricky
  input:
    n: { type: number, required: true }
  steps:
    - id: a
      plugin: core/human_gate
      when:
        path: input.n
        op: ">"
        value: 10
      actions: [accept]
      retry:
        attempts: 3
`)
	code, doc := postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, doc)
	}
	unsup, _ := doc["unsupported"].([]interface{})
	text := ""
	for _, u := range unsup {
		text += u.(string) + " "
	}
	// v0.27/v0.28: when и parallel_group под управлением редактора —
	// в unsupported не попадают
	for _, gone := range []string{"a: when", "a: parallel_group"} {
		if strings.Contains(text, gone) {
			t.Fatalf("unsupported = %q (%s больше не unsupported)", text, gone)
		}
	}
	if !strings.Contains(text, "a: retry") || !strings.Contains(text, "input.n") {
		t.Fatalf("unsupported = %q (ждём retry, input.n)", text)
	}
	// serialize такого — 409 (редактор не управляет полями → не терять их)
	d, _ := json.Marshal(doc)
	code, body := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
	if code != 409 {
		t.Fatalf("serialize unsupported: code=%d body=%v (want 409)", code, body)
	}
}

func TestEditorSerializeRoundTrip(t *testing.T) {
	ts, _ := gateTestServer(t)
	raw := fileToBytes(t, "../../examples/gate_demo.yaml")
	_, doc := postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	// назначаем позиции (редактор бы их поставил мышкой)
	for _, st := range doc["steps"].([]interface{}) {
		st.(map[string]interface{})["pos"] = []interface{}{float64(120), float64(80)}
	}
	d, _ := json.Marshal(doc)
	code, out := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, out)
	}
	if out["ok"] != true {
		t.Fatalf("ok=%v errors=%v", out["ok"], out["errors"])
	}
	yamlText, _ := out["yaml"].(string)
	if !strings.Contains(yamlText, "pos:") || !strings.Contains(yamlText, "120") {
		t.Fatalf("yaml без pos:\n%s", yamlText)
	}
	// v0.26a: версия исходного файла (0.1) сохраняется, а не подменяется
	if doc["format_version"] != "0.1" || !strings.Contains(yamlText, `format_version: "0.1"`) {
		t.Fatalf("round-trip потерял format_version (doc=%v):\n%s", doc["format_version"], yamlText)
	}
	// сгенерированный YAML обязан читаться ядром
	pf, err := pipeline.LoadPipelineFileFromBytes([]byte(yamlText))
	if err != nil {
		t.Fatalf("ядро не читает свой же YAML: %v", err)
	}
	errs, _ := pipeline.Validate(pf, plugin.NewEngine())
	if len(errs) > 0 {
		t.Fatalf("валидация: %v", errs)
	}
	// ре-парсинг: pos вернулся
	_, doc2 := postBytes(t, ts.URL+"/api/parse/pipeline", []byte(yamlText))
	st2 := doc2["steps"].([]interface{})[0].(map[string]interface{})
	pos, _ := st2["pos"].([]interface{})
	if len(pos) != 2 || pos[0] != float64(120) || pos[1] != float64(80) {
		t.Fatalf("pos после round-trip = %v", st2["pos"])
	}
}

func TestEditorWhenParse(t *testing.T) {
	// v0.27: when_demo — when теперь под управлением редактора (parse не
	// требует манифестов — только структура)
	ts, _ := gateTestServer(t)
	raw := fileToBytes(t, "../../examples/when_demo.yaml")
	code, doc := postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, doc)
	}
	unsup, _ := doc["unsupported"].([]interface{})
	if len(unsup) != 0 {
		t.Fatalf("unsupported = %v (when_demo теперь редакторский)", unsup)
	}
	steps := doc["steps"].([]interface{})
	w, _ := steps[1].(map[string]interface{})["when"].(map[string]interface{})
	if w == nil || w["path"] != "steps.stats.words" || w["op"] != "gte" || w["value"] != float64(10) {
		t.Fatalf("when = %v (ждём {steps.stats.words gte 10})", w)
	}
}

func TestEditorWhenRoundTrip(t *testing.T) {
	// v0.27: полный цикл parse → serialize → ядро читает+валидирует → re-parse.
	// Синтетика из core/human_gate: в тестовом окружении нет plugins/ —
	// builtin гейт манифест не требует.
	ts, _ := gateTestServer(t)
	raw := []byte(`format_version: "0.2"
pipeline:
  name: when_test
  input:
    n: 5
  steps:
    - id: a
      plugin: core/human_gate
      form:
        - field: input.n
          editable: true
      actions: [accept]

    - id: b
      plugin: core/human_gate
      when: { path: steps.a.n, op: gte, value: 10 }
      form:
        - field: input.n
          editable: false
      actions: [accept]
`)
	code, doc := postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, doc)
	}
	d, _ := json.Marshal(doc)
	code, out := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, out)
	}
	if out["ok"] != true {
		t.Fatalf("ok=%v errors=%v", out["ok"], out["errors"])
	}
	yamlText, _ := out["yaml"].(string)
	if !strings.Contains(yamlText, "when:") || !strings.Contains(yamlText, "steps.a.n") ||
		!strings.Contains(yamlText, "op: gte") || !strings.Contains(yamlText, "value: 10") {
		t.Fatalf("yaml без when:\n%s", yamlText)
	}
	// ядро читает и валидирует сгенерированный YAML (when-проверки валидатора
	// проходят: шаги в порядке, op из WhenOps)
	pf, err := pipeline.LoadPipelineFileFromBytes([]byte(yamlText))
	if err != nil {
		t.Fatalf("ядро не читает свой же YAML: %v", err)
	}
	errs, _ := pipeline.Validate(pf, plugin.NewEngine())
	if len(errs) > 0 {
		t.Fatalf("валидация: %v", errs)
	}
	// re-parse: when вернулся с теми же параметрами
	_, doc2 := postBytes(t, ts.URL+"/api/parse/pipeline", []byte(yamlText))
	w2, _ := doc2["steps"].([]interface{})[1].(map[string]interface{})["when"].(map[string]interface{})
	if w2 == nil || w2["path"] != "steps.a.n" || w2["op"] != "gte" || w2["value"] != float64(10) {
		t.Fatalf("when после round-trip = %v", w2)
	}
}

func TestEditorWhenTruthyAndCoerce(t *testing.T) {
	// v0.27: новое doc с when — truthy без value; и коэрсия строки «5» в число
	// для gt (UI шлёт value текстом)
	ts, _ := gateTestServer(t)
	doc := map[string]interface{}{
		"name":  "when_fresh",
		"input": []interface{}{map[string]interface{}{"name": "n", "default": "1"}},
		"steps": []interface{}{
			map[string]interface{}{
				"id": "a", "plugin": "core/human_gate", "pos": []interface{}{0.0, 0.0},
				"on_error": "stop", "form": []interface{}{}, "actions": []interface{}{"accept"},
				"on_reject": "stop",
				"when":      map[string]interface{}{"path": "input.n", "op": "truthy"},
			},
			map[string]interface{}{
				"id": "b", "plugin": "core/human_gate", "pos": []interface{}{0.0, 40.0},
				"on_error": "stop", "form": []interface{}{}, "actions": []interface{}{"accept"},
				"on_reject": "stop",
				"when":      map[string]interface{}{"path": "input.n", "op": "gt", "value": "5"},
			},
		},
		"unsupported": []interface{}{},
	}
	d, _ := json.Marshal(doc)
	code, out := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, out)
	}
	if out["ok"] != true {
		t.Fatalf("ok=%v errors=%v", out["ok"], out["errors"])
	}
	yamlText, _ := out["yaml"].(string)
	if !strings.Contains(yamlText, "op: truthy") {
		t.Fatalf("truthy потерялся:\n%s", yamlText)
	}
	// gt: «5» (строка из UI) обязано стать 5 (число) — иначе ядро на рантайме
	// не сможет сравнить
	if strings.Contains(yamlText, `value: "5"`) || !strings.Contains(yamlText, "value: 5") {
		t.Fatalf("коэрсия «5» → 5 не сработала:\n%s", yamlText)
	}
	pf, err := pipeline.LoadPipelineFileFromBytes([]byte(yamlText))
	if err != nil {
		t.Fatalf("ядро не читает: %v", err)
	}
	errs, _ := pipeline.Validate(pf, plugin.NewEngine())
	if len(errs) > 0 {
		t.Fatalf("валидация: %v", errs)
	}
}

// ── v0.28: foreach / parallel_group / after_foreach ─────────────────────────

// flowFixturePlugin — минимальный манифест (без бинаря: валидация манифест
// читает, исполнять не нужно).
const flowFixturePlugin = `id: test-fixture
version: 0.1.0
platform_api: "^0.1"
runtime:
  type: python
  entry: main.py
  requires: []
input:
  item:
    from: input.item
    type: string
    format: text
output:
  done: { type: boolean }
permissions:
  network: []
  filesystem: none
  secrets: []
`

// flowTestServer — gateTestServer + фикстурный плагин (для валидации
// step-foreach/parallel: в tempdir нет plugins/).
func flowTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	pipelines := filepath.Join(dir, "pipelines")
	runs := filepath.Join(dir, "runs")
	for _, d := range []string{plugins, pipelines, runs} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(plugins, "test-fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugins, "test-fixture", "plugin.yaml"), []byte(flowFixturePlugin), 0644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(plugins, pipelines, runs)
	srv.Engine.PluginsDir = plugins // реестровые имена (без plugins/) → tempdir
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, runs
}

func TestEditorForeachParse(t *testing.T) {
	// v0.28: реальные примеры — parse вытаскивает управляющий поток,
	// unsupported пуст (все три файла редакторские)
	ts, _ := gateTestServer(t)

	raw := fileToBytes(t, "../../examples/foreach_step_demo.yaml")
	_, doc := postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	if unsup, _ := doc["unsupported"].([]interface{}); len(unsup) != 0 {
		t.Fatalf("foreach_step_demo: unsupported = %v", unsup)
	}
	fx := doc["steps"].([]interface{})[0].(map[string]interface{})
	if fx["foreach"] != "input.texts" || fx["foreach_item"] != "text" {
		t.Fatalf("freqs = %v (ждём foreach input.texts, item text)", fx)
	}

	raw = fileToBytes(t, "../../examples/parallel_demo.yaml")
	_, doc = postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	// input.data — объект: по честному скоупу редактора это type-объявление
	// (модель input = name+default), но parallel_group должен быть в doc
	if unsup, _ := doc["unsupported"].([]interface{}); len(unsup) != 1 ||
		unsup[0].(string) != "input.data (type-объявление, не значение)" {
		t.Fatalf("parallel_demo: unsupported = %v (ждём только input.data)", unsup)
	}
	w := doc["steps"].([]interface{})[0].(map[string]interface{})
	if w["parallel_group"] != "analyze" {
		t.Fatalf("words.parallel_group = %v", w["parallel_group"])
	}

	raw = fileToBytes(t, "../../examples/csv_foreach_summary.yaml")
	_, doc = postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	if unsup, _ := doc["unsupported"].([]interface{}); len(unsup) != 0 {
		t.Fatalf("csv_foreach_summary: unsupported = %v", unsup)
	}
	if doc["foreach"] != "steps.load.rows" || doc["foreach_item"] != "row" || doc["item_type"] != "object" {
		t.Fatalf("pipeline-foreach = %v/%v/%v", doc["foreach"], doc["foreach_item"], doc["item_type"])
	}
	last := doc["steps"].([]interface{})
	sum := last[len(last)-1].(map[string]interface{})
	if !sum["after_foreach"].(bool) {
		t.Fatalf("summary.after_foreach = %v (ждём true)", sum["after_foreach"])
	}
}

func TestEditorForeachPipelineRoundTrip(t *testing.T) {
	// v0.28: pipeline-foreach + after_foreach на гейтах — полный цикл:
	// parse → serialize → ядро читает+валидирует → re-parse (гейты — builtin,
	// манифесты не нужны)
	ts, _ := gateTestServer(t)
	raw := []byte(`format_version: "0.2"
pipeline:
  name: foreach_test
  input:
    list: ["a", "b"]
  foreach: input.list
  foreach_item: row
  item_type: string
  steps:
    - id: per
      plugin: core/human_gate
      form:
        - field: input.row
          editable: false
      actions: [accept]

    - id: sum
      plugin: core/human_gate
      after_foreach: true
      form:
        - field: input.row
          editable: false
      actions: [accept]
`)
	code, doc := postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, doc)
	}
	d, _ := json.Marshal(doc)
	code, out := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, out)
	}
	if out["ok"] != true {
		t.Fatalf("ok=%v errors=%v", out["ok"], out["errors"])
	}
	yamlText, _ := out["yaml"].(string)
	for _, want := range []string{"foreach: input.list", "foreach_item: row", "item_type: string", "after_foreach: true"} {
		if !strings.Contains(yamlText, want) {
			t.Fatalf("yaml без %q:\n%s", want, yamlText)
		}
	}
	pf, err := pipeline.LoadPipelineFileFromBytes([]byte(yamlText))
	if err != nil {
		t.Fatalf("ядро не читает свой же YAML: %v", err)
	}
	errs, _ := pipeline.Validate(pf, plugin.NewEngine())
	if len(errs) > 0 {
		t.Fatalf("валидация: %v", errs)
	}
	// re-parse: всё вернулось
	_, doc2 := postBytes(t, ts.URL+"/api/parse/pipeline", []byte(yamlText))
	if doc2["foreach"] != "input.list" || doc2["foreach_item"] != "row" || doc2["item_type"] != "string" {
		t.Fatalf("pipeline-foreach после round-trip: %v", doc2)
	}
	sum2 := doc2["steps"].([]interface{})[1].(map[string]interface{})
	if !sum2["after_foreach"].(bool) {
		t.Fatalf("after_foreach после round-trip: %v", sum2)
	}
}

func TestEditorStepForeachRoundTrip(t *testing.T) {
	// v0.28: step-foreach + parallel_group на фикстурном плагине — полный
	// цикл с валидацией (существует ли плагин, пути, конфликты)
	ts, runs := flowTestServer(t)
	raw := []byte(`format_version: "0.2"
pipeline:
  name: step_foreach_test
  input:
    texts: ["a", "b"]
    x: "hi"
  steps:
    - id: fx
      plugin: test-fixture
      foreach: input.texts
      foreach_item: item
      on_error: stop

    - id: par
      plugin: test-fixture
      parallel_group: g1
      on_error: stop
      bind:
        item: input.x
`)
	code, doc := postBytes(t, ts.URL+"/api/parse/pipeline", raw)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, doc)
	}
	if unsup, _ := doc["unsupported"].([]interface{}); len(unsup) != 0 {
		t.Fatalf("unsupported = %v", unsup)
	}
	fx := doc["steps"].([]interface{})[0].(map[string]interface{})
	if fx["foreach"] != "input.texts" || fx["foreach_item"] != "item" {
		t.Fatalf("fx = %v", fx)
	}
	d, _ := json.Marshal(doc)
	code, out := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, out)
	}
	if out["ok"] != true {
		t.Fatalf("ok=%v errors=%v", out["ok"], out["errors"])
	}
	yamlText, _ := out["yaml"].(string)
	for _, want := range []string{"foreach: input.texts", "foreach_item: item", "parallel_group: g1"} {
		if !strings.Contains(yamlText, want) {
			t.Fatalf("yaml без %q:\n%s", want, yamlText)
		}
	}
	pf, err := pipeline.LoadPipelineFileFromBytes([]byte(yamlText))
	if err != nil {
		t.Fatalf("ядро не читает: %v", err)
	}
	eng := plugin.NewEngine()
	eng.PluginsDir = filepath.Join(filepath.Dir(runs), "plugins")
	errs, _ := pipeline.Validate(pf, eng)
	if len(errs) > 0 {
		t.Fatalf("валидация: %v", errs)
	}
}

func TestEditorForeachConflicts(t *testing.T) {
	// v0.28: конфликты валидатора проходят через редактор честно:
	// foreach + after_foreach и foreach + parallel_group → ok:false c ошибкой
	ts, _ := flowTestServer(t)
	mk := func(extra map[string]interface{}) {
		t.Helper()
		doc := map[string]interface{}{
			"name": "conflict", "input": []interface{}{map[string]interface{}{"name": "texts", "default": "a"}},
			"steps": []interface{}{map[string]interface{}{
				"id": "fx", "plugin": "test-fixture", "pos": []interface{}{0.0, 0.0},
				"on_error": "stop", "bind": map[string]interface{}{},
				"form": []interface{}{}, "actions": []interface{}{}, "on_reject": "",
				"foreach": "input.texts",
			}},
			"unsupported": []interface{}{},
		}
		step := doc["steps"].([]interface{})[0].(map[string]interface{})
		for k, v := range extra {
			step[k] = v
		}
		d, _ := json.Marshal(doc)
		code, out := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
		if code != 200 {
			t.Fatalf("code=%d body=%v", code, out)
		}
		if out["ok"] != false {
			t.Fatalf("конфликт %v не отловлен: ok=%v", extra, out)
		}
	}
	mk(map[string]interface{}{"after_foreach": true})
	mk(map[string]interface{}{"parallel_group": "g1"})
}

func TestEditorSerializeNewGatePipeline(t *testing.T) {
	ts, _ := gateTestServer(t)
	doc := map[string]interface{}{
		"name":  "fresh",
		"input": []interface{}{map[string]interface{}{"name": "note", "default": "hi"}},
		"steps": []interface{}{
			map[string]interface{}{
				"id": "review", "plugin": "core/human_gate", "pos": []interface{}{0.0, 0.0},
				"on_error":  "stop",
				"form":      []interface{}{map[string]interface{}{"field": "input.note", "editable": true}},
				"actions":   []interface{}{"accept", "reject"},
				"on_reject": "stop",
			},
		},
		"unsupported": []interface{}{},
	}
	d, _ := json.Marshal(doc)
	code, out := postBytes(t, ts.URL+"/api/serialize/pipeline", d)
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, out)
	}
	if out["ok"] != true {
		t.Fatalf("ok=%v errors=%v", out["ok"], out["errors"])
	}
	yamlText, _ := out["yaml"].(string)
	if !strings.Contains(yamlText, "core/human_gate") || !strings.Contains(yamlText, "on_reject: stop") {
		t.Fatalf("yaml:\n%s", yamlText)
	}
	// v0.26a: новое doc без версии → текущая 0.2
	if !strings.Contains(yamlText, `format_version: "0.2"`) {
		t.Fatalf("новому doc не присвоено 0.2:\n%s", yamlText)
	}
}
