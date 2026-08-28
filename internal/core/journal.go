package core

import "orchestrator/internal/journal"

type Journal = journal.Journal

func NewJournal(dir string) (*Journal, error) {
	return journal.NewJournal(dir)
}

func OpenJournalAppend(dir string) (*Journal, error) {
	return journal.OpenJournalAppend(dir)
}
