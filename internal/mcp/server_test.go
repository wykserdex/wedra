package mcp

import (
	"bytes"
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
		"permissions": map[string]interface{}{
			"network":    []map[string]interface{}{{"host": "api.example.com", "port": 443}},
			"filesystem": "workspace", "secrets": []string{"DEMO_TOKEN"},
		},
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

func writePolicyPlugin(t *testing.T, pluginsDir, id string, network []map[string]interface{}, input map[string]interface{}) string {
	t.Helper()
	dir := filepath.Join(pluginsDir, id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{
		"id": id, "version": "0.1", "platform_api": "0.1",
		"runtime": map[string]interface{}{"type": "python", "entry": "main.py"},
		"input":   input,
		"output":  map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}},
		"permissions": map[string]interface{}{
			"network": network, "filesystem": "workspace", "secrets": []string{},
		},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte("import json,sys\njson.load(sys.stdin)\njson.dump({'status':'ok','output':{'done':True}},sys.stdout)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
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

func TestMCPInitializeNegotiatesAndExposesPermissions(t *testing.T) {
	srv := testServer(t)
	resp := srv.handle(&Request{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize",
		Params: json.RawMessage(`{"protocolVersion":"2026-07-28"}`),
	})
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	result, _ := json.Marshal(resp.Result)
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(result, &init); err != nil {
		t.Fatal(err)
	}
	if init.ProtocolVersion != "2024-11-05" {
		t.Fatalf("unexpected protocol version: %q", init.ProtocolVersion)
	}

	listed, _, rpcErr := srv.callTool("list_plugins", map[string]interface{}{})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	var listOut struct {
		Plugins []struct {
			Permissions map[string]interface{} `json:"permissions"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(listed), &listOut); err != nil {
		t.Fatal(err)
	}
	if len(listOut.Plugins) != 1 {
		t.Fatalf("unexpected plugin list: %s", listed)
	}
	for _, key := range []string{"network", "filesystem", "secrets"} {
		if _, ok := listOut.Plugins[0].Permissions[key]; !ok {
			t.Fatalf("permission %q missing: %s", key, listed)
		}
	}
}

func TestMCPTransportPreservesStringID(t *testing.T) {
	tr := NewTransport(bytes.NewBufferString(`{"jsonrpc":"2.0","id":"abc","method":"ping"}`+"\n"), &bytes.Buffer{})
	req, err := tr.Read()
	if err != nil {
		t.Fatal(err)
	}
	if string(req.ID) != `"abc"` {
		t.Fatalf("unexpected request id: %s", req.ID)
	}
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
	found := false
	for _, issue := range out.Issues {
		if issue.Code == "E_PORT_SOURCE" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want E_PORT_SOURCE, got %s", res)
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

func TestMCPResolvesRelativePluginFromWorkDir(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	plugins := filepath.Join(work, "plugins")
	if err := os.MkdirAll(plugins, 0755); err != nil {
		t.Fatal(err)
	}
	writeFakePlugin(t, plugins, "echoer",
		map[string]interface{}{"text": map[string]interface{}{"type": "string", "from": "input.text"}},
		map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}})
	srv, err := NewServer(Options{PluginsDirs: []string{"plugins"}, WorkDir: work})
	if err != nil {
		t.Fatal(err)
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	otherCWD := filepath.Join(root, "other")
	if err := os.MkdirAll(otherCWD, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(otherCWD); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldCWD)

	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: relative_plugin\n  input:\n    text: \"hello\"\n  steps:\n    - id: s\n      plugin: plugins/echoer\n      bind:\n        text: input.text\n"
	res, isErr, rpcErr := srv.callTool("validate_pipeline", map[string]interface{}{"yaml": yamlStr})
	if rpcErr != nil || isErr {
		t.Fatalf("relative plugin failed: rpc=%v isErr=%v result=%s", rpcErr, isErr, res)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK {
		t.Fatalf("relative plugin validation failed: %s", res)
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

func TestMCPRejectsFileRefOutsideWorkdir(t *testing.T) {
	srv := testServer(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	writePolicyPlugin(t, srv.pluginsDirs[0], "filereader", nil, map[string]interface{}{
		"path": map[string]interface{}{"from": "input.path", "type": "string", "format": "file_ref"},
	})
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: file_read\n  input:\n    path: " + outside + "\n  steps:\n    - id: read\n      plugin: " + filepath.Join(srv.pluginsDirs[0], "filereader") + "\n      bind:\n        path: input.path\n"
	_, _, rpcErr := srv.callTool("validate_pipeline", map[string]interface{}{"yaml": yamlStr})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "E_FILE_REF_OUTSIDE_ROOT") {
		t.Fatalf("expected file_ref boundary error, got %v", rpcErr)
	}
}

func TestMCPAllowsFileRefInsideWorkdir(t *testing.T) {
	srv := testServer(t)
	inside := filepath.Join(srv.workDir, "inside.txt")
	if err := os.WriteFile(inside, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	pluginDir := writePolicyPlugin(t, srv.pluginsDirs[0], "filereader", nil, map[string]interface{}{
		"path": map[string]interface{}{"from": "input.path", "type": "string", "format": "file_ref"},
	})
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: file_read\n  input:\n    path: " + inside + "\n  steps:\n    - id: read\n      plugin: " + pluginDir + "\n      bind:\n        path: input.path\n"
	res, _, rpcErr := srv.callTool("validate_pipeline", map[string]interface{}{"yaml": yamlStr})
	if rpcErr != nil {
		t.Fatalf("inside file_ref rejected: %v", rpcErr)
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil || !out.OK {
		t.Fatalf("inside file_ref validation: %s", res)
	}
}

func TestMCPRejectsPrivateAndAnyHostNetwork(t *testing.T) {
	srv := testServer(t)
	for _, network := range [][]map[string]interface{}{
		{{"any_host": true, "port": 443}},
		{{"host": "127.0.0.1", "port": 80}},
		{{"host": "localhost", "port": 80}},
	} {
		pluginDir := writePolicyPlugin(t, srv.pluginsDirs[0], "network-plugin", network, map[string]interface{}{})
		yamlStr := "format_version: \"0.2\"\npipeline:\n  name: net\n  input: {}\n  steps:\n    - id: net\n      plugin: " + pluginDir + "\n"
		_, _, rpcErr := srv.callTool("validate_pipeline", map[string]interface{}{"yaml": yamlStr})
		if rpcErr == nil || !strings.Contains(rpcErr.Message, "E_NETWORK_DENIED") {
			t.Fatalf("network %v was accepted: %v", network, rpcErr)
		}
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
		rawID, _ := json.Marshal(id)
		return &Request{JSONRPC: "2.0", ID: rawID, Method: method, Params: raw}
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
