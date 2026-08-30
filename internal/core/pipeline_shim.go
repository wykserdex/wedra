package core

import "wedra/internal/pipeline"

func LoadPipelineFile(path string) (*PipelineFile, error) {
	return pipeline.LoadPipelineFile(path)
}
