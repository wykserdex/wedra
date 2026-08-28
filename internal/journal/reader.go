package journal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// Reader — чтение journal.jsonl для отладки и будущего --resume
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
	for sc.Scan() {
		var ev map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, sc.Err()
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
