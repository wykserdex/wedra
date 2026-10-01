package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer — вывод сервера пишется из горутины Serve, а читается из
// горутины теста. bytes.Buffer для этого не годится: без блокировки чтение
// видит полузаписанную строку (и ловит гонку под -race).
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// codeOf — код ответа без паники на неожиданной форме: тест обязан падать с
// сообщением, а не с interface conversion.
func codeOf(t *testing.T, resp map[string]interface{}) float64 {
	t.Helper()
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("в ответе нет объекта error: %v", resp)
	}
	code, ok := errObj["code"].(float64)
	if !ok {
		t.Fatalf("в ошибке нет числового code: %v", resp)
	}
	return code
}

// Разбор кадра — чистая функция, поэтому проверяется без сервера и без
// таймингов: именно здесь раньше рождалась ошибка чтения, убивавшая сессию.

func TestConformanceParseErrorKeepsSession(t *testing.T) {
	frame := parseFrame([]byte("это не json"))
	if len(frame.Items) != 1 || frame.Items[0].Err == nil {
		t.Fatalf("мусор обязан стать ошибкой разбора: %+v", frame)
	}
	if frame.Batch {
		t.Fatal("одиночная строка не батч")
	}
	resp := frame.Items[0].Err.response()
	if resp.Error == nil || resp.Error.Code != -32700 {
		t.Fatalf("ожидался -32700 Parse error: %+v", resp)
	}
	if string(resp.ID) != "null" {
		t.Fatalf("id ошибки разбора обязан быть null, получено %s", resp.ID)
	}
}

func TestConformanceBatchArrayAccepted(t *testing.T) {
	frame := parseFrame([]byte(`[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"ping"}]`))
	if !frame.Batch {
		t.Fatal("массив обязан распознаваться как батч")
	}
	if len(frame.Items) != 2 || frame.Items[0].Request == nil || frame.Items[1].Request == nil {
		t.Fatalf("батч из двух запросов разобран неверно: %+v", frame.Items)
	}
}

func TestConformanceBatchKeepsGoodElements(t *testing.T) {
	frame := parseFrame([]byte(`[{"jsonrpc":"2.0","id":1,"method":"ping"}, 42 ]`))
	if len(frame.Items) != 2 {
		t.Fatalf("элементы потеряны: %+v", frame.Items)
	}
	if frame.Items[0].Request == nil {
		t.Fatal("первый элемент обязан разобраться")
	}
	if frame.Items[1].Err == nil {
		t.Fatal("второй элемент (число) — ошибка разбора")
	}
}

func TestConformanceEmptyBatchInvalidRequest(t *testing.T) {
	frame := parseFrame([]byte(`[]`))
	if len(frame.Items) != 1 || frame.Items[0].Err == nil {
		t.Fatalf("пустой батч обязан стать ошибкой: %+v", frame)
	}
	if code := frame.Items[0].Err.response().Error.Code; code != -32600 {
		t.Fatalf("пустой батч — это Invalid Request (-32600), получено %d", code)
	}
}

func TestConformanceStringIDPreserved(t *testing.T) {
	frame := parseFrame([]byte(`{"jsonrpc":"2.0","id":"abc","method":"ping"}`))
	if got := string(frame.Items[0].Request.ID); got != `"abc"` {
		t.Fatalf("id испорчен: %s", got)
	}
}

// Дальше — поведение сервера на уровне stdio: то, что наблюдал бы настоящий
// клиент. Раньше на мусоре процесс завершался с кодом 0, то есть клиент видел
// «штатное завершение» вместо ошибки.

