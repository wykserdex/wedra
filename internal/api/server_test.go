package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const badPipeYAML = `format_version: "0.2"
pipeline:
  name: bad_demo
  input:
    email: "a@b.c"
  steps:
    - id: s
      plugin: fake/nope
      bind:
        email: input.nope
`

func TestAPIRunInvalid400Direct(t *testing.T) {
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	pipelines := filepath.Join(dir, "pipelines")
	runs := filepath.Join(dir, "runs")
	for _, d := range []string{plugins, pipelines, runs} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pipelines, "bad.yaml"), []byte(badPipeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelines, "gate_demo.yaml"), []byte(gatePipeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(plugins, pipelines, runs)
	ts := newTestServer(t, srv)

	before := listDir(t, runs)
	code, body := postJSON(t, ts.URL+"/api/run", map[string]interface{}{"file": "bad.yaml", "yes": true})
	if code != 400 {
		t.Fatalf("POST bad.yaml: code=%d body=%v (want 400)", code, body)
	}
	if _, hasRun := body["run"]; hasRun {
		t.Fatalf("400 не должен выдавать run_id: %v", body)
	}
	after := listDir(t, runs)
	if len(before) != len(after) {
		t.Fatalf("папка ранов изменилась на 400: до=%v после=%v", before, after)
	}
	// валидный — 202
	code, start := postJSON(t, ts.URL+"/api/run", map[string]interface{}{"file": "gate_demo.yaml", "yes": true})
	if code != 202 {
		t.Fatalf("POST gate_demo: code=%d body=%v (want 202)", code, start)
	}
	if _, ok := start["run"]; !ok {
		t.Fatalf("202 должен выдавать run: %v", start)
	}
	waitRunStatus(t, ts, start["run"].(string), "ok")
	nestedDir := filepath.Join(pipelines, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "gate_demo.yaml"), []byte(gatePipeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	code, nested := postJSON(t, ts.URL+"/api/run", map[string]interface{}{"file": filepath.Join("nested", "gate_demo.yaml"), "yes": true})
	if code != 202 {
		t.Fatalf("POST nested gate_demo: code=%d body=%v", code, nested)
	}
	waitRunStatus(t, ts, nested["run"].(string), "ok")
}

func TestValidatePipelineJSONUsesCanonicalFields(t *testing.T) {
	ts, _ := gateTestServer(t)
	code, out := postJSON(t, ts.URL+"/api/validate/pipeline", map[string]interface{}{
		"format_version": "0.2",
		"pipeline": map[string]interface{}{
			"name": "json_fields", "input": map[string]interface{}{"note": "x"},
			"steps": []interface{}{map[string]interface{}{
				"id": "gate", "plugin": "core/human_gate", "parallel_group": "p",
				"actions": []interface{}{"accept"},
			}},
		},
	})
	if code != 200 {
		t.Fatalf("code=%d body=%v", code, out)
	}
	if out["ok"] != false {
		t.Fatalf("canonical JSON fields were ignored: %v", out)
	}
}

func newTestServer(t *testing.T, srv *Server) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
