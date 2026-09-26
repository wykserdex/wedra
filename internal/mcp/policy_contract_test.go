package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// Контракт отказа политики в MCP, найденный прогоном реального пайплайна:
//
//   - validate_pipeline возвращает ok:false и issue с кодом — это нормальный
//     результат проверки, а не поломка вызова;
//   - run_pipeline по-прежнему отказывает, fail-closed не ослаблен.

func TestValidateReportsPolicyRefusalAsIssue(t *testing.T) {
	srv := testServer(t)
	pluginDir := writePolicyPlugin(t, srv.pluginsDirs[0], "network-plugin",
		[]map[string]interface{}{{"any_host": true, "port": 443}}, map[string]interface{}{})
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: net\n  input: {}\n  steps:\n    - id: net\n      plugin: " + pluginDir + "\n"
	assertPolicyRefusal(t, srv, yamlStr, "network_denied", "E_NETWORK_DENIED")
}

func TestValidateReportsOutsideRootAsIssue(t *testing.T) {
	srv := testServer(t)
	yamlStr := "format_version: \"0.2\"\npipeline:\n  name: evil\n  input: {}\n  steps:\n    - id: s\n      plugin: /tmp/evil/x\n"
	assertPolicyRefusal(t, srv, yamlStr, "plugin_outside_root", "E_PLUGIN_OUTSIDE_ROOT")
}

type validateIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type validateResult struct {
	OK     bool            `json:"ok"`
	Issues []validateIssue `json:"issues"`
}

func assertPolicyRefusal(t *testing.T, srv *Server, yamlStr, wantCode, wantPolicyPrefix string) {
	t.Helper()

	out, _, rpcErr := srv.callTool("validate_pipeline", map[string]interface{}{"yaml": yamlStr})
	if rpcErr != nil {
		t.Fatalf("validate_pipeline вернул RPC-ошибку %v, ожидался ok:false с issue", rpcErr)
	}
	res := decodeValidate(t, out)
	if res.OK {
		t.Fatalf("validate_pipeline сообщил ok:true для запрещённого пайплайна: %s", out)
	}
	found := false
	for _, is := range res.Issues {
		if is.Code != wantCode {
			continue
		}
		found = true
		if !strings.Contains(is.Message, wantPolicyPrefix) {
			t.Errorf("issue.code=%q, но в сообщении нет %q: %s", is.Code, wantPolicyPrefix, is.Message)
		}
		if is.Severity != "error" {
			t.Errorf("отказ политики должен быть severity=error, получено %q", is.Severity)
		}
	}
	if !found {
		t.Fatalf("в issues нет кода %q: %s", wantCode, out)
	}

	if _, _, runErr := srv.callTool("run_pipeline",
		map[string]interface{}{"yaml": yamlStr, "wait_seconds": 1}); runErr == nil {
		t.Fatal("run_pipeline выполнил запрещённый пайплайн — fail-closed нарушен")
	}
}

func decodeValidate(t *testing.T, out string) validateResult {
	t.Helper()
	// callTool отдаёт внутренний JSON; MCP-слой оборачивает его в content[].
	var bare validateResult
	if err := json.Unmarshal([]byte(out), &bare); err == nil && (bare.OK || bare.Issues != nil) {
		return bare
	}
	var wrapper struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &wrapper); err != nil || len(wrapper.Content) == 0 {
		t.Fatalf("не разобрал ответ validate: %s", out)
	}
	var res validateResult
	if err := json.Unmarshal([]byte(wrapper.Content[0].Text), &res); err != nil {
		t.Fatalf("разбор результата validate: %v (%s)", err, wrapper.Content[0].Text)
	}
	return res
}
