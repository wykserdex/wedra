package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type RunStore interface {
	Create(runID string) (*Journal, error)
	OpenAppend(runID string) (*Journal, error)
	AppendEvent(runID string, kind string, kv map[string]interface{}) error
	SaveArtifact(runID string, name string, data []byte) error
	LoadContext(runID string) (map[string]interface{}, error)
	MaxItemIndex(runID string) (int, error)
	ListArtifacts(runID string) ([]string, error)
	LoadArtifact(runID string, name string) ([]byte, error)
	ListRuns() ([]string, error)
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

func (s *FilesystemStore) ListArtifacts(runID string) ([]string, error) {
	dir := filepath.Join(s.runDir(runID), "artifacts")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range ents {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func (s *FilesystemStore) LoadArtifact(runID string, name string) ([]byte, error) {
	clean := filepath.Base(name)
	p := filepath.Join(s.runDir(runID), "artifacts", clean)
	return os.ReadFile(p)
}

func (s *FilesystemStore) ListRuns() ([]string, error) {
	ents, err := os.ReadDir(s.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// SQLiteStore — real implementation with single DB file (JSON) + FS fallback
// v0.14: single file var/runs/runs.db (JSON) with tables runs/events/artifacts
// No CGO, pure Go, works as SQLite-like store. Can be swapped to real sqlite driver later.

type dbFile struct {
	Runs      []dbRun      `json:"runs"`
	Events    []dbEvent    `json:"events"`
	Artifacts []dbArtifact `json:"artifacts"`
}

type dbRun struct {
	ID        string `json:"id"`
	Pipeline  string `json:"pipeline"`
	CreatedAt string `json:"created_at"`
}

type dbEvent struct {
	ID        int                    `json:"id"`
	RunID     string                 `json:"run_id"`
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	ItemIndex *int                   `json:"item_index,omitempty"`
}

type dbArtifact struct {
	ID     int    `json:"id"`
	RunID  string `json:"run_id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
}

type SQLiteStore struct {
	FilesystemStore
	DBPath string
	mu     sync.Mutex
}

func NewSQLiteStore(baseDir, dbPath string) *SQLiteStore {
	if baseDir == "" {
		baseDir = "var/runs"
	}
	if dbPath == "" {
		dbPath = filepath.Join(baseDir, "runs.db")
	}
	return &SQLiteStore{
		FilesystemStore: FilesystemStore{BaseDir: baseDir},
		DBPath:          dbPath,
	}
}

func (s *SQLiteStore) loadDB() (*dbFile, error) {
	if _, err := os.Stat(s.DBPath); os.IsNotExist(err) {
		return &dbFile{}, nil
	}
	raw, err := os.ReadFile(s.DBPath)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return &dbFile{}, nil
	}
	var db dbFile
	if err := json.Unmarshal(raw, &db); err != nil {
		return &dbFile{}, nil
	}
	return &db, nil
}

func (s *SQLiteStore) saveDB(db *dbFile) error {
	if err := os.MkdirAll(filepath.Dir(s.DBPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.DBPath, raw, 0o644)
}

func (s *SQLiteStore) Create(runID string) (*Journal, error) {
	j, err := s.FilesystemStore.Create(runID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	db, _ := s.loadDB()
	// check exists
	for _, r := range db.Runs {
		if r.ID == runID {
			return j, nil
		}
	}
	db.Runs = append(db.Runs, dbRun{ID: runID})
	_ = s.saveDB(db)
	return j, nil
}

func (s *SQLiteStore) AppendEvent(runID string, kind string, kv map[string]interface{}) error {
	_ = s.FilesystemStore.AppendEvent(runID, kind, kv)
	s.mu.Lock()
	defer s.mu.Unlock()
	db, _ := s.loadDB()
	var itemIdx *int
	if v, ok := kv["item_index"]; ok {
		switch vv := v.(type) {
		case float64:
			i := int(vv)
			itemIdx = &i
		case int:
			itemIdx = &vv
		case int64:
			i := int(vv)
			itemIdx = &i
		}
	}
	ev := dbEvent{
		ID:        len(db.Events) + 1,
		RunID:     runID,
		Type:      kind,
		Data:      kv,
		ItemIndex: itemIdx,
	}
	db.Events = append(db.Events, ev)
	return s.saveDB(db)
}

func (s *SQLiteStore) SaveArtifact(runID string, name string, data []byte) error {
	if err := s.FilesystemStore.SaveArtifact(runID, name, data); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	db, _ := s.loadDB()
	clean := filepath.Base(name)
	p := filepath.Join(s.runDir(runID), "artifacts", clean)
	db.Artifacts = append(db.Artifacts, dbArtifact{
		ID:    len(db.Artifacts) + 1,
		RunID: runID,
		Name:  clean,
		Path:  p,
	})
	return s.saveDB(db)
}

func (s *SQLiteStore) MaxItemIndex(runID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, err := s.loadDB()
	if err != nil {
		return s.FilesystemStore.MaxItemIndex(runID)
	}
	maxIdx := -1
	for _, ev := range db.Events {
		if ev.RunID != runID {
			continue
		}
		if ev.Type != "item_end" {
			continue
		}
		if ev.ItemIndex != nil && *ev.ItemIndex > maxIdx {
			maxIdx = *ev.ItemIndex
		} else if ev.Data != nil {
			if v, ok := ev.Data["item_index"]; ok {
				switch vv := v.(type) {
				case float64:
					if int(vv) > maxIdx {
						maxIdx = int(vv)
					}
				case int:
					if vv > maxIdx {
						maxIdx = vv
					}
				}
			}
		}
	}
	if maxIdx == -1 {
		// fallback to FS if DB empty
		return s.FilesystemStore.MaxItemIndex(runID)
	}
	return maxIdx, nil
}

func (s *SQLiteStore) ListArtifacts(runID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, err := s.loadDB()
	if err != nil {
		return s.FilesystemStore.ListArtifacts(runID)
	}
	out := []string{}
	for _, a := range db.Artifacts {
		if a.RunID == runID {
			out = append(out, a.Name)
		}
	}
	if len(out) == 0 {
		return s.FilesystemStore.ListArtifacts(runID)
	}
	return out, nil
}

func (s *SQLiteStore) LoadArtifact(runID string, name string) ([]byte, error) {
	return s.FilesystemStore.LoadArtifact(runID, name)
}

func (s *SQLiteStore) ListRuns() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	db, err := s.loadDB()
	if err != nil {
		return s.FilesystemStore.ListRuns()
	}
	if len(db.Runs) == 0 {
		return s.FilesystemStore.ListRuns()
	}
	out := []string{}
	for _, r := range db.Runs {
		out = append(out, r.ID)
	}
	return out, nil
}

func (s *SQLiteStore) Close() error { return nil }
