package pipeline

import (
	"strings"
)

type Plan struct {
	PipelineFile *PipelineFile
	Steps        []Step
	Warnings     []string
	Errors       []string
	DAG          DAG
}

type DAG struct {
	Nodes []DAGNode `json:"nodes"`
	Edges []DAGEdge `json:"edges"`
}

type DAGNode struct {
	ID            string            `json:"id"`
	Plugin        string            `json:"plugin"`
	Phase         string            `json:"phase"`
	Bind          map[string]string `json:"bind"`
	AfterForeach  bool              `json:"after_foreach"`
	When          string            `json:"when,omitempty"`
	Foreach       string            `json:"foreach,omitempty"`
	ParallelGroup string            `json:"parallel_group,omitempty"`
}

type DAGEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Via  string `json:"via"`
}

func PlanPipeline(pf *PipelineFile, eng Engine) (*Plan, error) {
	errs, warns := Validate(pf, eng)

	nodes := []DAGNode{}
	edges := []DAGEdge{}
	preSteps := map[string]bool{}
	if pf.Pipeline.Foreach != "" && strings.HasPrefix(pf.Pipeline.Foreach, "steps.") {
		parts := strings.Split(pf.Pipeline.Foreach, ".")
		if len(parts) >= 2 {
			srcID := parts[1]
			for _, st := range pf.Pipeline.Steps {
				preSteps[st.ID] = true
				if st.ID == srcID {
					break
				}
			}
		}
	}
	for _, st := range pf.Pipeline.Steps {
		phase := "foreach"
		if preSteps[st.ID] {
			phase = "pre"
		}
		if st.AfterForeach {
			phase = "post"
		}
		if st.ParallelGroup != "" {
			phase = "parallel"
		}
		whenStr := ""
		if st.When.IsSet() {
			whenStr = st.When.String()
		}
		nodes = append(nodes, DAGNode{
			ID: st.ID, Plugin: st.Plugin, Phase: phase, Bind: st.Bind, AfterForeach: st.AfterForeach,
			When: whenStr, Foreach: st.Foreach, ParallelGroup: st.ParallelGroup,
		})
		for _, from := range st.Bind {
			if strings.HasPrefix(from, "steps.") {
				parts := strings.Split(from, ".")
				if len(parts) >= 2 {
					dep := parts[1]
					if strings.HasSuffix(dep, "_all") {
						dep = strings.TrimSuffix(dep, "_all")
					}
					edges = append(edges, DAGEdge{From: dep, To: st.ID, Via: from})
				}
			}
		}
		// v0.20: зависимости from when/foreach
		for _, depPath := range []string{st.When.Path, st.Foreach} {
			if strings.HasPrefix(depPath, "steps.") {
				parts := strings.Split(depPath, ".")
				if len(parts) >= 2 {
					dep := parts[1]
					edges = append(edges, DAGEdge{From: dep, To: st.ID, Via: depPath})
				}
			}
		}
		for _, f := range st.Form {
			if strings.HasPrefix(f.Field, "steps.") {
				parts := strings.Split(f.Field, ".")
				if len(parts) >= 2 {
					dep := parts[1]
					if strings.HasSuffix(dep, "_all") {
						dep = strings.TrimSuffix(dep, "_all")
					}
					edges = append(edges, DAGEdge{From: dep, To: st.ID, Via: f.Field})
				}
			}
		}
	}

	return &Plan{
		PipelineFile: pf,
		Steps:        pf.Pipeline.Steps,
		Warnings:     warns,
		Errors:       errs,
		DAG:          DAG{Nodes: nodes, Edges: edges},
	}, nil
}
