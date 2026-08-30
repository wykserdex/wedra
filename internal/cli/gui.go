package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"orchestrator/internal/api"
)

// RunGUI — orchestrator gui [--listen 127.0.0.1:8765] [--open]
// v0.22: консоль GUI (раны, live-терминал, DAG, запуск --yes из браузера).
// Запускать из корня репозитория (CWD: plugins/, examples/, var/runs/, web/static/).
func RunGUI(args []string) {
	listen := "127.0.0.1:8765"
	open := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 9 && a[:9] == "--listen=" {
			listen = a[9:]
		} else if a == "--listen" && i+1 < len(args) {
			listen = args[i+1]
			i++
		} else if len(a) > 7 && a[:7] == "--port=" {
			listen = "127.0.0.1:" + a[7:]
		} else if a == "--port" && i+1 < len(args) {
			listen = "127.0.0.1:" + args[i+1]
			i++
		} else if a == "--open" {
			open = true
		}
	}
	srv := api.NewServer("plugins", "examples", "var/runs")
	handler := srv.Routes()

	ver := api.Version
	if raw, err := os.ReadFile("VERSION"); err == nil {
		ver = strings.TrimSpace(string(raw))
	}
	fmt.Printf("▶ GUI v%s — http://%s\n", ver, listen)
	fmt.Println("  API: /api/health, /api/plugins, /api/pipelines, /api/runs, /api/run, /api/validate/pipeline")
	fmt.Println("  Frontend: web/static (консоль: раны, live-журнал, DAG; /editor/ — редактор пайплайнов)")
	fmt.Println("  Ctrl+C — остановить")

	if open {
		go func() {
			url := "http://" + listen
			if host, _, err := net.SplitHostPort(listen); err == nil && (host == "0.0.0.0" || host == "::") {
				url = "http://localhost:" + listen[len(host)+1:]
			}
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", url)
			case "windows":
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
			default:
				cmd = exec.Command("xdg-open", url)
			}
			_ = cmd.Start()
		}()
	}

	if err := http.ListenAndServe(listen, handler); err != nil {
		fmt.Println("ошибка сервера:", err)
		os.Exit(1)
	}
}
