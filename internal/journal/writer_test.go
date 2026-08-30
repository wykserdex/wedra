package journal

// v0.23: журнал — надёжность (без мутаций, атомарный снапшот).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/internal/context"
)

// Event не мутирует переданный map (footgun для переиспользуемых мап).
func TestEventDoesNotMutateInput(t *testing.T) {
	j, err := NewJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	kv := map[string]interface{}{"step": "x", "attempt": 1}
	j.Event("step_start", kv)
	if _, ok := kv["ts"]; ok {
		t.Fatal("Event мутировал входной map: появился ts")
	}
	if _, ok := kv["type"]; ok {
		t.Fatal("Event мутировал входной map: появился type")
	}
	// и в файле событие корректное
	raw, err := os.ReadFile(filepath.Join(j.Dir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var ev map[string]interface{}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("битое событие: %v", err)
	}
	if ev["type"] != "step_start" || ev["step"] != "x" {
		t.Fatalf("событие некорректно: %v", ev)
	}
	if _, ok := ev["ts"]; !ok {
		t.Fatal("в файле нет ts")
	}
}

// Snapshot — context.json появляется атомарно, без .tmp-хвостов.
func TestSnapshotAtomic(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	ctx := context.NewCtx(map[string]interface{}{"input": "x"})
	ctx.SetStep("a", map[string]interface{}{"k": float64(1)})
	j.Snapshot(ctx)
	raw, err := os.ReadFile(filepath.Join(dir, "context.json"))
	if err != nil {
		t.Fatalf("context.json не появился: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("битый context.json: %v", err)
	}
	if data["steps"] == nil {
		t.Fatalf("steps нет в снапшоте: %v", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "context.json.tmp")); err == nil {
		t.Fatal("остался context.json.tmp — rename не сработал")
	}
}