// serveForTest поднимает Serve поверх канала и возвращает функцию «отправить
// строку» плюс функцию чтения ответов.
func serveForTest(t *testing.T, srv *Server) (func(string), func(time.Duration, int) []map[string]interface{}, func()) {
	t.Helper()
	in, inWriter := io.Pipe()
	var out lockedBuffer
	transport := NewTransport(in, &out)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(transport) }()

	send := func(line string) {
		if !strings.HasSuffix(line, "\n") {
			line += "\n"
		}
		if _, err := inWriter.Write([]byte(line)); err != nil {
			t.Fatalf("запись в stdin: %v", err)
		}
	}
	// readResponses ждёт, пока разобранных ответов станет не меньше atLeast:
	// «подождать ответа» и «убедиться, что ответа нет» — разные проверки, и
	// первая не должна возвращать уже накопленный хвост.
	readResponses := func(wait time.Duration, atLeast int) []map[string]interface{} {
		deadline := time.Now().Add(wait)
		for time.Now().Before(deadline) {
			parsed, complete := parseResponses(out.String())
			// Буфер читается целиком, поэтому недописанная строка — повод
			// подождать, а не «ответа нет».
			if complete && len(parsed) >= atLeast {
				return parsed
			}
			time.Sleep(20 * time.Millisecond)
		}
		return nil
	}
	stop := func() {
		_ = inWriter.Close()
		select {
		case err := <-done:
			if err != nil && err != io.EOF {
				t.Fatalf("Serve завершился ошибкой: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Serve не завершился")
		}
	}
	return send, readResponses, stop
}

func TestConformanceServeSurvivesGarbage(t *testing.T) {
	srv := testServer(t)
	send, readResponses, stop := serveForTest(t, srv)
	send("это не json")
	responses := readResponses(3*time.Second, 1)
	if len(responses) != 1 {
		t.Fatalf("ожидался один ответ на мусор: %v", responses)
	}
	if code := codeOf(t, responses[0]); code != -32700 {
		t.Fatalf("ожидался -32700: %v", responses[0])
	}
	if id, ok := responses[0]["id"]; !ok || id != nil {
		t.Fatalf("id ошибки разбора обязан быть null: %v", responses[0])
	}
	// Главное: сессия жива. Раньше здесь процесс уже завершался.
	send(`{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	responses = readResponses(3*time.Second, 2)
	if len(responses) < 2 {
		t.Fatalf("после мусора сервер не ответил на следующий запрос: %v", responses)
	}
	last := responses[len(responses)-1]
	if id, ok := last["id"].(float64); !ok || id != 7 {
		t.Fatalf("после мусора сервер не отвечает: %v", responses)
	}
	stop()
}

func TestConformanceBatchOrderPreserved(t *testing.T) {
	srv := testServer(t)
	send, readResponses, stop := serveForTest(t, srv)
	send(`[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"tools/list"}]`)
	responses := readResponses(3*time.Second, 2)
	if len(responses) != 2 {
		t.Fatalf("батч обязан дать два ответа: %v", responses)
	}
	if idOf(t, responses[0]) != 1 || idOf(t, responses[1]) != 2 {
		t.Fatalf("порядок ответов не совпал с порядком запросов: %v", responses)
	}
	// jsonrpc обязателен у каждого элемента, а не только у одиночного ответа:
	// подстановка «если пусто — 2.0» в Write до элементов батча не достаёт.
	for i, r := range responses {
		if r["jsonrpc"] != "2.0" {
			t.Fatalf("элемент %d батча без jsonrpc: %v", i, r)
		}
	}
	stop()
}

func TestConformanceBatchNotificationsSilent(t *testing.T) {
	srv := testServer(t)
	send, readResponses, stop := serveForTest(t, srv)
	send(`[{"jsonrpc":"2.0","method":"notifications/initialized"}]`)
	// Ответа быть не должно; проверяем, что следующий запрос всё равно
	// обслуживается — то есть батч из нотификаций не сломал цикл.
	send(`{"jsonrpc":"2.0","id":3,"method":"ping"}`)
	responses := readResponses(3*time.Second, 1)
	if len(responses) != 1 || idOf(t, responses[0]) != 3 {
		t.Fatalf("ответ не на тот запрос: %v", responses)
	}
	stop()
}

func TestConformanceCancelNotificationStopsRun(t *testing.T) {
	srv := testServer(t)
	// Ран, который живёт долго: плагин спит 30 секунд.
	plugDir := writeFakePluginDir(t, srv.pluginsDirs[0], "sleeper",
		"import sys,json,time\njson.load(sys.stdin)\ntime.sleep(30)\nprint('{\"status\":\"ok\",\"output\":{}}')\n")
	// Плагин создан ПОСЛЕ testServer(), который посчитал allow-list: без
	// пересчёта он остался бы недоверенным и на хосте без изолятора ран упал
	// бы сразу, не дожив до отмены (та же ловушка, что чинили в
	// TestMCPConcurrentCancelPicksLiveRun).
	srv.trust.Trusted = trustDirsIn(t, srv.pluginsDirs[0])
	yamlText := "format_version: \"0.2\"\npipeline:\n  name: cancel_probe\n  input: {}\n  steps:\n    - id: sleep\n      plugin: " + strconv.Quote(plugDir) + "\n      timeout: 30s\n"

	send, _, stop := serveForTest(t, srv)
	runReq := map[string]interface{}{
		"jsonrpc": "2.0", "id": 11, "method": "tools/call",
		"params": map[string]interface{}{
			"name": "run_pipeline", "arguments": map[string]interface{}{"yaml": yamlText, "wait_seconds": 30.0},
		},
	}
	raw, _ := json.Marshal(runReq)
	send(string(raw))

	// Ждём, пока ран действительно стартует (как в тесте конкурентной отмены).
	var runID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		runID = srv.currentRunID
		srv.mu.Unlock()
		if runID != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if runID == "" {
		t.Fatal("ран не стартовал")
	}

	// Отменяем НЕ инструментом cancel_run, а нотификацией — так делает
	// клиент, когда пользователь нажал Esc или оборвался запрос.
	send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":11,"reason":"user pressed esc"}}`)

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if status := srv.runStatus(runID); status == "cancelled" {
			stop()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	status := srv.runStatus(runID)
	stop()
	t.Fatalf("ран не отменён нотификацией, статус: %s", status)
}

