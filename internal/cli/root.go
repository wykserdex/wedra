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
	fmt.Print(`orchestrator — CLI-оркестратор цепочек с человеком в петле

Команды (новый стиль, масштабируется):
  orchestrator pipeline run <file.yaml> [--yes]
  orchestrator pipeline validate <file.yaml>
  orchestrator pipeline plan <file.yaml> [--dry-run]
  orchestrator plugin validate <dir>
  orchestrator plugin test <dir>
  orchestrator plugin create <dir> [--author --description --example]

Совместимость:
  orchestrator run <file.yaml>  == pipeline run
  orchestrator validate <file.yaml> == pipeline validate

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
		// для совместимости: если вызвали `tool run file.yaml`, sub уже file.yaml?
		// но handlePipeline вызван с ["run", "file.yaml"] — ок
		// если вызвали напрямую `tool validate file.yaml` через root, то cmd=validate, args=file.yaml
		// тогда sub = file.yaml, не команда — считаем что это validate
		if sub == "run" || sub == "validate" {
			// unreachable
		}
		// если первый arg — файл, считаем validate или run в зависимости от контекста
		// для простоты: если мы сюда попали из root с cmd=run/validate, то args[0]=file
		// но мы уже обработали в root — сюда не попадём
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
