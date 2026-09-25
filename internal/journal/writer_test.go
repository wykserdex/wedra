package journal

// v0.23: журнал — надёжность (без мутаций, атомарный снапшот).

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wedra/internal/runctx"
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
func TestNewJournalDoesNotTruncateExisting(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	j.Event("marker", map[string]interface{}{"value": "keep"})
	j.Close()
	if _, err := NewJournal(dir); err == nil {
		t.Fatal("повторное создание должно вернуть collision")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var ev map[string]interface{}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	if ev["type"] != "marker" {
		t.Fatalf("старый журнал перезаписан: %v", ev)
	}
}

func TestSnapshotAtomic(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	ctx := runctx.NewCtx(map[string]interface{}{"input": "x"})
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

func TestEventAndSnapshotReportSerializationErrors(t *testing.T) {
	j, err := NewJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Event("bad", map[string]interface{}{"value": math.NaN()}); err == nil {
		t.Fatal("Event должен вернуть ошибку сериализации")
	}
	if j.WriteErrors() != 1 {
		t.Fatalf("write errors=%d, want 1", j.WriteErrors())
	}
	ctx := runctx.NewCtx(map[string]interface{}{"bad": math.Inf(1)})
	if err := j.Snapshot(ctx); err == nil {
		t.Fatal("Snapshot должен вернуть ошибку сериализации")
	}
	if j.WriteErrors() != 2 {
		t.Fatalf("write errors=%d, want 2", j.WriteErrors())
	}
}

func TestReaderAcceptsLargeEvent(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("x", 100*1024)
	if err := j.Event("item_start", map[string]interface{}{"payload": payload}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	events, err := NewReader(dir).Events()
	if err != nil || len(events) != 1 {
		t.Fatalf("large event: count=%d err=%v", len(events), err)
	}
	if events[0]["payload"] != payload {
		t.Fatal("large event payload changed")
	}
}

func TestReaderRejectsMalformedEvent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), []byte("{bad\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewReader(dir).Events(); err == nil {
		t.Fatal("malformed event must be reported")
	}
}
