package common

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Правило из common/exec.go проверяется на том, ради чего написано: команда,
// оставившая пережившего потомка, ОБЯЗАНА вернуться. Без WaitDelay такой
// вызов не возвращается никогда — и это не экзотика, а Windows-специфика,
// где потомок наследует stdin/stdout/stderr всегда (golang/go#60942).
func TestOutputReturnsWhenDescendantHoldsPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Нужен интерпретатор, который умеет форкнуться; на Windows это cmd.
		// Маркер печатается ДО фона, чтобы доказать, что вывод команды мы не
		// потеряли, а «хвост» от потомка действительно отбросили.
		cmd := exec.Command("cmd", "/c", "echo done & start /b ping -n 60 127.0.0.1 >nul")
		assertBoundedOutput(t, cmd)
		return
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh недоступен")
	}
	// Потомок переживает команду и держит унаследованный stdout 60 с.
	cmd := exec.Command(sh, "-c", "sleep 60 & echo done; exit 0")
	assertBoundedOutput(t, cmd)
}

func TestCombinedOutputReturnsWhenDescendantHoldsPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "echo done 1>&2 & start /b ping -n 60 127.0.0.1 >nul")
		assertBoundedCombined(t, cmd)
		return
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh недоступен")
	}
	cmd := exec.Command(sh, "-c", "sleep 60 & echo done 1>&2; exit 0")
	assertBoundedCombined(t, cmd)
}

func assertBoundedOutput(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
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
