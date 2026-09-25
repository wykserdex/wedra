package pipeline

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadPipelineFile(path string) (*PipelineFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadPipelineFileFromBytes(raw)
}

func LoadPipelineFileFromBytes(raw []byte) (*PipelineFile, error) {
	var pf PipelineFile
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&pf); err != nil {
		return nil, fmt.Errorf("YAML: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("YAML: несколько документов")
		}
		return nil, fmt.Errorf("YAML: %w", err)
	}
	return &pf, nil
}
