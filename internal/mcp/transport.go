package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
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

// ParseError — сообщение stdin не разобралось как JSON-RPC.
//
// До этого транспорт возвращал ошибку чтения, и сервер завершался: одна
// мусорная строка (или батч-массив) убивала сессию целиком, причём процесс
// выходил с кодом 0 — клиент видел «штатное завершение», а не отказ. По
// JSON-RPC 2.0 на неразобранное сообщение положено ответить -32700 Parse error
// и продолжить обслуживание.
type ParseError struct {
	Raw []byte
	Err error
	// Code — код ответа: 0 означает -32700 (Parse error). -32600
	// (Invalid Request) ставится там, где строка разобралась как JSON, но
	// запросом не является — например, пустой батч.
	Code int
}

func (e *ParseError) Error() string { return "bad json-rpc: " + e.Err.Error() }

func (e *ParseError) Unwrap() error { return e.Err }

// response — ответ на неразобранное сообщение. По JSON-RPC 2.0 id здесь null:
// сопоставить ошибку с запросом нельзя, сообщение не разобрано.
func (e *ParseError) response() *Response {
	code := e.Code
	if code == 0 {
		code = -32700
	}
	return &Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage("null"),
		Error:   &RPCError{Code: code, Message: e.Err.Error()},
	}
}

// Item — одно сообщение кадра: разобранный запрос, ответ клиента на серверный
// запрос, либо ошибка разбора.
type Item struct {
	Request *Request
	Resp    *clientResponse
	Err     *ParseError
}

// Frame — одна строка stdin.
//
// Batch=true означает, что строка была JSON-массивом. Батчи входят в ревизию
// 2024-11-05 (их убрали только в 2025-06-18), и раньше сервер падал на них
// всем процессом с «cannot unmarshal array into Go value of type mcp.Request».
type Frame struct {
	Batch bool
	Items []Item
}

// Transport — line-delimited JSON-RPC поверх reader/writer.
// Stdout/stdin изоляция (Фаза 4.2): вызывающий держит proto=исходный stdout,
// а os.Stdout уже перенаправлен в stderr до старта.
type Transport struct {
	in  *bufio.Scanner
	out *bufio.Writer
	mu  sync.Mutex
}

func NewTransport(r io.Reader, w io.Writer) *Transport {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	return &Transport{in: sc, out: bufio.NewWriter(w)}
}

// ReadFrame — следующая непустая строка, разобранная в кадр.
//
// Ошибка возвращается только на уровне ввода-вывода (в том числе io.EOF):
// проблемы разбора — это элементы кадра, а не конец сессии.
func (t *Transport) ReadFrame() (*Frame, error) {
	for t.in.Scan() {
		line := append([]byte(nil), t.in.Bytes()...)
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		return parseFrame(line), nil
	}
	if err := t.in.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// parseFrame — строка → кадр. Здесь нет ни ввода-вывода, ни состояния: только
// разбор, поэтому и батч, и битая строка одинаково предсказуемы, и на обе
// сервер отвечает ошибкой, не прекращая сессию.
func parseFrame(line []byte) *Frame {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var elems []json.RawMessage
		if err := json.Unmarshal(trimmed, &elems); err != nil {
			return &Frame{Items: []Item{{Err: &ParseError{Raw: line, Err: err}}}}
		}
		// Пустой батч разобран как JSON, но запросом не является: это
		// Invalid Request (-32600), а не Parse error.
		if len(elems) == 0 {
			return &Frame{Items: []Item{{Err: &ParseError{
				Raw: line, Err: fmt.Errorf("пустой батч"), Code: -32600}}}}
		}
		frame := &Frame{Batch: true, Items: make([]Item, 0, len(elems))}
		for _, raw := range elems {
			frame.Items = append(frame.Items, parseItem(raw))
		}
		return frame
	}
	return &Frame{Items: []Item{parseItem(trimmed)}}
}

// parseItem — одно сообщение кадра: запрос, нотификация или ответ клиента на
// серверный запрос. Разделение по признаку method: у ответа его нет.
func parseItem(raw []byte) Item {
	var envelope struct {
		Method string          `json:"method"`
		ID     json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Item{Err: &ParseError{Raw: raw, Err: err}}
	}
	if envelope.Method == "" {
		if len(envelope.ID) == 0 {
			return Item{Err: &ParseError{Raw: raw, Err: fmt.Errorf(
				"сообщение без method и без id (ни запрос, ни ответ)"), Code: -32600}}
		}
		var resp clientResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return Item{Err: &ParseError{Raw: raw, Err: err}}
		}
		return Item{Resp: &resp}
	}
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return Item{Err: &ParseError{Raw: raw, Err: err}}
	}
	return Item{Request: &req}
}

// Read — совместимость: первый запрос кадра. Ошибки разбора возвращаются как
// *ParseError, но сервер обязан отвечать на них, а не завершаться, — поэтому
// основной путь чтения теперь ReadFrame.
func (t *Transport) Read() (*Request, error) {
	frame, err := t.ReadFrame()
	if err != nil {
		return nil, err
	}
	if len(frame.Items) == 0 {
		return nil, io.EOF
	}
	item := frame.Items[0]
	if item.Err != nil {
		return nil, item.Err
	}
	return item.Request, nil
}

func (t *Transport) Write(resp *Response) error {
	return t.writeJSON(resp)
}

// WriteBatch — ответ на батч: массив ответов в порядке запросов.
func (t *Transport) WriteBatch(responses []*Response) error {
	return t.writeJSON(responses)
}

// WriteNotification — нотификация клиенту (id нет по определению).
func (t *Transport) WriteNotification(method string, params interface{}) error {
	msg := map[string]interface{}{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	return t.writeJSON(msg)
}

// WriteRequest — запрос ОТ сервера клиенту (elicitation/create).
//
// Направление, которого раньше не было: до 2025-06-18 сервер только отвечал.
// Ответ придёт в stdin и будет разобран как clientResponse (см. parseFrame).
func (t *Transport) WriteRequest(req *Request) error {
	return t.writeJSON(req)
}

// clientResponse — ответ клиента на серверный запрос. Отличается от Request
// отсутствием method: спека делит сообщения по этому признаку, и путать их
// нельзя — запрос без method получил бы от сервера -32601.
type clientResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

func (t *Transport) writeJSON(v interface{}) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch resp := v.(type) {
	case *Response:
		if resp.JSONRPC == "" {
			resp.JSONRPC = "2.0"
		}
	case []*Response:
		// Элементы батча — тоже ответы, и поле jsonrpc обязательно у каждого.
		// Забыть его здесь легко: подстановка в Write до них не достаёт.
		for _, r := range resp {
			if r != nil && r.JSONRPC == "" {
				r.JSONRPC = "2.0"
			}
		}
	}
	b, err := json.Marshal(v)
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

// notificationPrefix — нотификации по JSON-RPC не имеют id и не получают
// ответа. Разбор по префиксу, а не по списку имён: сервер обязан молча
// принимать любую нотификацию, включая те, которых он не знает.
const notificationPrefix = "notifications/"

func isNotification(req *Request) bool {
	return strings.HasPrefix(req.Method, notificationPrefix)
}
