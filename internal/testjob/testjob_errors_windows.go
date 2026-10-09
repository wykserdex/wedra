//go:build windows

package testjob

import "fmt"

type errNoAdoption struct{}

func (errNoAdoption) Error() string {
	return "job object не создан: TestMain не вызвал testjob.Adopt() — тесты оставили бы осиротевших детей"
}

type errNoKillOnClose struct{ flags uint32 }

func (e errNoKillOnClose) Error() string {
	return fmt.Sprintf("KILL_ON_JOB_CLOSE не установлен на job'е, LimitFlags=0x%08x — дети переживут тестовый бинарь", e.flags)
}

type errUnexpectedProcesses struct{ n uint32 }

func (e errUnexpectedProcesses) Error() string {
	return fmt.Sprintf("ActiveProcesses=%d, ожидался 1 (только тестовый бинарь) — процесс не вошёл в свой job", e.n)
}

func wrapQuery(what string, err error) error {
	return fmt.Errorf("QueryInformationJobObject(%s): %w", what, err)
}
