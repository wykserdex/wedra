package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"orchestrator/internal/journal"
)

func RunRunsList(args []string) {
	runsDir := "var/runs"
	if len(args) > 0 {
		runsDir = args[0]
	}
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		fmt.Println("нет прогонов:", err)
		return
	}
	fmt.Printf("Прогоны в %s:\n", runsDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(runsDir, e.Name())
		rd := journal.NewReader(dir)
		events, _ := rd.Events()
		pipelineName := ""
		if len(events) > 0 {
			if pn, ok := events[0]["pipeline"].(string); ok {
				pipelineName = pn
			}
		}
		fmt.Printf("  %-40s  pipeline=%-20s  events=%d\n", e.Name(), pipelineName, len(events))
	}
}

func RunRunsShow(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен id прогона: orchestrator runs show <run_id>")
		os.Exit(2)
	}
	id := args[0]
	runsDir := "var/runs"
	if len(args) > 1 {
		runsDir = args[1]
	}
	dir := filepath.Join(runsDir, id)
	rd := journal.NewReader(dir)
	events, err := rd.Events()
	if err != nil {
		fmt.Println("ошибка чтения журнала:", err)
		os.Exit(1)
	}
	snap, _ := rd.ContextSnapshot()
	fmt.Printf("Run %s (%s):\n", id, dir)
	fmt.Printf("  events: %d\n", len(events))
	for _, ev := range events {
		b, _ := json.Marshal(ev)
		fmt.Println("  ", string(b))
	}
	if snap != nil {
		fmt.Println("\nContext snapshot:")
		b, _ := json.MarshalIndent(snap, "", "  ")
		fmt.Println(string(b))
	}
}

func RunRunsResume(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен id прогона: orchestrator runs resume <run_id> -- <pipeline.yaml>")
		os.Exit(2)
	}
	// формат: orchestrator runs resume <run_id> [--yes] -- <pipeline.yaml>
	// для MVP: просто вызываем pipeline run с --resume
	runID := args[0]
	// остальное — путь к пайплайну
	pipelineFile := ""
	yes := false
	for _, a := range args[1:] {
		if a == "--yes" {
			yes = true
		} else if a != "--" && pipelineFile == "" && (len(a) > 5 && (a[len(a)-5:] == ".yaml" || a[len(a)-4:] == ".yml")) {
			pipelineFile = a
		}
	}
	if pipelineFile == "" {
		fmt.Println("нужен файл пайплайна для resume: orchestrator runs resume <run_id> <pipeline.yaml> [--yes]")
		os.Exit(2)
	}
	// делегируем в RunPipelineRun с Resume
	// формируем args для RunPipelineRun: <file> --yes --resume=<id>
	newArgs := []string{pipelineFile}
	if yes {
		newArgs = append(newArgs, "--yes")
	}
	newArgs = append(newArgs, "--resume="+runID)
	RunPipelineRun(newArgs)
}
