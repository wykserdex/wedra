package plugin

import (
	"time"

	"orchestrator/internal/pipeline"
)

type Transport interface {
	Invoke(manifest *pipeline.Manifest, input []byte) ([]byte, error)
}

type StdioTransport struct {
	Timeout time.Duration
}

func (t *StdioTransport) Invoke(manifest *pipeline.Manifest, input []byte) ([]byte, error) {
	res := Exec(manifest, input, t.Timeout)
	if res.Output != nil {
		return nil, nil
	}
	if res.ErrMsg != "" {
		return nil, nil
	}
	return nil, nil
}

type Envelope struct {
	ProtocolVersion string      `json:"protocol_version"`
	Type            string      `json:"type"`
	RequestID       string      `json:"request_id"`
	Payload         interface{} `json:"payload"`
}