func TestConformanceCancelBeforeStartApplies(t *testing.T) {
	// Нотификация может обогнать старт рана: клиент отменяет запрос сразу
	// после отправки. Отмена обязана примениться, а не потеряться.
	srv := testServer(t)
	call := &inflightCall{}
	key := "42"
	srv.inflight = map[string]*inflightCall{key: call}
	srv.cancelInflight(key, "обогнала старт")

	// Ран заводится уже после отмены — bindRun обязан его отменить.
	done := make(chan struct{})
	close(done)
	st := &runState{id: "late", done: done, status: "running"}
	srv.runs = map[string]*runState{"late": st}
	cancelCalled := false
	srv.cancels = map[string]context.CancelFunc{"late": func() { cancelCalled = true }}
	srv.bindRun(call, "late")
	if !cancelCalled {
		t.Fatal("отмена, пришедшая до старта рана, потеряна")
	}
}

// tools/list обязан объяснять хосту, что делает инструмент: без аннотаций
// запуск чужого кода и чтение каталога выглядят для него одинаково.
func TestConformanceToolAnnotations(t *testing.T) {
	defs := toolDefs()
	if len(defs) == 0 {
		t.Fatal("пустой список инструментов")
	}
	byName := map[string]Tool{}
	for _, d := range defs {
		if d.Annotations == nil {
			t.Fatalf("у инструмента %s нет аннотаций", d.Name)
		}
		for _, key := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
			if _, ok := d.Annotations[key]; !ok {
				t.Fatalf("инструмент %s: нет %s", d.Name, key)
			}
		}
		byName[d.Name] = d
	}
	readOnly := func(name string) bool { v, _ := byName[name].Annotations["readOnlyHint"].(bool); return v }
	destructive := func(name string) bool { v, _ := byName[name].Annotations["destructiveHint"].(bool); return v }
	for _, name := range []string{"list_plugins", "describe_plugin", "validate_pipeline", "plan_pipeline", "get_run"} {
		if !readOnly(name) || destructive(name) {
			t.Fatalf("%s обязан быть read-only и не destructive", name)
		}
	}
	for _, name := range []string{"run_pipeline", "exec_plugin"} {
		if readOnly(name) || !destructive(name) {
			t.Fatalf("%s запускает код и обязан быть destructive", name)
		}
	}
	if readOnly("cancel_run") || destructive("cancel_run") {
		t.Fatal("cancel_run ничего не разрушает и не read-only")
	}
}

