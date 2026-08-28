package journal

// RunStore — абстракция хранилища прогонов (feedback: runs/ не должен быть рядом с исходниками)
// Реализации: filesystem (var/runs), SQLite, S3/PostgreSQL для сервера
type RunStore interface {
	Create(runID string) (*Journal, error)
	AppendEvent(runID string, kind string, kv map[string]interface{}) error
	SaveArtifact(runID string, name string, data []byte) error
	Load(runID string) (map[string]interface{}, error)
}

// FilesystemStore — локальная реализация для MVP
type FilesystemStore struct {
	BaseDir string // var/runs или runs
}

func NewFilesystemStore(baseDir string) *FilesystemStore {
	if baseDir == "" {
		baseDir = "var/runs"
	}
	return &FilesystemStore{BaseDir: baseDir}
}

func (s *FilesystemStore) Create(runID string) (*Journal, error) {
	return NewJournal(s.BaseDir + "/" + runID)
}

func (s *FilesystemStore) AppendEvent(runID string, kind string, kv map[string]interface{}) error {
	// для MVP — через Journal.Event, здесь заглушка
	return nil
}

func (s *FilesystemStore) SaveArtifact(runID string, name string, data []byte) error {
	// var/runs/<run-id>/artifacts/
	return nil
}

func (s *FilesystemStore) Load(runID string) (map[string]interface{}, error) {
	return nil, nil
}
