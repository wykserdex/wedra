package core

import "orchestrator/internal/pipeline"

func LoadPipelineFile(path string) (*PipelineFile, error) {
	return pipeline.LoadPipelineFile(path)
}
