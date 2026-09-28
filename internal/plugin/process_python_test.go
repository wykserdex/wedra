package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Регрессия на pythonInterpreter: в Windows в PATH почти всегда есть заглушка
// Microsoft Store (WindowsApps\python3.exe). Она находится через LookPath,
// печатает «Python was not found but can be installed from the Microsoft Store»
// и выходит с 9009. Раньше непроверенный кандидат всё равно возвращался, и
// каждый запуск плагина падал с 9009 и внятным только при внимательном чтении
// сообщением. Теперь такой кандидат пропускается, и следующий настоящий
// интерпретатор находится нормально.
func TestPythonInterpreterSkipsStubThatFailsProbe(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("заглушка Microsoft Store бывает только на Windows")
	}
	real, err := pythonInterpreter()
	if err != nil {
		t.Skipf("на этой машине нет настоящего python: %v", err)
	}

	dir := t.TempDir()
	stub := filepath.Join(dir, "python3.cmd")
	body := "@echo off\r\n" +
		"echo Python was not found but can be installed from the Microsoft Store: ms-windows-store://pdp/?productid=9NJ46SX7X90P 1>&2\r\n" +
		"exit /b 9009\r\n"
	if err := os.WriteFile(stub, []byte(body), 0o755); err != nil {
		t.Fatalf("не создать заглушку: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := pythonInterpreter()
	if err == nil {
		if samePath(got, stub) {
			t.Fatalf("вернулась заглушка %s — её нельзя возвращать после провала пробы", got)
		}
		if samePath(got, real) {
			return // штатный интерпретатор найден, заглушка пропущена
		}
		t.Logf("найден другой интерпретатор %s (заглушка пропущена)", got)
		return
	}
	// Ошибка — приемлемый исход, лишь бы не молча про заглушку.
	if !strings.Contains(err.Error(), "не проходит пробу") && !strings.Contains(err.Error(), "не найден интерпретатор") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
	if strings.Contains(err.Error(), stub) && !strings.Contains(err.Error(), "не проходит пробу") {
		t.Fatalf("ошибка упоминает заглушку как найденный интерпретатор: %v", err)
	}
}

func samePath(a, b string) bool {
	na := strings.ToLower(strings.TrimSpace(a))
	nb := strings.ToLower(strings.TrimSpace(b))
	return na != "" && nb != "" && (na == nb || strings.EqualFold(filepath.Clean(na), filepath.Clean(nb)))
}
