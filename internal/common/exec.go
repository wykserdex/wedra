package common

import (
	"os/exec"
	"time"
)

// ProcWaitDelay — сколько ждём закрытия пайпов вывода ПОСЛЕ выхода процесса.
//
// cmd.Wait() без WaitDelay ждёт закрытия пайпов бесконечно. На Windows потомок
// наследует stdin/stdout/stderr всегда (golang/go#60942), поэтому короткоживущая
// команда вроде `git` или пробного запуска интерпретатора может оставить
// пережившего её потомка — и тогда Wait() не возвращается никогда.
//
// Правило живёт здесь, а не на каждом вызывающем сайте: тот же баг уже
// возникал дважды именно потому, что одно и то же решение копировалось по
// коду (сначала в гейте сети, потом в Wait). Одно место — одно правило.
//
// Величина не добавляет задержки нормальным процессам: они пишут вывод до
// выхода, и WaitDelay платит только тот, кто действительно что-то утек.
const ProcWaitDelay = 5 * time.Second

// Output — exec с ограниченным ожиданием пайпов вместо бесконечного.
//
// ErrWaitDelay (пайп не закрылся) здесь НЕ является ошибкой команды: процесс
// до этого вышел, а вывод мы получили весь, что успел. Теряется только «хвост»
// от пережившего потомка, которого команда не писала. Все вызывающие
// места — пробы и вспомогательные команды, для которых важно завершение и
// артефакт на диске, а не полнота вывода; там, где вывод критичен (плагин),
// разбор ErrWaitDelay сделан явно в internal/plugin/process.go.
func Output(cmd *exec.Cmd) ([]byte, error) {
	cmd.WaitDelay = ProcWaitDelay
	out, err := cmd.Output()
	if isWaitDelay(err) {
		return out, nil
	}
	return out, err
}

// CombinedOutput — то же для слияния stdout и stderr.
func CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	cmd.WaitDelay = ProcWaitDelay
	out, err := cmd.CombinedOutput()
	if isWaitDelay(err) {
		return out, nil
	}
	return out, err
}

func isWaitDelay(err error) bool {
	return err != nil && err == exec.ErrWaitDelay
}
