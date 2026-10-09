//go:build windows

// Package testjob вводит тестовый бинарь в Job Object с
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, чтобы порождённые дети умирали вместе с
// родителем.
//
// Зачем. Без этого шаг CI на windows-latest зависает намертво. Механизм
// воспроизведён и проверен: родитель без job порождает ребёнка, тот наследует
// дескриптор stdout и не завершается сам по себе; os/exec ждёт закрытия пайпа,
// EOF не наступает, команда не возвращает управление. Таймаут шага это не
// лечит — процесс надо убить, а его нечем.
//
// Модель self-adoption выбрана сознательно. Альтернатива «родитель создаёт job
// и присваивает каждого ребёнка после старта» требует CreateProcess с
// CREATE_SUSPENDED и гонки между созданием процесса и его присвоением:
// процесс успевает выполнить код между этими двумя шагами. Здесь же бинарь уже
// в job, и гонки нет в принципе — нечего защищать.
//
// Вложенные job разрешены начиная с Windows 8, поэтому наш job внутри job'а
// раннера (runner вешает шаги в свой) — норма, а не конфликт.
//
// Пакет обычный, а не _test.go: его импортируют разные тестовые пакеты
// (internal/core, internal/plugin и далее), и импорт _test.go-файла из другого
// пакета невозможен. Поэтому здесь лежит и сам механизм, и самопроверка через
// обычные функции, а каждый тестовый пакет вызывает их из своего TestMain.
//
// Handle намеренно НИКОГДА не закрывается до выхода из процесса, и это не
// упущение:
//   - KILL_ON_JOB_CLOSE срабатывает при закрытии последнего handle, то есть
//     закрытие здесь = убийство всех детей прямо посреди прогона;
//   - windows.Handle = uintptr, GC его не отслеживает, поэтому «автоматически
//     закроется при смерти объекта» не работает и нужен явный KeepAlive;
//   - после CloseHandle Windows может выдать то же числовое значение другому
//     объекту, поэтому повторное закрытие — это уже не «ошибка», а удар по
//     чужому handle. Единственный способ от этого уйти — не закрывать.
package testjob

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// selfJob — handle job'а, в который мы ввели себя. Присваивается один раз, при
// старте тестов; читается самопроверкой и удерживается Alive.
var selfJob windows.Handle

// Adopt вводит текущий процесс в собственный Job Object с
// KILL_ON_JOB_CLOSE. Вызывать из TestMain до m.Run().
//
// Fail-closed: любой отказ возвращается ошибкой, а не оставляет процесс без
// защиты. Вызывающий обязан на ошибке не запускать тесты (обычно os.Exit(1)) —
// иначе прогон идёт с той самой поломкой, ради устранения которой хелпер и
// вызывается. Идемпотентна: повторный вызов ничего не делает и возвращает nil,
// чтобы тестовый пакет мог звать её не думая, сколько раз у него TestMain.
func Adopt() error {
	if selfJob != 0 {
		return nil
	}
	job, err := adoptSelf()
	if err != nil {
		return err
	}
	selfJob = job
	return nil
}

// adoptSelf создаёт job с KILL_ON_JOB_CLOSE и вводит в него текущий процесс.
func adoptSelf() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
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
		return 0, fmt.Errorf("SetInformationJobObject: %w", err)
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
		return 0, fmt.Errorf("OpenProcess(self): %w", err)
	}
	err = windows.AssignProcessToJobObject(job, process)
	_ = windows.CloseHandle(process)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return job, nil
}

// KeepAlive удерживает handle job'а живым до самого выхода из процесса. Тестовый
// пакет зовёт его сразу после m.Run(), перед os.Exit: именно закрытие handle'а
// ядром и убивает оставшихся детей. Без этого вызова компилятор вправе
// считать selfJob мёртвым раньше времени.
func KeepAlive() {
	runtime.KeepAlive(selfJob)
}
