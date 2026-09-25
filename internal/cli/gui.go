package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"wedra/internal/api"
	"wedra/internal/guidirs"
)

// RunGUI — wedra gui [--listen 127.0.0.1:8765] [--open]
// Каталоги: --plugins, --pipelines, --runs-dir; значения относительны CWD.
// По умолчанию: plugins/, examples/, var/runs/.
func RunGUI(args []string) {
	dirs, remaining, err := guidirs.Parse(args, guidirs.Default())
	if err != nil {
		fmt.Println("gui:", err)
		os.Exit(2)
	}
	if err := dirs.Ensure(); err != nil {
		fmt.Println("gui:", err)
		os.Exit(1)
	}
	args = remaining
	listen := "127.0.0.1:8765"
	open := false
	noSession := false
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
		} else if a == "--no-session" {
			noSession = true
		}
	}
	srv := api.NewServer(dirs.Plugins, dirs.Pipelines, dirs.Runs)
	// v0.9: сессия человека — мутации (запуск, гейт, отмена) только с cookie
	// из ссылки ?k=, напечатанной в ЭТОТ терминал. --no-session — старое
	// поведение (любой локальный процесс может одобрить гейт).
	secret := ""
	if !noSession {
		var err error
		if secret, err = api.NewSessionSecret(); err != nil {
			fmt.Println("не удалось сгенерировать ключ сессии:", err)
			os.Exit(1)
		}
		srv.EnableSession(secret)
	}
	ver := api.Version
	if raw, err := os.ReadFile("VERSION"); err == nil {
		ver = strings.TrimSpace(string(raw))
	}
	link := "http://" + listen + "/"
	if host, port, err := net.SplitHostPort(listen); err == nil && (host == "0.0.0.0" || host == "::" || host == "") {
		link = "http://localhost:" + port + "/"
	}
	if secret != "" {
		link += "?k=" + secret
	}
	fmt.Printf("▶ GUI v%s — %s\n", ver, link)
	if secret != "" {
		fmt.Println("  ссылка с ключом — только для человека: без неё запуск/гейты/отмена → 401")
	} else {
		fmt.Println("  ⚠ --no-session: любой локальный процесс может запускать раны и одобрять гейты")
	}
	fmt.Println("  API: /api/health, /api/plugins, /api/pipelines, /api/runs, /api/run, /api/validate/pipeline")
	fmt.Println("  Frontend: web/static с диска (если виден из CWD) или встроенный GUI (go:embed, v0.7) — консоль: раны, live-журнал, DAG; /editor/ — редактор пайплайнов")
	fmt.Println("  Ctrl+C — остановить")

	if open {
		go func() {
			url := link
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

	server := srv.HTTPServer(listen)
	if err := server.ListenAndServe(); err != nil {
		fmt.Println("ошибка сервера:", err)
		os.Exit(1)
	}
}
