package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Journal — append-only журнал прогона: runs/<run_id>/journal.jsonl
// + актуальный снапшот контекста context.json (основа будущего --resume).
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

func (j *Journal) Event(kind string, kv map[string]interface{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	kv["ts"] = time.Now().UTC().Format(time.RFC3339)
	kv["type"] = kind
	b, _ := json.Marshal(kv)
	j.f.Write(append(b, '\n'))
}

func (j *Journal) Snapshot(ctx *Ctx) {
	b, err := json.MarshalIndent(ctx.Data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(j.Dir, "context.json"), b, 0o644)
}

func (j *Journal) Close() { j.f.Close() }
