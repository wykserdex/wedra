package api

// v0.25: редактор — round-trip parse → doc → serialize → YAML обязан
// читаться ядром и проходить валидацию; pos возвращается; unsupported
// блокирует serialize.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"orchestrator/internal/pipeline"
	"orchestrator/internal/plugin"
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
	if !strings.Contains(text, "a: when") || !strings.Contains(text, "a: parallel_group") ||
		!strings.Contains(text, "input.n") {
		t.Fatalf("unsupported = %q (ждём when, parallel_group, input.n)", text)
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
}
