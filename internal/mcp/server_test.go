package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFakePlugin(t *testing.T, dir, id string, input, output map[string]interface{}) {
	t.Helper()
	d := filepath.Join(dir, id)
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{
		"id": id, "version": "0.1", "platform_api": "0.1",
		"runtime": map[string]interface{}{"type": "python", "entry": "main.py"},
		"input":   input, "output": output,
	}
	raw, _ := json.Marshal(manifest)
	// plugin.yaml нужен в YAML, но JSON — валидный YAML тоже
	if err := os.WriteFile(filepath.Join(d, "plugin.yaml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	py := "import sys,json\njson.load(sys.stdin)\njson.dump({'status':'ok','output':{'done':True}},sys.stdout)\n"
	if err := os.WriteFile(filepath.Join(d, "main.py"), []byte(py), 0644); err != nil {
		t.Fatal(err)
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	work := filepath.Join(dir, "work")
	os.MkdirAll(plugins, 0755)
	os.MkdirAll(work, 0755)
	writeFakePlugin(t, plugins, "echoer",
		map[string]interface{}{"text": map[string]interface{}{"type": "string", "from": "input.text"}},
		map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}})
	srv, err := NewServer(Options{PluginsDirs: []string{plugins}, WorkDir: work})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestMCPValidatePortSource(t *testing.T) {
	srv := testServer(t)
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: bad\n  input:\n    email: \"a@b.c\"\n  steps:\n    - id: s\n      plugin: " + srv.pluginsDirs[0] + "/echoer\n      bind:\n        text: input.nope\n"
	res, isErr, rpcErr := srv.callTool("validate_pipeline", map[string]interface{}{"yaml": yamlStr})
	if rpcErr != nil {
		t.Fatalf("rpc: %v", rpcErr)
	}
	if isErr {
		t.Fatalf("isError: %s", res)
	}
	var out struct {
		OK     bool `json:"ok"`
		Issues []struct {
			Code string `json:"code"`
		} `json:"issues"`
	}
	_ = json.Unmarshal([]byte(res), &out)
	if out.OK {
		t.Fatalf("want ok:false, got %s", res)
	}
	if len(out.Issues) == 0 || out.Issues[0].Code != "E_PORT_SOURCE" {
		t.Fatalf("want issues[0].code==E_PORT_SOURCE, got %s", res)
	}
}

func TestMCPToolsListAndRun(t *testing.T) {
	srv := testServer(t)
	res, _, rpcErr := srv.callTool("list_plugins", map[string]interface{}{})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if !strings.Contains(res, "echoer") {
		t.Fatalf("list_plugins без echoer: %s", res)
	}
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: simple\n  input:\n    text: \"hello\"\n  steps:\n    - id: s\n      plugin: " + srv.pluginsDirs[0] + "/echoer\n      bind:\n        text: input.text\n"
	res, _, rpcErr = srv.callTool("run_pipeline", map[string]interface{}{"yaml": yamlStr, "wait_seconds": 15.0})
	if rpcErr != nil {
		t.Fatalf("run: %v", rpcErr)
	}
	var runOut struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal([]byte(res), &runOut)
	if runOut.RunID == "" {
		t.Fatalf("нет run_id: %s", res)
	}
	// дождаться done через get_run
	deadline := time.Now().Add(20 * time.Second)
	for {
		res2, _, rpcErr := srv.callTool("get_run", map[string]interface{}{"run_id": runOut.RunID})
		if rpcErr != nil {
			t.Fatal(rpcErr)
		}
		var g struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal([]byte(res2), &g)
		if g.Status == "done" {
			return
		}
		if g.Status == "failed" {
			t.Fatalf("ран failed: %s", res2)
		}
		if time.Now().After(deadline) {
			t.Fatalf("ран не done за 20c: %s", res2)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func TestMCPSandboxOutsideRoot(t *testing.T) {
	srv := testServer(t)
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: evil\n  input: {}\n  steps:\n    - id: s\n      plugin: /tmp/evil/x\n"
	_, _, rpcErr := srv.callTool("validate_pipeline", map[string]interface{}{"yaml": yamlStr})
	if rpcErr == nil {
		t.Fatal("want E_PLUGIN_OUTSIDE_ROOT")
	}
	if !strings.Contains(rpcErr.Message, "E_PLUGIN_OUTSIDE_ROOT") {
		t.Fatalf("want OUTSIDE_ROOT, got %v", rpcErr)
	}
}

func TestMCPCancelRun(t *testing.T) {
	srv := testServer(t)
	// sleeper-плагин
	dir := t.TempDir()
	plugDir := dir + "/plugins"
	_ = plugDir
	sleeperSrc := srv.pluginsDirs[0] + "/sleeper"
	os.MkdirAll(sleeperSrc, 0755)
	manifest := map[string]interface{}{
		"id": "sleeper", "version": "0.1", "platform_api": "0.1",
		"runtime": map[string]interface{}{"type": "python", "entry": "main.py"},
		"input":   map[string]interface{}{},
		"output":  map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}},
	}
	raw, _ := json.Marshal(manifest)
	os.WriteFile(sleeperSrc+"/plugin.yaml", raw, 0644)
	py := "import sys,json,time\njson.load(sys.stdin)\ntime.sleep(30)\njson.dump({'status':'ok','output':{'done':True}},sys.stdout)\n"
	os.WriteFile(sleeperSrc+"/main.py", []byte(py), 0644)
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: sleep_demo\n  input: {}\n  steps:\n    - id: sleep\n      plugin: " + sleeperSrc + "\n      timeout: 30s\n"
	res, _, rpcErr := srv.callTool("run_pipeline", map[string]interface{}{"yaml": yamlStr})
	if rpcErr != nil {
		t.Fatalf("run: %v", rpcErr)
	}
	var runOut struct {
		RunID string `json:"run_id"`
	}
	_ = json.Unmarshal([]byte(res), &runOut)
	time.Sleep(500 * time.Millisecond)
	res, _, rpcErr = srv.callTool("cancel_run", map[string]interface{}{"run_id": runOut.RunID})
	if rpcErr != nil {
		t.Fatalf("cancel: %v", rpcErr)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		res2, _, _ := srv.callTool("get_run", map[string]interface{}{"run_id": runOut.RunID})
		var g struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal([]byte(res2), &g)
		if g.Status == "cancelled" {
			return
		}
		if g.Status == "failed" {
			t.Fatalf("failed вместо cancelled: %s", res2)
		}
		if time.Now().After(deadline) {
			t.Fatalf("нет cancelled: %s", res2)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Без пайпов: handle() возвращает только Response, runner/gate в MCP идут
// с Quiet:true и os.Stdout→stderr (см. cli/mcp.go), плагины пишут в capture-буферы.
func TestMCPStdioIsolation(t *testing.T) {
	srv := testServer(t)
	mkReq := func(id int64, method string, params interface{}) *Request {
		raw, _ := json.Marshal(params)
		return &Request{JSONRPC: "2.0", ID: &id, Method: method, Params: raw}
	}
	reqs := []*Request{
		mkReq(1, "initialize", map[string]interface{}{}),
		mkReq(2, "tools/list", map[string]interface{}{}),
		mkReq(3, "tools/call", map[string]interface{}{"name": "list_plugins", "arguments": map[string]interface{}{}}),
	}
	for _, req := range reqs {
		resp := srv.handle(req)
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		var back Response
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("ответ не JSON-RPC: %q", string(b))
		}
		if back.JSONRPC != "" && back.JSONRPC != "2.0" {
			t.Fatalf("bad jsonrpc: %q", string(b))
		}
	}
}
