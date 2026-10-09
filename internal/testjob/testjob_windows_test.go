//go:build windows

package testjob

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestAdoptSetsKillOnCloseConfigured читает флаг обратно из ядра: проверяется не
// «Adopt вернул nil», а что KILL_ON_JOB_CLOSE действительно стоит на job'е.
func TestAdoptSetsKillOnCloseConfigured(t *testing.T) {
	if err := Adopt(); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if selfJob == 0 {
		t.Fatal("selfJob не создан")
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

// TestAdoptContainsSelf подтверждает, что мы действительно внутри своего job, а
// не просто создали его. ActiveProcesses == 1 означает, что в job ровно наш
// процесс.
func TestAdoptContainsSelf(t *testing.T) {
	if err := Adopt(); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if selfJob == 0 {
		t.Fatal("selfJob не создан")
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

// TestSelfTestAgrees — SelfTest обязан быть не слабее прямых проверок выше:
// после Adopt он проходит, а проверки читают то же самое состояние ядра.
func TestSelfTestAgrees(t *testing.T) {
	if err := Adopt(); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if err := SelfTest(); err != nil {
		t.Fatalf("SelfTest после успешного Adopt: %v", err)
	}
	if Handle() == 0 {
		t.Fatal("Handle() == 0 после Adopt")
	}
}

// TestAdoptIsIdempotent — повторный вызов не должен ни создавать второй job, ни
// падать: тестовый пакет вправе звать Adopt не думая, сколько раз.
func TestAdoptIsIdempotent(t *testing.T) {
	if err := Adopt(); err != nil {
		t.Fatalf("первый Adopt: %v", err)
	}
	first := selfJob
	if err := Adopt(); err != nil {
		t.Fatalf("второй Adopt: %v", err)
	}
	if selfJob != first {
		t.Fatalf("повторный Adopt создал новый job: %d → %d", first, selfJob)
	}
}
