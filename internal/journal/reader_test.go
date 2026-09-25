package journal

// P2 F-03: бюджеты чтения журнала — окно EventsBounded (события/байты),
// потоковый ScanMeta (память O(1)) и неизменность полного Events().

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeJournal — журнал из n событий (плюс хвост run_end) без платы на
// пересоздание Journal: пишем jsonl напрямую.
func writeJournal(t *testing.T, dir string, n int) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < n; i++ {
		line, err := json.Marshal(map[string]interface{}{
			"type": "step_end", "ts": fmt.Sprintf("2026-01-01T00:00:%02dZ", i%60), "step": "s1",
		})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func eventTypes(events []map[string]interface{}) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		typ, _ := ev["type"].(string)
		out = append(out, typ)
	}
	return out
}

// Полное чтение не должно измениться: budget=0 — старые гарантии.
func TestEventsStaysUnboundedAndComplete(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, 50)
	events, err := NewReader(dir).Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 50 {
		t.Fatalf("Events()=%d, want 50", len(events))
	}
	full, err := NewReader(dir).EventsBounded(0, ReadLimits{MaxBytes: 1 << 30, MaxEvents: 1000, Tail: true})
	if err != nil {
		t.Fatal(err)
	}
	if full.Truncated || full.First != 0 || full.Next != 50 || full.Total != 50 {
		t.Fatalf("окно не покрывает журнал: %+v", full)
	}
	if strings.Join(eventTypes(full.Events), ",") != strings.Join(eventTypes(events), ",") {
		t.Fatal("ограниченное чтение отдало другой журнал, чем Events()")
	}
}

// head-окно: первые N событий, курсор next продолжает чтение без потерь.
func TestEventsBoundedHeadWindow(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, 10)
	limits := ReadLimits{MaxEvents: 4}
	first, err := NewReader(dir).EventsBounded(0, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 4 || first.First != 0 || first.Next != 4 {
		t.Fatalf("head-окно: %+v", first)
	}
	if !first.Truncated {
		t.Fatal("head-окно обязано помечать обрезание")
	}
	seen := append([]string{}, eventTypes(first.Events)...)
	// догрузка по курсору: окно за окном, пока не покроет журнал
	since := first.Next
	for range 5 {
		chunk, err := NewReader(dir).EventsBounded(since, limits)
		if err != nil {
			t.Fatal(err)
		}
		if chunk.First != since {
			t.Fatalf("курсор since=%d: first=%d", since, chunk.First)
		}
		seen = append(seen, eventTypes(chunk.Events)...)
		if !chunk.Truncated {
			break
		}
		if chunk.Next <= since {
			t.Fatalf("курсор не продвинулся: since=%d next=%d", since, chunk.Next)
		}
		since = chunk.Next
	}
	all, err := NewReader(dir).Events()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(seen, ",") != strings.Join(eventTypes(all), ",") {
		t.Fatalf("догрузка по курсору потеряла события: %d != %d", len(seen), len(all))
	}
}

