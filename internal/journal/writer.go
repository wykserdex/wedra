package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"orchestrator/internal/context"
)

// Journal — append-only журнал прогона: var/runs/<run_id>/journal.jsonl
type Journal struct {
	mu  sync.Mutex
	f   *os.File
	Dir string
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

func (j *Journal) Event(kind string, kv map[string]interface{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	kv["ts"] = time.Now().UTC().Format(time.RFC3339)
	kv["type"] = kind
	b, _ := json.Marshal(kv)
	j.f.Write(append(b, '\n'))
}

func (j *Journal) Snapshot(ctx *context.Ctx) {
	b, err := json.MarshalIndent(ctx.Data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(j.Dir, "context.json"), b, 0o644)
}

func (j *Journal) Close() { j.f.Close() }
