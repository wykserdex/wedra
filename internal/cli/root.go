package cli

import (
	"fmt"
	"os"
	"strings"

	"orchestrator/internal/api"
)

func Run() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(2)
	}
	cmd := os.Args[1]
	switch cmd {
	case "pipeline":
		handlePipeline(os.Args[2:])
	case "plugin":
		handlePlugin(os.Args[2:])
	case "runs", "run":
		if cmd == "runs" {
			handleRuns(os.Args[2:])
		} else {
			handlePipeline(append([]string{"run"}, os.Args[2:]...))
		}
	case "gui", "serve":
		RunGUI(os.Args[2:])
	case "version", "--version", "-v":
		ver := api.Version
		if raw, err := os.ReadFile("VERSION"); err == nil {
			ver = strings.TrimSpace(string(raw))
		}
		fmt.Printf("orchestrator v%s (CLI focus, M6 meat), protocol v0.2\n", ver)
	case "registry":
		RunRegistryValidate(os.Args[2:])
	case "validate":
		handlePipeline(append([]string{"validate"}, os.Args[2:]...))
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
	fmt.Printf(`orchestrator — CLI-оркестратор цепочек с человеком в петле (v%s, CLI focus)

Команды (мясо, не косметика):
  orchestrator pipeline run <file.yaml> [--yes] [--resume=<run_id>] [--runs-dir=var/runs] [--store=fs|json]
  orchestrator pipeline install <name|file.yaml|url> [--registry=<url|path>]  # пресет + автоустановка плагинов
  orchestrator pipeline validate <file.yaml>
  orchestrator pipeline plan <file.yaml>
  orchestrator pipeline lint <file.yaml>          # validate + file_ref error
  orchestrator plugin install <name>[@version] [--registry=<url|path>]        # из реестра в plugins/
  orchestrator plugin validate <dir>
  orchestrator plugin test <dir>
  orchestrator plugin create <dir> [--author --description --example]
  orchestrator plugin inspect <dir>
  orchestrator plugin search <query>              # поиск по official/community
  orchestrator plugin list                        # список всех плагинов
  orchestrator registry validate [--registry=<url|path>] [--local-source=<dir>]
                                                 # v0.17: trust-гейт реестра (манифест, id, конформность)
  orchestrator runs list [var/runs]               # список прогонов (fs + json)
  orchestrator runs show <run_id> [var/runs]      # журнал + context + artifacts
  orchestrator runs resume <run_id> <pipeline.yaml> [--yes]
  orchestrator gui [--port 8080] [--open]         # отложено, косметика

Совместимость:
  orchestrator run <file.yaml> == pipeline run
  orchestrator validate <file.yaml> == pipeline validate

Версия: v%s (CLI focus, честная 0.x), протокол v0.2
См. PROTOCOL.md, TUTORIAL_PLUGINS.md, CHANGELOG.md
`, ver, ver)
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
	case "validate", "lint":
		RunPipelineValidate(args[1:])
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