// tail-окно: последние события, Total точный (проход доходит до конца).
func TestEventsBoundedTailWindow(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, 30)
	res, err := NewReader(dir).EventsBounded(0, ReadLimits{MaxEvents: 5, Tail: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 5 || res.First != 25 || res.Next != 30 || res.Total != 30 {
		t.Fatalf("tail-окно: %+v", res)
	}
	if !res.Truncated {
		t.Fatal("tail-окно обязано помечать обрезание")
	}
	// последнее событие журнала — последнее в окне
	all, err := NewReader(dir).Events()
	if err != nil {
		t.Fatal(err)
	}
	last := all[len(all)-1]
	if res.Events[len(res.Events)-1]["ts"] != last["ts"] {
		t.Fatal("tail-окно не доведено до последнего события")
	}
}

// Байтовый потолок: окно не превышает бюджет, но не выбрасывает всё.
func TestEventsBoundedByteBudget(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 40; i++ {
		line, err := json.Marshal(map[string]interface{}{
			"type": "item_end", "item_index": i, "payload": strings.Repeat("y", 512),
		})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	limits := ReadLimits{MaxBytes: 2048, Tail: true}
	res, err := NewReader(dir).EventsBounded(0, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) == 0 || len(res.Events) >= 40 {
		t.Fatalf("потолок по байтам не сработал: событий %d", len(res.Events))
	}
	if res.Bytes > limits.MaxBytes+1024 {
		t.Fatalf("окно держит %d байт при бюджете %d", res.Bytes, limits.MaxBytes)
	}
	if !res.Truncated || res.Total != 40 || res.Next != 40 {
		t.Fatalf("tail по байтам: %+v", res)
	}
	// хвост журнала на месте: последнее событие — item_index 39
	last := res.Events[len(res.Events)-1]
	if idx, _ := ParseItemIndex(last["item_index"]); idx != 39 {
		t.Fatalf("хвост потерян: последнее событие %v", last)
	}
}

// since за концом журнала: пусто, курсор не убегает за Total.
func TestEventsBoundedSinceBeyondTotal(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, 3)
	res, err := NewReader(dir).EventsBounded(99, ReadLimits{MaxEvents: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 0 || res.Total != 3 || res.Next != 3 || res.Truncated {
		t.Fatalf("since за концом: %+v", res)
	}
	tail, err := NewReader(dir).EventsBounded(-5, ReadLimits{MaxEvents: 10, Tail: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Events) != 3 || tail.First != 0 {
		t.Fatalf("отрицательный since: %+v", tail)
	}
}

// Потолок по умолчанию: ограниченный и в хвост (для короткого журнала окно
// покрывает его целиком).
func TestDefaultReadLimitsAreBoundedTail(t *testing.T) {
	limits := DefaultReadLimits()
	if limits.MaxBytes <= 0 || limits.MaxEvents <= 0 || !limits.Tail {
		t.Fatalf("дефолтный потолок не ограничен: %+v", limits)
	}
	dir := t.TempDir()
	writeJournal(t, dir, 5)
	res, err := NewReader(dir).EventsBounded(0, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 5 || res.Truncated || res.First != 0 || res.Next != 5 {
		t.Fatalf("окно короткого журнала: %+v", res)
	}
}

// Битое событие в окне: ошибка с номером строки, как у Events().
func TestEventsBoundedReportsMalformedEvent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), []byte("{\"type\":\"run_start\"}\n{bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewReader(dir).EventsBounded(0, ReadLimits{MaxEvents: 10, Tail: true}); err == nil {
		t.Fatal("битое событие в окне должно давать ошибку")
	} else if !strings.Contains(err.Error(), "строка 2") {
		t.Fatalf("номер строки потерян: %v", err)
	}
}

// ScanMeta: все события, O(1) память, нужные поля на месте.
func TestScanMetaCoversWholeJournal(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	j.Event("run_start", map[string]interface{}{"pipeline": "demo", "blob": strings.Repeat("z", 4096)})
	j.Event("item_end", map[string]interface{}{"item_index": 0, "status": "ok"})
	j.Event("item_end", map[string]interface{}{"item_index": 1, "status": "aborted"})
	j.Event("run_end", map[string]interface{}{"aborted": float64(1), "ts": "2026-01-01T00:00:05Z"})
	j.Close()

	var types []string
	var statuses []string
	indexes := []int{}
	aborted := 0.0
	res, err := NewReader(dir).ScanMeta(func(meta EventMeta) error {
		types = append(types, meta.Type)
		if meta.Status != "" {
			statuses = append(statuses, meta.Status)
		}
		if meta.ItemIndex != nil {
			indexes = append(indexes, int(*meta.ItemIndex))
		}
		if meta.Aborted != nil {
			aborted = *meta.Aborted
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Truncated || res.Total != 4 {
		t.Fatalf("ScanMeta: %+v", res)
	}
	if strings.Join(types, ",") != "run_start,item_end,item_end,run_end" {
		t.Fatalf("типы: %v", types)
	}
	if strings.Join(statuses, ",") != "ok,aborted" {
		t.Fatalf("статусы: %v", statuses)
	}
	if len(indexes) != 2 || indexes[0] != 0 || indexes[1] != 1 {
		t.Fatalf("item_index: %v", indexes)
	}
	if aborted != 1 {
		t.Fatalf("aborted=%v", aborted)
	}
}

// ScanMeta упирается в байтовый потолок и честно помечает Truncated.
func TestScanMetaStopsAtByteBudget(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, 200)
	restore := metaScanByteBudget
	metaScanByteBudget = 256
	t.Cleanup(func() { metaScanByteBudget = restore })

	res, err := NewReader(dir).ScanMeta(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Fatal("превышение потолка прохода должно давать Truncated")
	}
	if res.Total == 0 || res.Total >= 200 {
		t.Fatalf("проход не оборвался на потолке: %+v", res)
	}
}

// MaxItemIndex через потоковый проход: тот же результат, что у полного чтения.
func TestMaxItemIndexSeesAllItemEndEvents(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		if i == 4999 {
			j.Event("item_end", map[string]interface{}{"item_index": i, "status": "aborted"})
			continue
		}
		j.Event("item_end", map[string]interface{}{"item_index": i, "status": "ok"})
	}
	j.Close()
	store := NewFilesystemStore(filepath.Dir(dir))
	idx, err := store.MaxItemIndex(filepath.Base(dir))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 4998 {
		t.Fatalf("MaxItemIndex=%d, want 4998", idx)
	}
	if idx, err := NewFilesystemStore(dir).MaxItemIndex("missing-run"); err != nil || idx != -1 {
		t.Fatalf("MaxItemIndex для прогона без журнала: idx=%d err=%v", idx, err)
	}
}
