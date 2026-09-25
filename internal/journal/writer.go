package journal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wedra/internal/runctx"
)

const maxJournalPayloadSize = 16 << 20

// Journal — append-only журнал прогона: var/runs/<run_id>/journal.jsonl
type Journal struct {
	// v0.23: счётчик потерянных событий (write-ошибки)
	writeErrs int
	mu        sync.Mutex
	f         *os.File
	Dir       string
}

func NewJournal(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "journal.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
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

// Event — journal-событие. v0.23: не мутирует переданный map (footgun для
// переиспользуемых мап), ошибки записи не глотаются (disk-full = видимая
// потеря, не молчаливая).
func (j *Journal) Event(kind string, kv map[string]interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil {
		j.writeErrs++
		return os.ErrClosed
	}
	e := make(map[string]interface{}, len(kv)+2)
	for k, v := range kv {
		e[k] = v
	}
	e["ts"] = time.Now().UTC().Format(time.RFC3339)
	e["type"] = kind
	b, err := json.Marshal(e)
	if err != nil {
		j.writeErrs++
		fmt.Fprintf(os.Stderr, "journal: marshal %s: %v (событие потеряно)\n", kind, err)
		return err
	}
	line := append(b, '\n')
	if len(line) > maxJournalPayloadSize {
		j.writeErrs++
		err := fmt.Errorf("journal event %s exceeds %d bytes", kind, maxJournalPayloadSize)
		fmt.Fprintf(os.Stderr, "journal: %v (событие потеряно)\n", err)
		return err
	}
	n, werr := j.f.Write(line)
	if werr == nil && n != len(line) {
		werr = io.ErrShortWrite
	}
	if werr != nil {
		j.writeErrs++
		if j.writeErrs <= 3 {
			fmt.Fprintf(os.Stderr, "journal: запись %s не удалась: %v (событие потеряно)\n", kind, werr)
		}
		return werr
	}
	return nil
}

// Snapshot — context.json. v0.23: атомарно (temp+rename) — краш в середине
// больше не даёт битый файл, из-за которого resume отвалился бы.
func (j *Journal) Snapshot(ctx *runctx.Ctx) error {
	if ctx == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil {
		j.writeErrs++
		return os.ErrClosed
	}
	b, err := json.MarshalIndent(ctx.Data, "", "  ")
	if err != nil {
		j.writeErrs++
		fmt.Fprintf(os.Stderr, "journal: snapshot marshal: %v\n", err)
		return err
	}
	if len(b) > maxJournalPayloadSize {
		j.writeErrs++
		err := fmt.Errorf("context snapshot exceeds %d bytes", maxJournalPayloadSize)
		fmt.Fprintf(os.Stderr, "journal: %v\n", err)
		return err
	}
	tmp := filepath.Join(j.Dir, "context.json.tmp")
	if werr := os.WriteFile(tmp, b, 0o644); werr != nil {
		j.writeErrs++
		fmt.Fprintf(os.Stderr, "journal: snapshot: %v\n", werr)
		return werr
	}
	if rerr := os.Rename(tmp, filepath.Join(j.Dir, "context.json")); rerr != nil {
		j.writeErrs++
		fmt.Fprintf(os.Stderr, "journal: snapshot rename: %v\n", rerr)
		return rerr
	}
	return nil
}

// WriteErrors — сколько событий не записалось (для честного финального отчёта).
func (j *Journal) WriteErrors() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.writeErrs
}

func (j *Journal) Close() {
	j.mu.Lock()
	n := j.writeErrs
	f := j.f
	j.f = nil
	j.mu.Unlock()
	if n > 0 {
		fmt.Fprintf(os.Stderr, "journal: %d ошибок записи за ран (журнал неполный!)\n", n)
	}
	if f != nil {
		_ = f.Close()
	}
}
