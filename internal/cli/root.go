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
	case "gui", "serve":
		RunGUI(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("orchestrator v0.11 (M6 GUI scaffold), protocol v0.2")
	case "run", "validate":
		// backward compat: плоские команды как раньше (tool run, tool validate)
		// делегируем в pipeline
		handlePipeline(append([]string{cmd}, os.Args[2:]...))
	default:
		fmt.Printf("неизвестная команда %q\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Print(`orchestrator — CLI-оркестратор цепочек с человеком в петле (v0.11, M6)

Команды:
  orchestrator pipeline run <file.yaml> [--yes]
  orchestrator pipeline validate <file.yaml>
  orchestrator pipeline plan <file.yaml>
  orchestrator plugin validate <dir>
  orchestrator plugin test <dir>
  orchestrator plugin create <dir> [--author --description --example]
  orchestrator plugin inspect <dir>
  orchestrator gui [--port 8080] [--open]   # M6: веб-GUI (drag-and-drop, live YAML, JSON на линиях)

Совместимость:
  orchestrator run <file.yaml>  == pipeline run
  orchestrator validate <file.yaml> == pipeline validate

Версия: v0.11 (честная 0.x, было v10), протокол v0.2
См. PROTOCOL.md и TUTORIAL_PLUGINS.md
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
	case "validate":
		RunPipelineValidate(args[1:])
	case "plan":
		RunPipelinePlan(args[1:])
	default:
		fmt.Printf("неизвестная pipeline команда %q\n", sub)
		os.Exit(2)
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
