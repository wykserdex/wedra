package cli

import (
	"fmt"
	"os"
	"strings"

	"wedra/internal/mcp"
)

// RunMCP — wedra mcp --plugins <dir>... --workdir <dir> [--runs-dir <dir>].
// Stdio JSON-RPC. Изоляция (Фаза 4.2): proto=исходный stdout, os.Stdout→stderr,
// Quiet:true, StdinUI запрещён (MCPMode).
func RunMCP(args []string) {
	var plugins []string
	workdir := ""
	runsdir := ""
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
		case a == "--help" || a == "-h":
			fmt.Fprintln(os.Stderr, "wedra mcp --plugins <dir> [--plugins <dir>...] --workdir <dir> [--runs-dir <dir>]")
			return
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "неизвестный флаг %q\n", a)
			os.Exit(2)
		default:
			// позиционные --plugins без = : wedra mcp <plugindir> <workdir>? не поддерживаем
			fmt.Fprintf(os.Stderr, "неизвестный аргумент %q (см. --help)\n", a)
			os.Exit(2)
		}
	}
	if workdir == "" {
		cwd, _ := os.Getwd()
		workdir = cwd
	}
	// Изоляция stdout: JSON-RPC — только в исходный stdout, всё остальное — в stderr.
	proto := os.Stdout
	os.Stdout = os.Stderr
	t := mcp.NewTransport(os.Stdin, proto)
	srv, err := mcp.NewServer(mcp.Options{PluginsDirs: plugins, WorkDir: workdir, RunsDir: runsdir})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp:", err)
		os.Exit(1)
	}
	if err := srv.Serve(t); err != nil {
		// EOF клиента — нормальное завершение
		if err.Error() != "EOF" && !strings.Contains(err.Error(), "EOF") {
			fmt.Fprintln(os.Stderr, "mcp:", err)
		}
	}
}
