package cli

import (
	"fmt"
	"os"
	"strings"

	"wedra/internal/api"
)

func Run() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(2)
	}
	cmd := os.Args[1]
	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		printHelp()
		return
	}
	switch cmd {
	case "pipeline":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		handlePipeline(os.Args[2:])
	case "plugin":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		handlePlugin(os.Args[2:])
	case "runs", "run":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		if cmd == "runs" {
			handleRuns(os.Args[2:])
		} else {
			handlePipeline(append([]string{"run"}, os.Args[2:]...))
		}
	case "gui", "serve":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		RunGUI(os.Args[2:])
	case "version", "--version", "-v":
		ver := api.Version
		if raw, err := os.ReadFile("VERSION"); err == nil {
			ver = strings.TrimSpace(string(raw))
		}
		fmt.Printf("wedra v%s, protocol v0.2\n", ver)
	case "registry":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		RunRegistryValidate(os.Args[2:])
	case "validate":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		handlePipeline(append([]string{"validate"}, os.Args[2:]...))
	case "mcp":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		RunMCP(os.Args[2:])
	case "approve":
		if helpRequested(os.Args[2:]) {
			printHelp()
			return
		}
		RunApprove(os.Args[2:])
	default:
		fmt.Printf("неизвестная команда %q\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	ver := api.Version
	if raw, err := os.ReadFile("VERSION"); err == nil {
		ver = strings.TrimSpace(string(raw))
	}
	fmt.Printf(`WEDRA — контрактный исполнитель цепочек с человеком в петле (v%s)

Команды (мясо, не косметика):
  wedra pipeline run <file.yaml> [--yes] [--resume=<run_id>] [--runs-dir=var/runs] [--store=fs|json] [--no-auto-approve] [--deny-untrusted-plugins]
  wedra pipeline install <name|file.yaml|url> [--registry=<url|path>]  # пресет + автоустановка плагинов
  wedra pipeline validate <file.yaml> [--json]      # --json: {ok, issues[]} с кодами (ERRORS.md)
  wedra pipeline plan <file.yaml>
  wedra pipeline lint <file.yaml>          # validate + file_ref error
  wedra plugin install <name>[@version] [--registry=<url|path>]        # из реестра в plugins/
  wedra plugin validate <dir>
  wedra plugin test <dir> [--conformance] [--json]
  wedra plugin test --conformance [--json] [--fixtures=dir]
  wedra plugin create <dir> [--author --description --example]
  wedra plugin inspect <dir>
  wedra plugin search <query>              # поиск по official/community
  wedra plugin list                        # список всех плагинов
  wedra registry validate [--registry=<url|path>] [--local-source=<dir>]
                                                 # v0.17: trust-гейт реестра (манифест, id, конформность)
  wedra runs list [var/runs]               # список прогонов (fs + json)
  wedra runs show <run_id> [var/runs]      # журнал + context + artifacts
  wedra runs resume <run_id> <pipeline.yaml> [--yes] [--runs-dir=var/runs] [--store=fs|json] [--db-path=file] [--no-auto-approve] [--deny-untrusted-plugins]
  wedra gui [--port 8765] [--open] [--no-session] [--plugins=<dir>] [--pipelines=<dir>] [--runs-dir=<dir>]  # консоль; мутации — по ссылке ?k= из терминала
  wedra mcp --plugins=<dir> [--workdir=<dir>] [--no-gui]  # MCP-сервер (stdio) для LLM-агентов
  wedra approve <run_id> <step_id>                # только интерактивный TTY

Совместимость:
  wedra run <file.yaml> == pipeline run
  wedra validate <file.yaml> == pipeline validate

Версия: v%s (честная 0.x), протокол v0.2
См. protocol/v0.2/PROTOCOL.md, protocol/v0.2/ERRORS.md, docs/mcp.md, CHANGELOG.md
`, ver, ver)
}

func helpRequested(args []string) bool {
	for _, a := range args {
		if a == "help" || a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

func handlePipeline(args []string) {
	if len(args) == 0 {
		printHelp()
		os.Exit(2)
	}
	sub := args[0]
	switch sub {
	case "run":
		RunPipelineRun(args[1:])
	case "install":
		RunPipelineInstall(args[1:])
	case "validate":
		RunPipelineValidate(args[1:])
	case "lint":
		RunPipelineValidate(append(args[1:], "--lint"))
	case "plan":
		RunPipelinePlan(args[1:])
	default:
		fmt.Printf("неизвестная pipeline команда %q\n", sub)
		os.Exit(2)
	}
}

func handleRuns(args []string) {
	if len(args) == 0 {
		RunRunsList(nil)
		return
	}
	sub := args[0]
	switch sub {
	case "list":
		RunRunsList(args[1:])
	case "show":
		RunRunsShow(args[1:])
	case "resume":
		RunRunsResume(args[1:])
	default:
		if strings.HasPrefix(sub, "-") {
			fmt.Printf("неизвестная runs команда %q\n", sub)
			os.Exit(2)
		}
		if len(args) == 1 {
			RunRunsShow(args)
		} else {
			fmt.Printf("неизвестная runs команда %q\n", sub)
			os.Exit(2)
		}
	}
}

func handlePlugin(args []string) {
	if len(args) == 0 {
		printHelp()
		os.Exit(2)
	}
	sub := args[0]
	switch sub {
	case "validate":
		RunPluginValidate(args[1:])
	case "test":
		RunPluginTest(args[1:])
	case "create":
		RunPluginCreate(args[1:])
	case "inspect":
		RunPluginInspect(args[1:])
	case "install":
		RunPluginInstall(args[1:])
	case "search":
		RunPluginSearch(args[1:])
	case "list":
		RunPluginList(args[1:])
	default:
		fmt.Printf("неизвестная plugin команда %q\n", sub)
		os.Exit(2)
	}
}
