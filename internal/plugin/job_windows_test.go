//go:build windows

package plugin

// Тестовый бинарь вводит САМ СЕБЯ в Job Object с JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// до старта любых детей. Тогда каждый потомок (python, go run, taskkill) рождается
// уже внутри job, и умирает вместе с нами — а не остаётся сиротой, держащим
// унаследованный дескриптор stdout, из-за чего EOF пайпа не наступает и шаг
// GitHub Actions на windows-latest виснет намертво.
//
// Модель self-adoption выбрана сознательно. Альтернатива «родитель создаёт job и
// присваивает каждого ребёнка после старта» требует CreateProcess с
// CREATE_SUSPENDED и гонки между созданием процесса и его присвоением: процесс
// успевает выполнить код между этими двумя шагами. Здесь же бинарь уже в job, и
// гонки нет в принципе — нечего защищать.
//
// Вложенные job разрешены начиная с Windows 8, поэтому наш job внутри job'а
// раннера (runner вешает шаги в свой) — норма, а не конфликт.
//
// Handle намеренно НИКОГДА не закрывается до os.Exit, и это не упущение:
//   - KILL_ON_JOB_CLOSE срабатывает при закрытии последнего handle, то есть
//     закрытие здесь = убийство всех детей прямо посреди прогона;
//   - windows.Handle = uintptr, GC его не отслеживает, поэтому «автоматически
//     закроется при смерти объекта» не работает и нужен явный KeepAlive;
//   - после CloseHandle Windows может выдать то же числовое значение другому
//     объекту, поэтому повторное закрытие — это уже не «ошибка», а удар по
//     чужому handle. Единственный способ от этого уйти — не закрывать.

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObjectBasicAccountingInformation — JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.
// В x/sys/windows нет ни этого типа, ни IsProcessInJob, поэтому структура
// описана здесь. Размер на windows/amd64 — 48 байт, выравнивание 8:
// четыре uint64 подряд (32 байта) и четыре uint32 подряд (16 байт), полей с
// нечётным смещением нет, поэтому неявной вставки не возникает.
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

// selfJob — handle job'а, в который мы ввели себя. Ровно одно присваивание,
// при старте тестов; читается только для самопроверки и KeepAlive.
var selfJob windows.Handle

// adoptSelfIntoKillOnCloseJob создаёт job с KILL_ON_JOB_CLOSE и вводит в него
// текущий процесс. Fail-closed: любой отказ возвращается ошибкой, а не
// оставляет нас без защиты.
func adoptSelfIntoKillOnCloseJob() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("CreateJobObject: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("SetInformationJobObject: %w", err)
	}

	// AssignProcessToJobObject требует PROCESS_SET_QUOTA | PROCESS_TERMINATE
	// на целевом процессе; открываем себя по PID.
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(os.Getpid()),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("OpenProcess(self): %w", err)
	}
	err = windows.AssignProcessToJobObject(job, process)
	_ = windows.CloseHandle(process)
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}

	selfJob = job
	return nil
}

func TestMain(m *testing.M) {
	if err := adoptSelfIntoKillOnCloseJob(); err != nil {
		fmt.Fprintf(os.Stderr, "job object: self-adoption failed, tests would leave orphans: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	// Handle жив до самого os.Exit: именно его закрытие ядром и убивает job.
	runtime.KeepAlive(selfJob)
	os.Exit(code)
}

// TestSelfJobKillOnCloseConfigured читает флаг обратно из ядра. Проверяет не
// «функция вернула nil», а что KILL_ON_JOB_CLOSE действительно стоит на job'е.
func TestSelfJobKillOnCloseConfigured(t *testing.T) {
	if selfJob == 0 {
		t.Fatal("selfJob не создан: TestMain не выполнил self-adoption")
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
		t.Fatalf("QueryInformationJobObject: %v", err)
	}

	if info.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Fatalf("KILL_ON_JOB_CLOSE не установлен, LimitFlags=0x%08x",
			info.BasicLimitInformation.LimitFlags)
	}
}

// TestSelfJobContainsSelf подтверждает, что мы действительно внутри своего job,
// а не просто создали его. ActiveProcesses == 1 означает, что в job ровно наш
// процесс: дети ещё не порождены.
func TestSelfJobContainsSelf(t *testing.T) {
	if selfJob == 0 {
		t.Fatal("selfJob не создан: TestMain не выполнил self-adoption")
	}

	var basic jobObjectBasicAccountingInformation
	err := windows.QueryInformationJobObject(
		selfJob,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&basic)),
		uint32(unsafe.Sizeof(basic)),
		nil,
	)
	if err != nil {
		t.Fatalf("QueryInformationJobObject: %v", err)
	}
	if basic.ActiveProcesses != 1 {
		t.Fatalf("ActiveProcesses=%d, ожидался 1 (только мы)", basic.ActiveProcesses)
	}
}
