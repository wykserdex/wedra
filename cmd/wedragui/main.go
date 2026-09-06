// Command wedragui — WEDRA desktop (v0.8): двойной клик = окно с GUI.
//
// Внутри: тот же встроенный сервер, что у `wedra gui` (go:embed, v0.7) —
// консоль (раны, live-журнал, DAG) + редактор (/editor/). На Windows GUI
// живёт в собственном окне (WebView2, чистый Go, без CGO); если рантайм
// WebView2 не найден — честно падаем в браузер (URL в консоли и в логе).
//
// Каталоги — рядом с exe (CWD при двойном клике = папка exe):
//
//	plugins/    — плагины (wedra plugin install / tool)
//	pipelines/  — YAML-пайплайны (консоль их видит в списке)
//	runs/       — журналы ранов
//
// Лог: %APPDATA%/WEDRA/wedragui.log (Windows) / $XDG_CONFIG_HOME/WEDRA/.
// Выход: закрыть окно (на фолбэк-пути — закрыть консоль / Ctrl+C).
package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"wedra/internal/api"
)

func main() {
	var port int
	debug := false
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		switch {
		case strings.HasPrefix(a, "--port="):
			fmt.Sscanf(a[7:], "%d", &port)
		case a == "--port" && i+1 < len(os.Args):
			fmt.Sscanf(os.Args[i+1], "%d", &port)
			i++
		case a == "--debug":
			debug = true
		}
	}

	// каталоги рядом с exe (CWD при двойном клике = папка exe)
	for _, d := range []string{"plugins", "pipelines", "runs"} {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Println("каталог", d, ":", err)
			os.Exit(1)
		}
	}

	srv := api.NewServer("plugins", "pipelines", "runs")

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		fmt.Println("порт недоступен:", err)
		os.Exit(1)
	}
	url := "http://" + ln.Addr().String()

	logPath, logFile := openLog()
	defer logFile.Close()

	ver := api.Version
	if raw, e := os.ReadFile("VERSION"); e == nil {
		ver = strings.TrimSpace(string(raw))
	}

	logf := func(format string, a ...interface{}) {
		line := "[" + time.Now().Format("15:04:05") + "] " + fmt.Sprintf(format, a...) + "\n"
		fmt.Print(line)
		if logFile != nil {
			io.WriteString(logFile, line)
		}
	}

	logf("WEDRA desktop v%s — GUI: %s", ver, url)
	logf("каталоги: %s (plugins/, pipelines/, runs/)", workingDir())
	logf("лог: %s", logPath)
	fmt.Println("  Закрыть окно (или Ctrl+C) — остановить WEDRA.")

	httpErr := make(chan error, 1)
	go func() { httpErr <- http.Serve(ln, srv.Routes()) }()

	// окно (Windows) или браузер + ожидание (остальные ОС / фолбэк)
	desktop(url, debug, logf)

	ln.Close()
	select {
	case err := <-httpErr:
		if err != nil && !strings.Contains(err.Error(), "closed") {
			logf("сервер: %v", err)
		}
	default:
	}
}

// openLog — лог в каталоге настроек: %APPDATA%/WEDRA (Windows),
// $XDG_CONFIG_HOME/WEDRA (unix). Фолбэк — рядом с exe.
func openLog() (string, *os.File) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir, _ = os.Getwd()
	}
	dir = filepath.Join(dir, "WEDRA")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil
	}
	p := filepath.Join(dir, "wedragui.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return p, nil
	}
	return p, f
}

func workingDir() string {
	d, err := os.Getwd()
	if err != nil {
		return "."
	}
	return d
}

// openBrowser — URL в системный браузер (фолбэк и dev-путь).
func openBrowser(url string) {
	var (
		name string
		args []string
	)
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		fmt.Println("  (браузер не открылся:", err.Error(), "— URL выше)")
	}
}

// waitSignal — ждать Ctrl+C / SIGTERM (фолбэк-путь: консоль открыта).
func waitSignal() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}
