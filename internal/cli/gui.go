package cli

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"orchestrator/internal/api"
)

func RunGUI(args []string) {
	port := "8080"
	open := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 7 && a[:7] == "--port=" {
			port = a[7:]
		} else if a == "--port" && i+1 < len(args) {
			port = args[i+1]
			i++
		} else if a == "--open" {
			open = true
		}
	}
	srv := api.NewServer("plugins", "pipelines", "var/runs")
	handler := srv.Routes()

	fmt.Printf("▶ GUI v%s — http://localhost:%s\n", api.Version, port)
	fmt.Println("  API: /api/health, /api/plugins, /api/pipelines, /api/runs, /api/validate/pipeline")
	fmt.Println("  Frontend: web/static/index.html (drag-and-drop, live YAML, JSON на линиях)")
	fmt.Println("  Нажми Ctrl+C чтобы остановить")

	if open {
		go func() {
			url := "http://localhost:" + port
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

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		fmt.Println("ошибка сервера:", err)
		os.Exit(1)
	}
}
