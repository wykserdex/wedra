package cli

import (
	"fmt"
	"os"

	"wedra/internal/core"
)

func RunPipelineRun(args []string) {
	if len(args) == 0 {
		fmt.Println("нужен файл пайплайна: orchestrator pipeline run <file.yaml>")
		os.Exit(2)
	}
	file := args[0]
	yes := false
	runsDir := ""
	resume := ""
	store := "fs"
	dbPath := ""
	for _, a := range args[1:] {
		if a == "--yes" {
			yes = true
		}
		if len(a) > 11 && a[:11] == "--runs-dir=" {
			runsDir = a[11:]
		}
		if len(a) > 9 && a[:9] == "--resume=" {
			resume = a[9:]
		}
		if len(a) > 8 && a[:8] == "--store=" {
			store = a[8:]
		}
		if len(a) > 9 && a[:9] == "--db-path=" {
			dbPath = a[9:]
		}
	}
	if runsDir == "" {
		runsDir = "var/runs"
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
	stats, err := core.Run(pf, eng, core.RunOptions{Yes: yes, RunsDir: runsDir, Resume: resume, Store: store, DBPath: dbPath})
	if err != nil {
		fmt.Println("ран упал:", err)
		os.Exit(1)
	}
	if stats.Aborted > 0 {
		os.Exit(1)
	}
}
