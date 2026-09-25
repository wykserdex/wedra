package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const maxJournalEventSize = 16 << 20

type Reader struct {
	Dir string
}

func NewReader(dir string) *Reader {
	return &Reader{Dir: dir}
}

func (r *Reader) Events() ([]map[string]interface{}, error) {
	f, err := os.Open(filepath.Join(r.Dir, "journal.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []map[string]interface{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), maxJournalEventSize)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var ev map[string]interface{}
		if err := json.Unmarshal(raw, &ev); err != nil {
			return nil, fmt.Errorf("journal: строка %d: %w", line, err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("journal: чтение: %w", err)
	}
	return events, nil
}

func (r *Reader) ContextSnapshot() (map[string]interface{}, error) {
	raw, err := os.ReadFile(filepath.Join(r.Dir, "context.json"))
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}
