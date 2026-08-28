package core

import "orchestrator/internal/pipeline"

type Duration = pipeline.Duration
type PipelineFile = pipeline.PipelineFile
type Pipeline = pipeline.Pipeline
type Step = pipeline.Step
type Retry = pipeline.Retry
type FormField = pipeline.FormField
type Port = pipeline.Port
type Runtime = pipeline.Runtime
type Permissions = pipeline.Permissions
type NetworkPermission = pipeline.NetworkPermission
type Manifest = pipeline.Manifest

const PlatformAPI = pipeline.PlatformAPI

func PortSource(portName string, port Port, st *Step) string {
	return pipeline.PortSource(portName, port, st)
}

func portSource(portName string, port Port, st *Step) string {
	return pipeline.PortSource(portName, port, st)
}
