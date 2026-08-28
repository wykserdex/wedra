package core

import "orchestrator/internal/journal"

// Journal — теперь в internal/journal (var/runs/<run_id>)
type Journal = journal.Journal

func NewJournal(dir string) (*Journal, error) {
	return journal.NewJournal(dir)
}
