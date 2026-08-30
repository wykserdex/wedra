package api

// v0.25 (v0.26a: format_version): редактор — round-trip parse → doc → serialize → YAML обязан
// читаться ядром и проходить валидацию; pos возвращается; unsupported
// блокирует serialize.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
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
      parallel_group: g1
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
	// v0.27: when теперь под управлением редактора — в unsupported не попадает
	if strings.Contains(text, "a: when") {
		t.Fatalf("unsupported = %q (when больше не unsupported)", text)
	}
	if !strings.Contains(text, "a: parallel_group") || !strings.Contains(text, "input.n") {
		t.Fatalf("unsupported = %q (ждём parallel_group, input.n)", text)
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
