package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Request — JSON-RPC 2.0 запрос (stdio, по одному сообщению на строку).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response — ответ. Error — только для протокольных ошибок;
// ok:false в validate — это нормальный result, не error.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError — JSON-RPC error.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Transport — line-delimited JSON-RPC поверх reader/writer.
// Stdout/stdin изоляция (Фаза 4.2): вызывающий держит proto=исходный stdout,
// а os.Stdout уже перенаправлен в stderr до старта.
type Transport struct {
	in  *bufio.Scanner
	out *bufio.Writer
}

func NewTransport(r io.Reader, w io.Writer) *Transport {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	return &Transport{in: sc, out: bufio.NewWriter(w)}
}

func (t *Transport) Read() (*Request, error) {
	for t.in.Scan() {
		line := t.in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			return nil, fmt.Errorf("bad json-rpc: %w", err)
		}
		return &req, nil
	}
	if err := t.in.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (t *Transport) Write(resp *Response) error {
	if resp.JSONRPC == "" {
		resp.JSONRPC = "2.0"
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := t.out.Write(b); err != nil {
		return err
	}
	return t.out.Flush()
}

// StdioTransport — транспорт на реальных stdin/stdout процесса.
func StdioTransport() *Transport {
	return NewTransport(os.Stdin, os.Stdout)
}
