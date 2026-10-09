//go:build windows

package testjob

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObjectBasicAccountingInformation — JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.
// В x/sys/windows нет ни этого типа, ни IsProcessInJob, поэтому структура
// описана здесь. Размер на windows/amd64 — 48 байт, выравнивание 8: четыре
// uint64 подряд (32 байта) и четыре uint32 подряд (16 байт), полей с нечётным
// смещением нет, поэтому неявной вставки не возникает.
type jobObjectBasicAccountingInformation struct {
	TotalUserTime             uint64
	TotalKernelTime           uint64
	ThisPeriodTotalUserTime   uint64
	ThisPeriodTotalKernelTime uint64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// SelfTest проверяет, что Adopt действительно сработал, читая состояние job'а
// обратно из ядра. Проверяется не «функция вернула nil», а что
// KILL_ON_JOB_CLOSE действительно стоит на job'е и что мы находимся внутри
// него.
//
// Вызывать из TestMain ДО m.Run() — как preflight-шаг. Если флажка нет, тесты
// бессмысленно гоняют процессы в среде, где осиротевшие дети переживут бинарь:
// лучше падать сразу и с внятным текстом. Возвращает ошибку, а не принимает
// testing.T, чтобы пакет оставался пригодным вне тестов.
func SelfTest() error {
	if selfJob == 0 {
		return errNoAdoption{}
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	err := windows.QueryInformationJobObject(
		selfJob,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	)
	if err != nil {
		return wrapQuery("KILL_ON_JOB_CLOSE", err)
	}
	if info.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		return errNoKillOnClose{flags: info.BasicLimitInformation.LimitFlags}
	}

	var basic jobObjectBasicAccountingInformation
	err = windows.QueryInformationJobObject(
		selfJob,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&basic)),
		uint32(unsafe.Sizeof(basic)),
		nil,
	)
	if err != nil {
		return wrapQuery("ActiveProcesses", err)
	}
	// ActiveProcesses == 1: в job ровно наш процесс, дети ещё не порождены.
	// Иначе мы создали job, но не вошли в него — и защиты нет.
	if basic.ActiveProcesses != 1 {
		return errUnexpectedProcesses{n: basic.ActiveProcesses}
	}
	return nil
}

// Handle возвращает handle нашего job'а. Нужен тестам, которые хотят копнуть
// глубже SelfTest. Вне Windows всегда 0.
func Handle() uintptr { return uintptr(selfJob) }
