package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"wedra/internal/api"
	"wedra/internal/gate"
	"wedra/internal/mcp"
)

// RunMCP — wedra mcp --plugins=<dir>... [--workdir=<dir>] [--runs-dir=<dir>]
// [--gui-listen=127.0.0.1:0] [--no-gui] [--no-open] [--print-link].
//
// Stdio JSON-RPC. Изоляция (Фаза 4.2): proto = исходный stdout, os.Stdout →
// stderr, раны Quiet, терминальный гейт запрещён (MCPMode).
//
// Гейты: MCP-процесс поднимает на 127.0.0.1 встроенную консоль с сессией
// человека. Когда ран агента доходит до human_gate, браузер человека
// открывается на этом ране (ссылка с ключом); агент получает только адрес
// без ключа. Решение принимает человек в браузере → source: gui.
func RunMCP(args []string) {
	var plugins []string
	workdir, runsdir := "", ""
	guiListen := "127.0.0.1:0"
	noGUI, noOpen, printLink := false, false, false
	// docs/mcp.md и конфиги клиентов пишут "--plugins /abs" (через пробел):
	// нормализуем в "--plugins=/abs" до разбора.
	args = joinFlagValues(args, "--plugins", "--plugin", "--workdir", "--runs-dir", "--gui-listen")
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--plugins="):
			plugins = append(plugins, strings.TrimPrefix(a, "--plugins="))
		case strings.HasPrefix(a, "--plugin="):
			plugins = append(plugins, strings.TrimPrefix(a, "--plugin="))
		case strings.HasPrefix(a, "--workdir="):
			workdir = strings.TrimPrefix(a, "--workdir=")
		case strings.HasPrefix(a, "--runs-dir="):
			runsdir = strings.TrimPrefix(a, "--runs-dir=")
		case strings.HasPrefix(a, "--gui-listen="):
			guiListen = strings.TrimPrefix(a, "--gui-listen=")
		case a == "--no-gui":
			noGUI = true
		case a == "--no-open":
			noOpen = true
		case a == "--print-link":
			printLink = true
		case a == "--help" || a == "-h":
			fmt.Fprintln(os.Stderr, "wedra mcp --plugins=<dir> [--plugins=<dir>...] [--workdir=<dir>] [--runs-dir=<dir>]")
			fmt.Fprintln(os.Stderr, "          [--gui-listen=127.0.0.1:0] [--no-gui] [--no-open] [--print-link]")
			return
		default:
			fmt.Fprintf(os.Stderr, "неизвестный аргумент %q (см. --help)\n", a)
			os.Exit(2)
		}
	}
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	absWork, _ := filepath.Abs(workdir)
	if runsdir == "" {
		runsdir = filepath.Join(absWork, "var", "runs")
	}
	if raw, err := os.ReadFile(filepath.Join(absWork, "VERSION")); err == nil {
		mcp.Version = strings.TrimSpace(string(raw))
	}

	// Изоляция stdout: JSON-RPC — только в исходный stdout, всё остальное — в stderr.
	proto := os.Stdout
	os.Stdout = os.Stderr

	opts := mcp.Options{PluginsDirs: plugins, WorkDir: absWork, RunsDir: runsdir}
	if !noGUI {
		h, err := startHumanConsole(guiListen, plugins, absWork, runsdir, !noOpen, printLink)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mcp: консоль для гейтов не поднялась:", err, "(гейты будут недоступны)")
		} else {
			opts.Human = h
		}
	}
	srv, err := mcp.NewServer(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp:", err)
		os.Exit(1)
	}
	t := mcp.NewTransport(os.Stdin, proto)
	if err := srv.Serve(t); err != nil {
		if !strings.Contains(err.Error(), "EOF") {
			fmt.Fprintln(os.Stderr, "mcp:", err)
		}
	}
}

// humanConsole — mcp.HumanChannel поверх встроенного api.Server.
type humanConsole struct {
	srv    *api.Server
	base   string // http://127.0.0.1:PORT (без ключа)
	secret string
	open   bool
	print  bool
}

func startHumanConsole(listen string, plugins []string, workdir, runsDir string, open, printLink bool) (*humanConsole, error) {
	pluginsDir := filepath.Join(workdir, "plugins")
	if len(plugins) > 0 {
		pluginsDir = plugins[0]
		if !filepath.IsAbs(pluginsDir) {
			pluginsDir = filepath.Join(workdir, pluginsDir)
		}
	}
	if !filepath.IsAbs(runsDir) {
		runsDir = filepath.Join(workdir, runsDir)
	}
	srv := api.NewServer(pluginsDir, filepath.Join(workdir, "examples"), runsDir)
	secret, err := api.NewSessionSecret()
	if err != nil {
		return nil, err
	}
	srv.EnableSession(secret)
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	go func() { _ = srv.HTTPServer(ln.Addr().String()).Serve(ln) }()
	base := "http://" + ln.Addr().String()
	fmt.Fprintf(os.Stderr, "wedra mcp: консоль гейтов %s (ключ сессии — только в браузере человека)\n", base)
	return &humanConsole{srv: srv, base: base, secret: secret, open: open, print: printLink}, nil
}

func (h *humanConsole) AttachRun(id string, ui *gate.ChannelUI, cancel context.CancelFunc) {
	h.srv.AttachRun(id, ui, cancel)
}

func (h *humanConsole) DetachRun(id string) { h.srv.DetachRun(id) }

func (h *humanConsole) PublicURL(id string) string { return h.base + "/?run=" + id }

func (h *humanConsole) GateWaiting(id string) {
	link := h.base + "/?run=" + id + "&k=" + h.secret
	if h.print {
		fmt.Fprintln(os.Stderr, "wedra mcp: гейт ждёт человека:", link)
	}
	if !h.open {
		if !h.print {
			fmt.Fprintln(os.Stderr, "wedra mcp: гейт ждёт человека (--no-open без --print-link: ссылку никто не увидит)")
		}
		return
	}
	openBrowser(link)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Stdout, cmd.Stderr = nil, nil
	_ = cmd.Start()
}

// joinFlagValues: ["--x", "v"] → ["--x=v"] для перечисленных флагов.
func joinFlagValues(args []string, flags ...string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		matched := false
		for _, f := range flags {
			if a == f && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				out = append(out, f+"="+args[i+1])
				i++
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, a)
		}
	}
	return out
}