// idOf — id ответа без паники на неожиданной форме.
func idOf(t *testing.T, resp map[string]interface{}) float64 {
	t.Helper()
	id, ok := resp["id"].(float64)
	if !ok {
		t.Fatalf("в ответе нет числового id: %v", resp)
	}
	return id
}

func TestConformanceUnknownToolIsInvalidParams(t *testing.T) {
	srv := testServer(t)
	_, _, rpcErr := srv.callTool(nil, "нет_такого", map[string]interface{}{})
	if rpcErr == nil || rpcErr.Code != -32602 {
		t.Fatalf("спека требует -32602 на неизвестный инструмент: %+v", rpcErr)
	}
}

// parseResponses — разбор накопленного вывода: строки JSON-RPC, каждая либо
// объект, либо массив (ответ на батч). complete=false означает «буфер ещё
// дописывается» — по нему вызывающий понимает, что нужно подождать.
func parseResponses(raw string) ([]map[string]interface{}, bool) {
	var parsed []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var one map[string]interface{}
		if err := json.Unmarshal([]byte(line), &one); err == nil {
			parsed = append(parsed, one)
			continue
		}
		var many []map[string]interface{}
		if err := json.Unmarshal([]byte(line), &many); err != nil {
			return nil, false
		}
		parsed = append(parsed, many...)
	}
	return parsed, true
}

// Ран переживает свой запрос: run_pipeline отдаёт ответ на waiting_human, а ран
// живёт дальше. Отмена, пришедшая после ответа, обязана находить ран — иначе
// единственный слот остаётся занятым и агент получает E_RUN_BUSY до таймаута.
// Живая проба поймала именно этот случай уже после того, как «отмена в вызове»
// заработала: блокирующий wait_seconds кончался раньше, чем приходила
// нотификация.
func TestConformanceCancelAfterResponseFindsRun(t *testing.T) {
	srv := testServer(t)
	req := &Request{ID: json.RawMessage(`5`)}
	call := srv.beginCall(nil, req)
	done := make(chan struct{})
	srv.runs = map[string]*runState{"run-1": {id: "run-1", done: done, status: "waiting_human"}}
	cancelled := false
	srv.cancels = map[string]context.CancelFunc{"run-1": func() { cancelled = true }}
	srv.bindRun(call, "run-1")
	srv.endCall(req) // ответ отправлен, вызова больше нет
	if _, ok := srv.inflight["5"]; ok {
		t.Fatal("вызов обязан исчезнуть из inflight после ответа")
	}
	srv.cancelInflight("5", "esc")
	if !cancelled {
		t.Fatal("ран, переживший свой запрос, не отменён нотификацией")
	}
}

// Владелец связи — ран, а не запрос: кончился ран — отменять нечего.
func TestConformanceCancelFinishedRunNoop(t *testing.T) {
	srv := testServer(t)
	srv.runRequests = map[string]string{"6": "run-2"}
	srv.runs = map[string]*runState{}
	srv.cancels = map[string]context.CancelFunc{}
	srv.cancelInflight("6", "поздно") // не должно паниковать и не должно «отменять»
	if len(srv.runRequests) != 1 {
		t.Fatalf("карта связей изменена: %v", srv.runRequests)
	}
}
