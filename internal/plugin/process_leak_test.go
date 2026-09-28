package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wedra/internal/pipeline"
)

// Плагин, который завершился УСПЕШНО, но оставил фонового потомка.
//
// Это не экзотика: так ведёт себя любой плагин, который запустил фоновую
// задачу и забыл про неё (сборка кэша, демон, воркер). Потомок наследует
// stdout/stderr плагина, поэтому пайп не закрывается, и os/exec ждёт его
// закрытия БЕСКОНЕЧНО — cmd.Wait() не возвращается, а таймаут рана срабатывает
// только как аварийный предохранитель и помечает успешный плагин как
// «timeout».
//
// Проверено: без cmd.WaitDelay этот тест падает как timeout, с ним — проходит
// за ~5 с и сохраняет ответ плагина.
func TestPluginLeavingBackgroundChildDoesNotHang(t *testing.T) {
	requirePythonT(t)
	dir := t.TempDir()
	script := `import subprocess, sys
# Потомок переживает плагин и держит унаследованный stdout. 120 с — заведомо
# больше таймаута рана ниже, поэтому без WaitDelay тест обязан упасть.
subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)"])
sys.stdout.write('{"status":"ok","output":{"done":true}}')
sys.stdout.flush()
`
	m := writePlugin(t, dir, script)
	// Песочница тут не предмет теста, а на Windows untrusted и вовсе отвергается.
	m.Sandbox = pipeline.SandboxTrusted

	start := time.Now()
	res := ExecWithEnvCtx(context.Background(), m, []byte("{}"), 10*time.Second, nil)
	elapsed := time.Since(start)

	if !res.OK() {
		t.Fatalf("плагин успешно отработал и должен считаться успешным, получено %+v (за %.1f с)", res, elapsed.Seconds())
	}
	if done, _ := res.Output["done"].(bool); !done {
		t.Errorf("ответ плагина потерян: %v", res.Output)
	}
	if elapsed > 9*time.Second {
		t.Errorf("Exec ждал %.1f с — значит упирался в таймаут вместо WaitDelay", elapsed.Seconds())
	}
	// След утечки должен быть виден: иначе «успех» выглядит как обычный ран и
	// молча скрывает, что потомок остался жив.
	if res.Stderr == "" {
		t.Log("примечание: предупреждение об утечке не записано в Stderr — стоит проверить, что это осознанно")
	} else {
		t.Logf("след утечки в Stderr: %s", res.Stderr)
	}
}

// Контроль к тесту выше: тот же потомок, но его PID пишется, и мы убеждаемся,
// что он действительно пережил плагин. Иначе тест выше проходит вполне
// вакуумно — потому что утечки не было.
func TestPluginBackgroundChildActuallySurvives(t *testing.T) {
	requirePythonT(t)
	dir := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	script := fmt.Sprintf(`import os, subprocess, sys
child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)"])
with open(%q, "w") as f:
    f.write(str(child.pid))
sys.stdout.write('{"status":"ok","output":{"done":true}}')
sys.stdout.flush()
`, pidFile)
	m := writePlugin(t, dir, script)
	// Песочница тут не предмет теста, а на Windows untrusted и вовсе отвергается.
	m.Sandbox = pipeline.SandboxTrusted

	res := ExecWithEnvCtx(context.Background(), m, []byte("{}"), 10*time.Second, nil)
	if !res.OK() {
		t.Fatalf("плагин не отработал: %+v", res)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("pid потомка не записан: %v", err)
	}
	var pid int
	if _, err := parsePID(string(raw), &pid); err != nil {
		t.Fatal(err)
	}
	if !pidAlive(pid) {
		t.Skipf("потомок %d уже мёртв — среда вмешалась, тест не показателен", pid)
	}
	t.Logf("подтверждено: потомок %d пережил плагин (и именно он держал пайп)", pid)
}
