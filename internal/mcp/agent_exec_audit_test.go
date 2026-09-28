package mcp

// Тесты аудита exec_plugin.
//
// Этот код — журнал безопасности: он фиксирует, что агент запускал код.
// Раньше он не был покрыт НИ ОДНИМ тестом, хотя именно он определяет, чем
// на самом деле является «fail-closed» в документации.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wedra/internal/pipeline"
	"wedra/internal/plugin"
)

func auditTestServer(t *testing.T, runsDir string) *Server {
	t.Helper()
	return &Server{
		trust:   plugin.TrustPolicy{AgentCanExec: true},
		execSem: make(chan struct{}, MaxConcurrentAgentExec),
		runsDir: runsDir,
	}
}

func auditManifest(id string) *pipeline.Manifest {
	return &pipeline.Manifest{
		ID:      id,
		Version: "0.1.0",
		Dir:     filepath.Join("plugins", id),
	}
}

// rpcCodeOf достаёт машинный код из RPCError. rpcErr кладёт его в
// Data["code"], а не в Message, поэтому проверять надо именно Data.
func rpcCodeOf(e *RPCError) string {
	if e == nil {
		return ""
	}
	if m, ok := e.Data.(map[string]string); ok {
		return m["code"]
	}
	return ""
}

// hasPython повторяет порядок поиска из internal/plugin: сначала python3,
// потом python. Если интерпретатора нет, тест со «спящим» плагином надо
// пропустить, а не падать из-за окружения.
func hasPython() bool {
	for _, name := range []string{"python3", "python"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func TestAgentExecAuditWritesRecord(t *testing.T) {
	dir := t.TempDir()
	s := auditTestServer(t, dir)

	err := s.auditAgentExec("text_stats", auditManifest("text_stats"),
		&plugin.ExecResult{ExitCode: 0}, 120*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, agentExecAuditFile))
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(raw))
	if strings.Contains(line, "\n") {
		t.Fatalf("в аудите должна быть одна строка на запуск, получено %d", strings.Count(line, "\n")+1)
	}
	var rec map[string]interface{}
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("строка аудита не разбирается как JSON: %v\n%s", err, line)
	}
	for k, want := range map[string]interface{}{
		"event":     "agent_plugin_exec",
		"source":    "agent_auto",
		"plugin":    "text_stats",
		"plugin_id": "text_stats",
		"exit_code": float64(0),
	} {
		if rec[k] != want {
			t.Errorf("rec[%q] = %v, ждали %v", k, rec[k], want)
		}
	}
	for _, k := range []string{"ts", "duration", "untrusted", "err_code"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("в записи нет поля %q: %s", k, line)
		}
	}
}

// Append-only, а не перезапись: журнал, который теряет предыдущие запуски,
// бесполезен для разбора инцидента.
func TestAgentExecAuditAppends(t *testing.T) {
	dir := t.TempDir()
	s := auditTestServer(t, dir)

	for i, ref := range []string{"first", "second", "third"} {
		if err := s.auditAgentExec(ref, auditManifest(ref), &plugin.ExecResult{}, time.Duration(i)*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, agentExecAuditFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("строк в журнале %d, ждали 3", len(lines))
	}
	for i, want := range []string{"first", "second", "third"} {
		var rec map[string]interface{}
		if err := json.Unmarshal([]byte(lines[i]), &rec); err != nil {
			t.Fatalf("строка %d не JSON: %v", i+1, err)
		}
		if rec["plugin"] != want {
			t.Errorf("строка %d: plugin = %v, ждали %v", i+1, rec["plugin"], want)
		}
	}
}

func TestAgentExecAuditMarksUntrusted(t *testing.T) {
	dir := t.TempDir()
	s := auditTestServer(t, dir)

	// Плагин агента: компонента пути равна AgentPluginDir.
	agent := &pipeline.Manifest{ID: "mailer", Version: "0.1.0",
		Dir: filepath.Join("work", "agent-plugins", "mailer")}
	if err := s.auditAgentExec("agent-plugins/mailer", agent, &plugin.ExecResult{}, 0); err != nil {
		t.Fatal(err)
	}
	// Обычный плагин.
	plain := &pipeline.Manifest{ID: "text_stats", Version: "0.1.0",
		Dir: filepath.Join("plugins", "text_stats")}
	if err := s.auditAgentExec("text_stats", plain, &plugin.ExecResult{}, 0); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, agentExecAuditFile))
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("строк %d, ждали 2", len(lines))
	}
	for i, want := range []bool{true, false} {
		var rec struct {
			Untrusted bool `json:"untrusted"`
		}
		if err := json.Unmarshal([]byte(lines[i]), &rec); err != nil {
			t.Fatal(err)
		}
		if rec.Untrusted != want {
			t.Errorf("строка %d: untrusted = %v, ждали %v", i+1, rec.Untrusted, want)
		}
	}
}

