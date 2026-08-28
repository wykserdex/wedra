package core

import "orchestrator/internal/pipeline"

func Validate(pf *PipelineFile, eng *Engine) (errs, warns []string) {
	return pipeline.Validate(pf, eng)
}

func ValidatePluginDir(dir string) []string {
	return pipeline.ValidatePluginDir(dir)
}

func checkPortFormats(pfx, name string, port Port, errs []string) []string {
	return pipeline.CheckPortFormats(pfx, name, port, errs)
}
