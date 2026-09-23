package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const sleeperPipeYAML = `format_version: "0.2"
pipeline:
  name: sleep_demo
  input: {}
  steps:
    - id: sleep
      plugin: PLUGIN_DIR
      timeout: 30s
`

func writeSleeperPlugin(t *testing.T, dir string) string {
	t.Helper()
	d := filepath.Join(dir, "sleeper")
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{
		"id": "sleeper", "version": "0.1", "platform_api": "0.1",
		"runtime": map[string]interface{}{"type": "python", "entry": "main.py"},
		"input":   map[string]interface{}{},
		"output":  map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(d, "plugin.yaml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	py := "import sys,json,time\njson.load(sys.stdin)\ntime.sleep(30)\njson.dump({'status':'ok','output':{'done':True}},sys.stdout)\n"
	if err := os.WriteFile(filepath.Join(d, "main.py"), []byte(py), 0644); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAPIRunCancel(t *testing.T) {
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	pipelines := filepath.Join(dir, "pipelines")
	runs := filepath.Join(dir, "runs")
	for _, d := range []string{plugins, pipelines, runs} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	sleeperDir := writeSleeperPlugin(t, plugins)
	_ = sleeperDir
	pipeYAML := `format_version: "0.2"
pipeline:
  name: sleep_demo
  input: {}
  steps:
    - id: sleep
      plugin: sleeper
      timeout: 30s
`
	if err := os.WriteFile(filepath.Join(pipelines, "sleep.yaml"), []byte(pipeYAML), 0644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(plugins, pipelines, runs)
	ts := newTestServer(t, srv)

	code, start := postJSON(t, ts.URL+"/api/run", map[string]interface{}{"file": "sleep.yaml", "yes": true})
	if code != 202 {
		t.Fatalf("POST /api/run: code=%d body=%v", code, start)
	}
	runID, _ := start["run"].(string)
	time.Sleep(500 * time.Millisecond)

	// отмена
	code, body := postJSON(t, ts.URL+"/api/runs/"+runID+"/cancel", map[string]interface{}{})
	if code != 202 {
		t.Fatalf("POST cancel: code=%d body=%v (want 202)", code, body)
	}
	// ждём cancelled (не failed/timeout)
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, d := getJSON(t, ts.URL+"/api/runs/"+runID)
		if d["status"] == "cancelled" {
			return
		}
		if d["status"] == "failed" {
			t.Fatalf("статус failed вместо cancelled: %v", d)
		}
		if time.Now().After(deadline) {
			t.Fatalf("нет cancelled за 15с: %v", d)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