func TestAgentExecAuditFailsWithoutRunsDir(t *testing.T) {
	s := auditTestServer(t, "")
	if err := s.auditAgentExec("x", auditManifest("x"), &plugin.ExecResult{}, 0); err == nil {
		t.Fatal("без каталога ранов аудит обязан падать: иначе запуск неоплачен")
	}
}

// Недоступный каталог ранов. Ставим на его место ФАЙЛ: тогда OpenFile внутри
// даёт ENOTDIR одинаково на Unix и на Windows, без fiddling с правами,
// которые на Windows всё равно работают иначе.
func TestAgentExecAuditFailsWhenRunsDirIsNotADir(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := auditTestServer(t, blocker)
	err := s.auditAgentExec("x", auditManifest("x"), &plugin.ExecResult{}, 0)
	if err == nil {
		t.Fatal("аудит обязан падать, если каталог недоступен")
	}
	if !strings.Contains(err.Error(), agentExecAuditFile) {
		t.Errorf("в ошибке должно называться имя файла журнала: %v", err)
	}
}

// На месте журнала — каталог. Запись в файл невозможна, аудит обязан падать.
func TestAgentExecAuditFailsWhenJournalPathIsADir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, agentExecAuditFile), 0o755); err != nil {
		t.Fatal(err)
	}
	s := auditTestServer(t, dir)
	if err := s.auditAgentExec("x", auditManifest("x"), &plugin.ExecResult{}, 0); err == nil {
		t.Fatal("аудит обязан падать, если на месте журнала каталог")
	}
}

