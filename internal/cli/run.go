package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"wedra/internal/core"
	"wedra/internal/execution"
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
	noAuto := false
	for _, a := range args[1:] {
		if a == "--yes" {
			yes = true
		}
		// v0.9: --no-auto-approve — политика: --yes не одобряет ни один гейт
		if a == "--no-auto-approve" {
			noAuto = true
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
		// v0.9: было a[:9] == "--db-path=" (10 символов) — никогда не совпадало
		if strings.HasPrefix(a, "--db-path=") {
			dbPath = strings.TrimPrefix(a, "--db-path=")
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
	// v0.9: первый Ctrl+C — graceful cancel (журнал run_cancelled + snapshot,
	// --resume поднимет), второй — жёсткий выход.
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		fmt.Println("\n  ! Ctrl+C — отменяю ран (ещё раз — выйти сразу)")
		cancel()
		<-sig
		os.Exit(130)
	}()
	stats, err := core.Run(pf, eng, core.RunOptions{Yes: yes, RunsDir: runsDir, Resume: resume, Store: store, DBPath: dbPath, Ctx: runCtx, NoAutoApprove: noAuto})
	if err != nil {
		if errors.Is(err, execution.ErrCancelled) {
			fmt.Println("ран отменён; продолжить: wedra runs resume", stats.RunDir)
			os.Exit(130)
		}
		fmt.Println("ран упал:", err)
		os.Exit(1)
	}
	if stats.Aborted > 0 {
		os.Exit(1)
	}
}
