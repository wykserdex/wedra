package cli

import (
	"fmt"
	"os"

	"orchestrator/internal/core"
)

func RunPipelineRun(args []string) {
	// args: <file.yaml> [--yes] [--runs-dir var/runs]
	// делегируем в core.Run через старый main.go логику
	// для MVP просто вызываем core напрямую, как раньше
	if len(args) == 0 {
		fmt.Println("нужен файл пайплайна: orchestrator pipeline run <file.yaml>")
		os.Exit(2)
	}
	file := args[0]
	yes := false
	runsDir := ""
	for _, a := range args[1:] {
		if a == "--yes" {
			yes = true
		}
		if len(a) > 11 && a[:11] == "--runs-dir=" {
			runsDir = a[11:]
		}
	}
	if runsDir == "" {
		runsDir = "var/runs"
		// fallback на runs/ для совместимости если var/runs нет
		if _, err := os.Stat(runsDir); os.IsNotExist(err) {
			runsDir = "runs"
		}
	}

	eng := core.NewEngine()
	pf, err := core.LoadPipelineFile(file)
	if err != nil {
		fmt.Println("ошибка загрузки пайплайна:", err)
		os.Exit(2)
	}
	errs, warns := core.Validate(pf, eng)
	for _, w := range warns {
		fmt.Println("  · предупреждение:", w)
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println("  ✗", e)
		}
		os.Exit(1)
	}
	stats, err := core.Run(pf, eng, core.RunOptions{Yes: yes, RunsDir: runsDir})
	if err != nil {
		fmt.Println("ран упал:", err)
		os.Exit(1)
	}
	if stats.Aborted > 0 {
		os.Exit(1)
	}
}
