package core

import "github.com/wykserdex/wedra/internal/pipeline"

func LoadPipelineFile(path string) (*PipelineFile, error) {
	return pipeline.LoadPipelineFile(path)
}
