package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
	"orchestrator/internal/core"
	"orchestrator/internal/pipeline"
)

func RunPipelineValidate(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен файл пайплайна: orchestrator pipeline validate <file.yaml>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Println("ошибка чтения:", err)
		os.Exit(2)
	}
	var pf core.PipelineFile
	if err := yaml.Unmarshal(raw, &pf); err != nil {
		fmt.Println("YAML ошибка:", err)
		os.Exit(2)
	}
	errs, warns := core.Validate(&pf, core.NewEngine())
	for _, w := range warns {
		fmt.Println("  · предупреждение:", w)
	}
	for _, e := range errs {
		fmt.Println("  ✗", e)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
	fmt.Println("OK: цепочка совместима")
}

func RunPipelinePlan(args []string) {
	if len(args) < 1 {
		fmt.Println("нужен файл пайплайна: orchestrator pipeline plan <file.yaml>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Println("ошибка чтения:", err)
		os.Exit(2)
	}
	pf, err := pipeline.LoadPipelineFileFromBytes(raw)
	if err != nil {
		fmt.Println("YAML ошибка:", err)
		os.Exit(2)
	}
	plan, err := pipeline.PlanPipeline(pf, core.NewEngine())
	if err != nil {
		fmt.Println("ошибка плана:", err)
		os.Exit(2)
	}
	fmt.Printf("Pipeline: %s\n", pf.Pipeline.Name)
	fmt.Printf("Input: %v\n", pf.Pipeline.Input)
	if pf.Pipeline.Foreach != "" {
		fmt.Printf("Foreach: %s (item=%s, type=%s, format=%s)\n", pf.Pipeline.Foreach, pf.Pipeline.ForeachItem, pf.Pipeline.ItemType, pf.Pipeline.ItemFormat)
	}
	fmt.Println("Steps (DAG):")
	for i, st := range pf.Pipeline.Steps {
		phase := "foreach"
		for _, n := range plan.DAG.Nodes {
			if n.ID == st.ID {
				phase = n.Phase
				break
			}
		}
		fmt.Printf("  %d. %s → %s (on_error=%s, phase=%s, bind=%v)\n", i+1, st.ID, st.Plugin, st.OnError, phase, st.Bind)
		if len(st.Form) > 0 {
			fmt.Printf("     form: %v\n", st.Form)
		}
	}
	fmt.Println("Edges:")
	for _, e := range plan.DAG.Edges {
		fmt.Printf("  %s → %s via %s\n", e.From, e.To, e.Via)
	}
	for _, w := range plan.Warnings {
		fmt.Println("  · предупреждение:", w)
	}
	if len(plan.Errors) > 0 {
		fmt.Println("Ошибки валидации:")
		for _, e := range plan.Errors {
			fmt.Println("  ✗", e)
		}
		os.Exit(1)
	}
	if len(os.Args) > 3 && os.Args[3] == "--json" {
		b, _ := json.MarshalIndent(plan.DAG, "", "  ")
		fmt.Println(string(b))
	}
	fmt.Println("Plan OK — зависимостей и циклов нет (проверка циклов: v0.13)")
}