// САМЫЙ ВАЖНЫЙ ТЕСТ ФАЙЛА: он доказывает, в каком порядке идут запуск и
// аудит, и не даёт формулировке в документации разойтись с кодом.
//
// В toolExecPlugin ExecWithEnvCtx стоит на L1306, аудит на L1310. Значит
// плагин УЖЕ ОТРАБОТАЛ, когда пишется запись. Доказательство: плагин спит
// 1.2 с, а записанная в аудит duration не может быть ~0 — она включает
// время исполнения. Если бы аудит писался ДО запуска, duration была бы
// около нуля и тест упал бы.
//
// Раньше здесь стояла проверка «появилась ли метка на диске», и она врала:
// отсутствие метки означало не «плагин не запускался», а что мой рукописный
// скрипт угадал неверную форму конверта. Вывод из побочного эффекта здесь
// недопустим — порядок доказывается измерением, а не предположением.
func TestAgentExecAuditIsWrittenAfterExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("запускает настоящий плагин со сном")
	}
	if !hasPython() {
		t.Skip("интерпретатор python не найден в PATH")
	}
	const slept = 1200 * time.Millisecond

	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	work := filepath.Join(dir, "work")
	runs := filepath.Join(dir, "runs")
	for _, d := range []string{plugins, work, runs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFakePlugin(t, plugins, "sleeper",
		map[string]interface{}{"text": map[string]interface{}{"type": "string", "from": "input.text"}},
		map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}})
	py := "import json,sys,time" + "\n" +
		"json.load(sys.stdin)" + "\n" +
		"time.sleep(1.2)" + "\n" +
		"json.dump({'status':'ok','output':{'done':True}},sys.stdout)" + "\n"
	if err := os.WriteFile(filepath.Join(plugins, "sleeper", "main.py"), []byte(py), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(Options{
		PluginsDirs: []string{plugins},
		WorkDir:     work,
		RunsDir:     runs,
		Trust:       plugin.TrustPolicy{AgentCanExec: true, AllowUntrusted: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, rpcErr := srv.toolExecPlugin(map[string]interface{}{
		"plugin": "sleeper",
		"input":  map[string]interface{}{"text": "x"},
	}); rpcErr != nil {
		t.Fatalf("exec_plugin должен был отработать: %+v", rpcErr)
	}

	raw, err := os.ReadFile(filepath.Join(runs, agentExecAuditFile))
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		Duration string `json:"duration"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &rec); err != nil {
		t.Fatal(err)
	}
	got, err := time.ParseDuration(rec.Duration)
	if err != nil {
		t.Fatalf("duration %q не разбирается: %v", rec.Duration, err)
	}
	if got < slept {
		t.Fatalf("записанная duration = %s, а плагин спал %s. Значит аудит пишется "+
			"ДО запуска — документацию про «плагин не запускается» можно ужесточить, "+
			"а комментарий в коде исправить", got, slept)
	}
}

// Fail-closed: при недоступном аудите агент получает код ошибки и НЕ получает
// результат исполнения. Ровно это и обещает документация — на этом тест и
// останавливается: обещать он не может, что код не запускался.
func TestAgentExecAuditFailureWithholdsResult(t *testing.T) {
	if testing.Short() {
		t.Skip("запускает настоящий плагин")
	}
	dir := t.TempDir()
	plugins := filepath.Join(dir, "plugins")
	work := filepath.Join(dir, "work")
	for _, d := range []string{plugins, work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFakePlugin(t, plugins, "echo_ok",
		map[string]interface{}{"text": map[string]interface{}{"type": "string", "from": "input.text"}},
		map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}})

	// Каталог ранов нормальный, но на месте журнала — каталог: запись
	// невозможна, при этом NewServer проходит, иначе помеха поймалась бы на
	// конструкторе и до exec_plugin дело не дошло бы.
	runs := filepath.Join(dir, "runs")
	if err := os.MkdirAll(filepath.Join(runs, agentExecAuditFile), 0o755); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(Options{
		PluginsDirs: []string{plugins},
		WorkDir:     work,
		RunsDir:     runs,
		Trust:       plugin.TrustPolicy{AgentCanExec: true, AllowUntrusted: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	out, isErr, rpcErr := srv.toolExecPlugin(map[string]interface{}{
		"plugin": "echo_ok",
		"input":  map[string]interface{}{"text": "x"},
	})
	// isErr тут НЕ маркер отказа: toolExecPlugin при отказе возвращает
	// ("" , false, rpcErr). Признак отказа — непустой rpcErr.
	if rpcErr == nil {
		t.Fatalf("при недоступном аудите ожидался отказ; получено isErr=%v out=%q", isErr, out)
	}
	if code := rpcCodeOf(rpcErr); code != "E_AGENT_EXEC_AUDIT" {
		t.Errorf("код = %q, ждали E_AGENT_EXEC_AUDIT (msg=%q)", code, rpcErr.Message)
	}
	if out != "" {
		t.Errorf("агент не должен получать результат неоплаченного запуска, получено: %q", out)
	}
}

// Пустой результат аудита не должен молча считаться успешным: пустой runsDir
// даёт ошибку, и она обязана доходить до вызывающего.
func TestAgentExecAuditErrorMentionsDirectory(t *testing.T) {
	s := auditTestServer(t, "")
	err := s.auditAgentExec("x", auditManifest("x"), &plugin.ExecResult{}, 0)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if !strings.Contains(err.Error(), "runs") && !strings.Contains(err.Error(), "каталог") {
		t.Errorf("ошибка должна объяснять, чего не хватает: %v", err)
	}
}
