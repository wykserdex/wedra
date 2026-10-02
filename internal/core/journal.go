package core

import "github.com/wykserdex/wedra/internal/journal"

type Journal = journal.Journal

func NewJournal(dir string) (*Journal, error) {
	return journal.NewJournal(dir)
}

func OpenJournalAppend(dir string) (*Journal, error) {
	return journal.OpenJournalAppend(dir)
}
