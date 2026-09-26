package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"wedra/internal/core"
	"wedra/internal/execution"
)

func runIDFromDir(runDir string) string {
	if runDir == "" {
		return ""
	}
	return filepath.Base(filepath.Clean(runDir))
}

func RunPipelineRun(args []string) {
	file := ""
	yes := false
	runsDir := ""
	resume := ""
	store := "fs"
	dbPath := ""
	noAuto := false
	denyUntrusted := false
	allowUntrusted := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--yes":
			yes = true
		case a == "--no-auto-approve":
			noAuto = true
		case a == "--deny-untrusted-plugins":
			denyUntrusted = true
		case a == "--allow-untrusted-plugins":
			allowUntrusted = true
		case strings.HasPrefix(a, "--runs-dir="):
			runsDir = strings.TrimPrefix(a, "--runs-dir=")
		case a == "--runs-dir":
			if i+1 >= len(args) {
				fmt.Println("флагу --runs-dir нужно значение")
				os.Exit(2)
			}
			i++
			runsDir = args[i]
		case strings.HasPrefix(a, "--resume="):
			resume = strings.TrimPrefix(a, "--resume=")
		case a == "--resume":
			if i+1 >= len(args) {
				fmt.Println("флагу --resume нужно значение")
				os.Exit(2)
			}
			i++
			resume = args[i]
		case strings.HasPrefix(a, "--store="):
			store = strings.TrimPrefix(a, "--store=")
		case a == "--store":
			if i+1 >= len(args) {
				fmt.Println("флагу --store нужно значение")
				os.Exit(2)
			}
			i++
			store = args[i]
		case strings.HasPrefix(a, "--db-path="):
			dbPath = strings.TrimPrefix(a, "--db-path=")
		case a == "--db-path":
			if i+1 >= len(args) {
				fmt.Println("флагу --db-path нужно значение")
				os.Exit(2)
			}
			i++
			dbPath = args[i]
		case strings.HasPrefix(a, "-"):
			fmt.Printf("неизвестный флаг pipeline run %q\n", a)
			os.Exit(2)
		case file == "":
			file = a
		default:
			fmt.Printf("лишний аргумент pipeline run %q\n", a)
			os.Exit(2)
		}
	}
	if file == "" {
		fmt.Println("нужен файл пайплайна: orchestrator pipeline run <file.yaml>")
		os.Exit(2)
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
	stats, err := core.Run(pf, eng, core.RunOptions{Yes: yes, RunsDir: runsDir, Resume: resume, Store: store, DBPath: dbPath, Ctx: runCtx, NoAutoApprove: noAuto, DenyUntrusted: denyUntrusted, AllowUntrusted: allowUntrusted})
	if err != nil {
		if errors.Is(err, execution.ErrCancelled) {
			fmt.Println("ран отменён; продолжить: wedra runs resume", runIDFromDir(stats.RunDir))
			os.Exit(130)
		}
		fmt.Println("ран упал:", err)
		os.Exit(1)
	}
	if code := runExitCode(pf.Pipeline.Foreach, stats); code != 0 {
		os.Exit(code)
	}
}

// runExitCode — PROTOCOL §6: `0` — ран дошёл до конца (per-item итоги в
// журнале), `1` — рановая неудача: платформенная ошибка (сюда не доходит,
// её обрабатывает вызывающий по err) либо, в одиночном режиме без foreach,
// stop/reject. В батче (foreach) частичные aborts — это per-item результат
// (`item_aborted` в журнале), а не рановая неудача: список из 10 000 строк,
// где три битые, дошёл до конца, и CI не должен видеть по нему отказ.
func runExitCode(foreach string, stats execution.RunStats) int {
	if stats.Aborted > 0 && foreach == "" {
		return 1
	}
	return 0
}
