package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Регрессии, найденные прогоном реального пайплайна через MCP (dogfood):
// агент должен видеть провал рана, а не "done", и статус должен читаться из
// журнала, даже если рана нет в памяти процесса.

func TestRunSucceededMatchesCLISemantics(t *testing.T) {
	cases := []struct {
		name        string
		aborted, ok int
		foreach     string
		wantOK      bool
	}{
		{"всё прошло", 0, 3, "", true},
		{"ошибка шага без foreach", 2, 0, "", false},
		{"частичный провал", 1, 4, "", false},
		{"foreach отфильтровал", 3, 2, "input.paths", true},
		{"ничего не выполнено", 1, 0, "input.paths", true}, // как в CLI: abort в foreach не провал
	}
	for _, c := range cases {
		if got := runSucceeded(c.aborted, c.ok, c.foreach); got != c.wantOK {
			t.Errorf("%s: runSucceeded(%d,%d,%q) = %v, ожидалось %v",
				c.name, c.aborted, c.ok, c.foreach, got, c.wantOK)
		}
	}
}

// TestRunStatusFromJournalWhenNotInMemory — get_run в новом процессе обязан
// отдавать реальный статус из журнала, а не "unknown".
func TestRunStatusFromJournalWhenNotInMemory(t *testing.T) {
	srv := testServer(t)
	runID := "20260101-000000-journal-abcdef123456"
	dir := filepath.Join(srv.runsDir, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	events := []map[string]interface{}{
		{"type": "run_start", "pipeline": "x", "ts": "2026-01-01T00:00:00Z"},
		{"type": "step_start", "step": "s1"},
		{"type": "step_failed", "step": "s1", "error": "boom"},
		{"type": "run_end", "ok": 0, "aborted": 1},
	}
	writeJournal(t, dir, events)

	// Ран намеренно НЕ добавлен в s.runs: эмулируем чужой процесс.
	if got := srv.runStatus(runID); got != "failed" {
		t.Fatalf("статус рана с прерванными шагами = %q, ожидался \"failed\"", got)
	}
}

func TestRunStatusFromJournalReportsSuccess(t *testing.T) {
	srv := testServer(t)
	runID := "20260101-000000-okrun-abcdef123456"
	dir := filepath.Join(srv.runsDir, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeJournal(t, dir, []map[string]interface{}{
		{"type": "run_start", "pipeline": "x"},
		{"type": "run_end", "ok": 2, "aborted": 0},
	})
	if got := srv.runStatus(runID); got != "done" {
		t.Fatalf("успешный ран = %q, ожидался \"done\"", got)
	}
}

func TestRunStatusUnknownForMissingRun(t *testing.T) {
	srv := testServer(t)
	if got := srv.runStatus("20260101-000000-nope-abcdef123456"); got != "unknown" {
		t.Fatalf("несуществующий ран = %q, ожидался \"unknown\"", got)
	}
}

func writeJournal(t *testing.T, dir string, events []map[string]interface{}) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		t.Fatalf("create journal: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("write journal: %v", err)
		}
	}
}
