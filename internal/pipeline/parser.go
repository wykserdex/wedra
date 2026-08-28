package pipeline

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadPipelineFile(path string) (*PipelineFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pf PipelineFile
	if err := yaml.Unmarshal(raw, &pf); err != nil {
		return nil, fmt.Errorf("YAML: %w", err)
	}
	return &pf, nil
}
