package mcp

import (
	"strings"
	"testing"

	"wedra/internal/plugin"
)

// agentServer — сервер с ЯВНО разрешённым exec_plugin. Тесты ниже проверяют
// разбор аргументов и границы пути, а не политику, поэтому они должны
// разрешать запуск: иначе их отличает отказ по политике, а проверяемое
// поведение не наступает.
func agentServer() *Server {
	return &Server{trust: plugin.TrustPolicy{AgentCanExec: true}}
}

// Политика доверия по умолчанию закрывает инструмент. Проверка разрешения
// идёт ПЕРВОЙ, до аргументов, до чтения манифеста и до запуска: без явного
// согласия ядра агент не исполняет код вообще.
func TestToolExecPluginDeniedByDefault(t *testing.T) {
	s := &Server{}
	_, _, rpcErr := s.toolExecPlugin(map[string]interface{}{"plugin": "echo_ok"})
	if rpcErr == nil {
		t.Fatal("exec_plugin без политики должен отказать, а не запускать код")
	}
	if !strings.Contains(rpcErr.Message, "--allow-agent-exec") {
		t.Error("отказ должен называть нужный флаг, получено: " + rpcErr.Message)
	}
	// Отказ до разбора аргументов: даже корректный аргумент не проходит.
	_, _, rpcErr = s.toolExecPlugin(map[string]interface{}{})
	if rpcErr == nil || !strings.Contains(rpcErr.Message, "--allow-agent-exec") {
		t.Error("проверка разрешения должна идти до проверки аргументов, получено: " + rpcErr.Message)
	}
}

func TestToolExecPluginRequiresPlugin(t *testing.T) {
	s := agentServer()
	_, _, rpcErr := s.toolExecPlugin(map[string]interface{}{})
	if rpcErr == nil {
		t.Error("exec_plugin без plugin должен вернуть ошибку")
	}
	if !strings.Contains(rpcErr.Message, "нужен plugin") {
		t.Error("Ожидается сообщение 'нужен plugin', получено: " + rpcErr.Message)
	}
}

func TestToolExecPluginRejectsOutsideRoot(t *testing.T) {
	s := agentServer()
	s.workDir = "/tmp/test"
	_, _, rpcErr := s.toolExecPlugin(map[string]interface{}{
		"plugin": "../outside",
	})
	if rpcErr == nil {
		t.Error("exec_plugin с .. должен вернуть ошибку")
	}
	if !strings.Contains(rpcErr.Message, "E_PLUGIN_OUTSIDE_ROOT") {
		t.Error("Ожидается E_PLUGIN_OUTSIDE_ROOT, получено: " + rpcErr.Message)
	}
}

func TestToolExecPluginRejectsUnknownPlugin(t *testing.T) {
	s := agentServer()
	s.workDir = "/tmp/test"
	s.multi = &multiEngine{dirs: []string{"/tmp/test/plugins"}, workDir: "/tmp/test"}
	_, _, rpcErr := s.toolExecPlugin(map[string]interface{}{
		"plugin": "nonexistent/plugin",
	})
	if rpcErr == nil {
		t.Error("exec_plugin с неизвестным плагином должен вернуть ошибку")
	}
}
