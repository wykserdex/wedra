package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wedra/internal/runctx"
)

// Journal — append-only журнал прогона: var/runs/<run_id>/journal.jsonl
type Journal struct {
	// v0.23: счётчик потерянных событий (write-ошибки)
	writeErrs int
	mu        sync.Mutex
	f         *os.File
	Dir       string
	onEvent   func(string, map[string]interface{})
}

func NewJournal(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		return nil, err
	}
	return &Journal{f: f, Dir: dir}, nil
}

func OpenJournalAppend(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "journal.jsonl"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return &Journal{f: f, Dir: dir}, nil
}

func (j *Journal) setEventSink(sink func(string, map[string]interface{})) {
	j.mu.Lock()
	j.onEvent = sink
	j.mu.Unlock()
}

// Event — journal-событие. v0.23: не мутирует переданный map (footgun для
// переиспользуемых мап), ошибки записи не глотаются (disk-full = видимая
// потеря, не молчаливая).
func (j *Journal) Event(kind string, kv map[string]interface{}) {
	j.mu.Lock()
	e := make(map[string]interface{}, len(kv)+2)
	for k, v := range kv {
		e[k] = v
	}
	e["ts"] = time.Now().UTC().Format(time.RFC3339)
	e["type"] = kind
	b, err := json.Marshal(e)
	if err != nil {
		j.mu.Unlock()
		fmt.Fprintf(os.Stderr, "journal: marshal %s: %v (событие потеряно)\n", kind, err)
		return
	}
	_, werr := j.f.Write(append(b, '\n'))
	if werr != nil {
		j.writeErrs++
		if j.writeErrs <= 3 {
			fmt.Fprintf(os.Stderr, "journal: запись %s не удалась: %v (событие потеряно)\n", kind, werr)
		}
	}
	sink := j.onEvent
	j.mu.Unlock()
	if werr == nil && sink != nil {
		data := make(map[string]interface{}, len(kv))
		for k, v := range kv {
			data[k] = v
		}
		sink(kind, data)
	}
}

// Snapshot — context.json. v0.23: атомарно (temp+rename) — краш в середине
// больше не даёт битый файл, из-за которого resume отвалился бы.
func (j *Journal) Snapshot(ctx *runctx.Ctx) {
	b, err := json.MarshalIndent(ctx.Data, "", "  ")
	if err != nil {
		return
	}
	tmp := filepath.Join(j.Dir, "context.json.tmp")
	if werr := os.WriteFile(tmp, b, 0o644); werr != nil {
		fmt.Fprintf(os.Stderr, "journal: snapshot: %v\n", werr)
		return
	}
	if rerr := os.Rename(tmp, filepath.Join(j.Dir, "context.json")); rerr != nil {
		fmt.Fprintf(os.Stderr, "journal: snapshot rename: %v\n", rerr)
	}
}

// WriteErrors — сколько событий не записалось (для честного финального отчёта).
func (j *Journal) WriteErrors() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.writeErrs
}

func (j *Journal) Close() {
	if j.writeErrs > 0 {
		fmt.Fprintf(os.Stderr, "journal: %d событий не записалось за ран (журнал неполный!)\n", j.writeErrs)
	}
	j.f.Close()
}
