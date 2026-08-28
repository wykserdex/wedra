package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type RunStore interface {
	Create(runID string) (*Journal, error)
	OpenAppend(runID string) (*Journal, error)
	AppendEvent(runID string, kind string, kv map[string]interface{}) error
	SaveArtifact(runID string, name string, data []byte) error
	LoadContext(runID string) (map[string]interface{}, error)
	MaxItemIndex(runID string) (int, error)
}

type FilesystemStore struct {
	BaseDir string
}

func NewFilesystemStore(baseDir string) *FilesystemStore {
	if baseDir == "" {
		baseDir = "var/runs"
	}
	return &FilesystemStore{BaseDir: baseDir}
}

func (s *FilesystemStore) runDir(runID string) string {
	return filepath.Join(s.BaseDir, runID)
}

func (s *FilesystemStore) Create(runID string) (*Journal, error) {
	return NewJournal(s.runDir(runID))
}

func (s *FilesystemStore) OpenAppend(runID string) (*Journal, error) {
	return OpenJournalAppend(s.runDir(runID))
}

func (s *FilesystemStore) AppendEvent(runID string, kind string, kv map[string]interface{}) error {
	j, err := s.OpenAppend(runID)
	if err != nil {
		return err
	}
	defer j.Close()
	j.Event(kind, kv)
	return nil
}

func (s *FilesystemStore) SaveArtifact(runID string, name string, data []byte) error {
	dir := filepath.Join(s.runDir(runID), "artifacts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	clean := filepath.Base(name)
	return os.WriteFile(filepath.Join(dir, clean), data, 0o644)
}

func (s *FilesystemStore) LoadContext(runID string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(filepath.Join(s.runDir(runID), "context.json"))
	if err != nil {
		return nil, fmt.Errorf("context.json: %w", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("context.json битый: %w", err)
	}
	return data, nil
}

func (s *FilesystemStore) MaxItemIndex(runID string) (int, error) {
	rd := NewReader(s.runDir(runID))
	events, err := rd.Events()
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return -1, err
	}
	maxIdx := -1
	for _, ev := range events {
		t, _ := ev["type"].(string)
		if t != "item_end" {
			continue
		}
		switch v := ev["item_index"].(type) {
		case float64:
			if int(v) > maxIdx {
				maxIdx = int(v)
			}
		case int:
			if v > maxIdx {
				maxIdx = v
			}
		case json.Number:
			if i, _ := v.Int64(); int(i) > maxIdx {
				maxIdx = int(i)
			}
		}
	}
	return maxIdx, nil
}

func (s *FilesystemStore) Load(runID string) (map[string]interface{}, error) {
	return s.LoadContext(runID)
}

type SQLiteStore struct {
	FilesystemStore
	DBPath string
}

func NewSQLiteStore(baseDir, dbPath string) *SQLiteStore {
	return &SQLiteStore{
		FilesystemStore: FilesystemStore{BaseDir: baseDir},
		DBPath:          dbPath,
	}
}
