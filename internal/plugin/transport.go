package plugin

import "orchestrator/internal/pipeline"

// Transport — абстракция транспорта (сейчас только stdio JSON, план — JSONL + envelope + gRPC)

type Transport interface {
	Invoke(manifest *pipeline.Manifest, input []byte) ([]byte, error)
}

// StdioTransport — текущая реализация: stdin JSON → stdout JSON конверт
type StdioTransport struct{}

func (t *StdioTransport) Invoke(manifest *pipeline.Manifest, input []byte) ([]byte, error) {
	// заглушка — реальный вызов в process.go Exec
	return nil, nil
}

// Envelope (план v0.3) — обёртка с protocol_version, type, request_id
type Envelope struct {
	ProtocolVersion string      `json:"protocol_version"`
	Type            string      `json:"type"` // invoke, cancel, handshake
	RequestID       string      `json:"request_id"`
	Payload         interface{} `json:"payload"`
}
