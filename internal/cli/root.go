package cli

import (
	"fmt"
	"os"
)

// Root — точка сборки CLI, теперь в internal/cli вместо cmd/tool/main.go
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
		// runs — новая команда v0.12, run — алиас для pipeline run (совместимость)
		if cmd == "runs" {
			handleRuns(os.Args[2:])
		} else {
			handlePipeline(append([]string{"run"}, os.Args[2:]...))
		}
	case "gui", "serve":
		RunGUI(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("orchestrator v0.12 (CLI focus, M6 meat), protocol v0.2")
	case "validate":
		handlePipeline(append([]string{"validate"}, os.Args[2:]...))
	default:
		fmt.Printf("неизвестная команда %q\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`orchestrator — CLI-оркестратор цепочек с человеком в петле (v0.12, CLI focus)

Команды (мясо, не косметика):
  orchestrator pipeline run <file.yaml> [--yes] [--resume=<run_id>] [--runs-dir=var/runs]
  orchestrator pipeline validate <file.yaml>
  orchestrator pipeline plan <file.yaml>
  orchestrator pipeline lint <file.yaml>          # alias validate + file_ref check
  orchestrator plugin validate <dir>
  orchestrator plugin test <dir>
  orchestrator plugin create <dir> [--author --description --example]
  orchestrator plugin inspect <dir>
  orchestrator runs list [var/runs]               # список прогонов
  orchestrator runs show <run_id> [var/runs]      # журнал + context
  orchestrator runs resume <run_id> <pipeline.yaml> [--yes]  # --resume
  orchestrator gui [--port 8080] [--open]         # отложено, косметика (v0.11 scaffold)

Совместимость:
  orchestrator run <file.yaml> == pipeline run
  orchestrator validate <file.yaml> == pipeline validate

Версия: v0.12 (CLI focus, честная 0.x), протокол v0.2
См. PROTOCOL.md, TUTORIAL_PLUGINS.md, CHANGELOG.md
`)
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
		// если первый arg — не команда, а run_id для show
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
	default:
		fmt.Printf("неизвестная plugin команда %q\n", sub)
		os.Exit(2)
	}
}
