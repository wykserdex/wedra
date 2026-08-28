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
	storeType := "fs"
	dbPath := ""
	for _, a := range args {
		if len(a) > 11 && a[:11] == "--runs-dir=" {
			runsDir = a[11:]
		} else if len(a) > 8 && a[:8] == "--store=" {
			storeType = a[8:]
		} else if len(a) > 9 && a[:9] == "--db-path=" {
			dbPath = a[9:]
		} else if !isFlag(a) {
			runsDir = a
		}
	}
	var store journal.RunStore
	if storeType == "sqlite" {
		store = journal.NewSQLiteStore(runsDir, dbPath)
	} else {
		store = journal.NewFilesystemStore(runsDir)
	}
	ids, err := store.ListRuns()
	if err != nil {
		fmt.Println("нет прогонов:", err)
		return
	}
	fmt.Printf("Прогоны в %s (store=%s):\n", runsDir, storeType)
	for _, id := range ids {
		dir := filepath.Join(runsDir, id)
		rd := journal.NewReader(dir)
		events, _ := rd.Events()
		pipelineName := ""
		if len(events) > 0 {
			if pn, ok := events[0]["pipeline"].(string); ok {
				pipelineName = pn
			}
		}
		arts, _ := store.ListArtifacts(id)
		fmt.Printf("  %-40s  pipeline=%-20s  events=%d  artifacts=%d\n", id, pipelineName, len(events), len(arts))
	}
}

func RunRunsShow(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен id прогона: orchestrator runs show <run_id>")
		os.Exit(2)
	}
	id := args[0]
	runsDir := "var/runs"
	storeType := "fs"
	dbPath := ""
	for _, a := range args[1:] {
		if len(a) > 11 && a[:11] == "--runs-dir=" {
			runsDir = a[11:]
		} else if len(a) > 8 && a[:8] == "--store=" {
			storeType = a[8:]
		} else if len(a) > 9 && a[:9] == "--db-path=" {
			dbPath = a[9:]
		} else if !isFlag(a) {
			runsDir = a
		}
	}
	var store journal.RunStore
	if storeType == "sqlite" {
		store = journal.NewSQLiteStore(runsDir, dbPath)
	} else {
		store = journal.NewFilesystemStore(runsDir)
	}
	dir := filepath.Join(runsDir, id)
	rd := journal.NewReader(dir)
	events, err := rd.Events()
	if err != nil {
		fmt.Println("ошибка чтения журнала:", err)
		os.Exit(1)
	}
	snap, _ := rd.ContextSnapshot()
	fmt.Printf("Run %s (%s) store=%s:\n", id, dir, storeType)
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
	arts, _ := store.ListArtifacts(id)
	if len(arts) > 0 {
		fmt.Printf("\nArtifacts (%d):\n", len(arts))
		for _, a := range arts {
			fmt.Println("  -", a)
		}
	}
}

func RunRunsResume(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен id прогона: orchestrator runs resume <run_id> -- <pipeline.yaml>")
		os.Exit(2)
	}
	runID := args[0]
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
	newArgs := []string{pipelineFile}
	if yes {
		newArgs = append(newArgs, "--yes")
	}
	newArgs = append(newArgs, "--resume="+runID)
	RunPipelineRun(newArgs)
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}
