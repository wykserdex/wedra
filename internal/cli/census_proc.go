package cli

// Платформенные куски census: запуск go test с выводом в файл, ожидание
// процесса с пределом, убийство дерева и скриншот рабочего стола.
//
// Вывод go test идёт в ФАЙЛ, а не в пайп нашего процесса. Причина не в
// стиле: и в .NET, и в os/exec ожидание завершения может зацепиться за EOF
// унаследованных пайпов, и тогда потомок, удерживающий конец пайпа,
// блокирует шаг ПОСЛЕ того, как всё отработало. Файловый дескриптор, который
// кто-то держит, не блокирует никого.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type waited struct {
	exited bool
	code   int
}

func goTestCommand(dir, pkg string, goTimeoutSec int, logFile *os.File) *exec.Cmd {
	cmd := exec.Command("go", "test", pkg, "-count=1",
		"-timeout", strconv.Itoa(goTimeoutSec)+"s")
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	applyProcAttr(cmd)
	return cmd
}

// waitProcess ждёт процесс, НЕ читая его вывод. Это важно: если вывод идёт
// пайпом и его никто не забирает, он упирается в размер буфера и процесс
// встаёт сам по себе. У нас вывод в файле, поэтому безопасно.
func waitProcess(cmd *exec.Cmd, seconds int) waited {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return waited{exited: true, code: exitCode(err)}
	case <-time.After(time.Duration(seconds) * time.Second):
		return waited{exited: false, code: -1}
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// windowsScreenshot снимает рабочий стол средствами GDI. Снимок не справится,
// если графической сессии нет (сервис без десктопа) — тогда уликой остаётся
// procs.txt, и это честнее пустого PNG.
func windowsScreenshot() ([]byte, error) {
	if !isWindows() {
		return nil, fmt.Errorf("снимок только для Windows")
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("wedra-census-shot-%d.png", time.Now().UnixNano()))
	defer os.Remove(tmp)
	ps := `Add-Type -AssemblyName System.Windows.Forms,System.Drawing;` +
		`$b=[System.Windows.Forms.SystemInformation]::VirtualScreen;` +
		`$bmp=New-Object System.Drawing.Bitmap $b.Width,$b.Height;` +
		`$g=[System.Drawing.Graphics]::FromImage($bmp);` +
		`$g.CopyFromScreen($b.Left,$b.Top,0,0,$bmp.Size);` +
		`$bmp.Save('` + tmp + `',[System.Drawing.Imaging.ImageFormat]::Png);` +
		`$g.Dispose();$bmp.Dispose()`
	if _, err := runCapture(".", "powershell", "-NoProfile", "-Command", ps); err != nil {
		return nil, err
	}
	return os.ReadFile(tmp)
}
