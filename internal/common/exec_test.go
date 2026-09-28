package common

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Правило из common/exec.go проверяется на том, ради чего написано: команда,
// оставившая пережившего потомка, ОБЯЗАНА вернуться. Без WaitDelay такой
// вызов не возвращается никогда — и это не экзотика, а Windows-специфика,
// где потомок наследует stdin/stdout/stderr всегда (golang/go#60942).
//
// Потомка тест создаёт НАМЕРЕННО, но убирает за собой сам: оставлять после
// прогона живой процесс нельзя. Такой процесс держит пайпы родителя (на
// Windows это тот самый класс, который чинится) и вдобавок засоряет census
// в CI, где мы ищем именно такие выжившие процессы.
func TestOutputReturnsWhenDescendantHoldsPipe(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	cmd := leakDescendant("[Console]::Out.WriteLine('done')", pidFile)
	assertBoundedOutput(t, cmd)
	killDescendant(t, pidFile)
}

func TestCombinedOutputReturnsWhenDescendantHoldsPipe(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	cmd := leakDescendant("[Console]::Error.WriteLine('done')", pidFile)
	assertBoundedCombined(t, cmd)
	killDescendant(t, pidFile)
}

// leakDescendant — команда, которая печатает marker, запускает переживающего её
// потомка и выходит. PID потомка пишется в pidFile, а НЕ в stdout: сам потомок
// наследует наш stdout, и его вывод попал бы в проверяемый результат.
//
// marker передаётся как готовый фрагмент: на Windows это прямой вызов .NET,
// потому что `echo x 1>&2` в PowerShell — это Write-Output, который портит
// код возврата, и тест падал бы на стороне PowerShell, а не на стороне WaitDelay.
func leakDescendant(marker, pidFile string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// Start-Process -NoNewWindow -PassThru: потомок наследует наши stdio
		// (именно это и держит пайп), а его PID мы получаем через -PassThru.
		script := "$p = Start-Process -FilePath ping.exe -ArgumentList '-n','60','127.0.0.1' " +
			"-NoNewWindow -PassThru; " + marker + "; $p.Id | Set-Content -LiteralPath '" + pidFile + "'; exit 0"
		return exec.Command("powershell", "-NoProfile", "-Command", script)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		return nil
	}
	shMarker := strings.Replace(marker, "[Console]::Out.WriteLine('done')", "echo done", 1)
	shMarker = strings.Replace(shMarker, "[Console]::Error.WriteLine('done')", "echo done 1>&2", 1)
	return exec.Command(sh, "-c",
		"sleep 60 & echo $! > '"+pidFile+"'; "+shMarker+"; exit 0")
}

func killDescendant(t *testing.T, pidFile string) {
	t.Helper()
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Logf("файл с PID потомка не создан (%v) — убирать нечего", err)
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Errorf("не разобрали PID потомка из %q: %v", raw, err)
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	} else {
		_ = exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
	}
	t.Logf("потомок pid=%d убран", pid)
}

func assertBoundedOutput(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Skip("нет интерпретатора для запуска потомка")
	}
	start := time.Now()
	out, err := Output(cmd)
	elapsed := time.Since(start)
	if elapsed > ProcWaitDelay+15*time.Second {
		t.Fatalf("Output ждал %.1f с при пределе %s — утечка пайпа всё ещё не ограничена", elapsed.Seconds(), ProcWaitDelay)
	}
	// Утечка пайпа — не ошибка команды: процесс вышел, вывод мы получили.
	if err != nil {
		t.Fatalf("процесс завершился успешно, но Output вернул ошибку: %v", err)
	}
	if !strings.Contains(string(out), "done") {
		t.Errorf("вывод команды потерян: %q", out)
	}
}

func assertBoundedCombined(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Skip("нет интерпретатора для запуска потомка")
	}
	start := time.Now()
	out, err := CombinedOutput(cmd)
	elapsed := time.Since(start)
	if elapsed > ProcWaitDelay+15*time.Second {
		t.Fatalf("CombinedOutput ждал %.1f с при пределе %s", elapsed.Seconds(), ProcWaitDelay)
	}
	if err != nil {
		t.Fatalf("процесс завершился успешно, но CombinedOutput вернул ошибку: %v", err)
	}
	if !strings.Contains(string(out), "done") {
		t.Errorf("вывод команды потерян: %q", out)
	}
}

// Нормальная команда не должна платить за предел: WaitDelay ограничивает
// только ожидание ПОСЛЕ выхода, поэтому быстрый процесс возвращается сразу.
func TestOutputDoesNotDelayFastCommand(t *testing.T) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "echo ok")
	} else {
		sh, err := exec.LookPath("sh")
		if err != nil {
			t.Skip("sh недоступен")
		}
		cmd = exec.Command(sh, "-c", "echo ok")
	}
	start := time.Now()
	if _, err := Output(cmd); err != nil {
		t.Fatalf("Output: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("быстрая команда заняла %.1f с — предел добавляет задержку здоровым процессам", elapsed.Seconds())
	}
}
