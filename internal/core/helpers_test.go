package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requirePython пропускает интеграционные тесты без интерпретатора.
func requirePython(t *testing.T) {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	t.Skip("python не найден — пропускаю интеграционный тест")
}

// newStdin подменяет os.Stdin скриптом ответов (для тестов human_gate).
func newStdin(t *testing.T, data string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(data); err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; r.Close() })
}

// readEvents читает journal.jsonl прогона построчно; любая битая строка = падение теста
// (журнал — контракт, он обязан быть валидным JSONL всегда).
func readEvents(t *testing.T, runDir string) []map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runDir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev map[string]interface{}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("битая строка журнала %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

func countEvents(events []map[string]interface{}, typ string) int {
	n := 0
	for _, e := range events {
		if e["type"] == typ {
			n++
		}
	}
	return n
}

func quietOpts(t *testing.T) RunOptions {
	return RunOptions{Quiet: true, RunsDir: t.TempDir()}
}
