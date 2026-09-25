package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const maxJournalEventSize = 16 << 20

// P2 F-03: бюджеты чтения журнала. Раньше единственная точка чтения (Events)
// разбирала journal.jsonl целиком в []map — файл в сотни мегабайт съедал
// память процесса, а GET /api/runs делал это для каждого рана в каталоге и
// кодировал журнал в ответ без потолка. Полное чтение осталось там, где
// нужна вся история (resume по item_end, MaxItemIndex, сверка pipeline_hash),
// внешние/HTTP-пути идут через EventsBounded/ScanMeta с этими лимитами.
const (
	// DefaultReadByteBudget — сколько байт событий держим в памяти за чтение.
	DefaultReadByteBudget int64 = 64 << 20
	// DefaultReadEventBudget — сколько событий держим в памяти за чтение.
	DefaultReadEventBudget = 20000
	// maxMetaScanBytes — потолок потокового прохода (summary, max item
	// index): журнал больше — проход обрывается, Truncated=true.
	maxMetaScanBytes int64 = 256 << 20
)

// metaScanByteBudget — рабочее значение потолка ScanMeta (тесты уменьшают,
// чтобы не гонять 256 МБ).
var metaScanByteBudget int64 = maxMetaScanBytes

type Reader struct {
	Dir string
}

func NewReader(dir string) *Reader {
	return &Reader{Dir: dir}
}

// ReadLimits — потолки одного чтения журнала. Ноль или отрицательное значение
// поля означает «без лимита» (совместимое полное чтение). Tail=true оставляет
// последние события, иначе — первые (начиная с since).
type ReadLimits struct {
	MaxBytes  int64
	MaxEvents int
	Tail      bool
}

// DefaultReadLimits — консервативный потолок по умолчанию: хвост журнала в
// пределах DefaultReadByteBudget/DefaultReadEventBudget.
func DefaultReadLimits() ReadLimits {
	return ReadLimits{MaxBytes: DefaultReadByteBudget, MaxEvents: DefaultReadEventBudget, Tail: true}
}

// ReadResult — результат ограниченного чтения. Окно Events описывается
// парой First/Next — индексов первого и следующего за окном события, поэтому
// потерянные события видны всегда: First событий до окна и Total-Next после.
type ReadResult struct {
	// Events — окно событий в порядке журнала (при полном чтении — весь
	// журнал, при since за концом — пусто).
	Events []map[string]interface{}
	// First — индекс первого события окна, Next — индекс сразу за окном.
	First int
	Next  int
	// Total — событий насчитано в журнале; если проход упёрся в потолок,
	// это нижняя оценка, а не точное число.
	Total int
	// Bytes — байт событий, попавших в окно.
	Bytes int64
	// Truncated — окно не покрывает журнал: за ним остались события.
	Truncated bool
}

// Events — полное чтение журнала в память: все события, без обрезания.
// Нужен там, где важна вся история (resume по item_end, MaxItemIndex,
// сверка pipeline_hash). HTTP/API-пути используют EventsBounded/ScanMeta,
// чтобы память и тело ответа оставались ограниченными.
func (r *Reader) Events() ([]map[string]interface{}, error) {
	res, err := r.EventsBounded(0, ReadLimits{})
	if err != nil {
		return nil, err
	}
	return res.Events, nil
}

// EventsBounded — чтение журнала с бюджетом: в память попадает не больше
// limits.MaxBytes байт и limits.MaxEvents событий. since — индекс первого
// нужного события (курсор ?since=). Что осталось за окном, показывают
// First/Next/Total/Truncated, поэтому хвост догружается по курсору.
func (r *Reader) EventsBounded(since int, limits ReadLimits) (ReadResult, error) {
	f, err := os.Open(filepath.Join(r.Dir, "journal.jsonl"))
	if err != nil {
		return ReadResult{}, err
	}
	defer f.Close()
	if since < 0 {
		since = 0
	}
	if limits.MaxBytes <= 0 && limits.MaxEvents <= 0 {
		return readAllEvents(f, since)
	}
	return readEventWindow(f, since, limits)
}

// readAllEvents — полное чтение: строки декодируются прямо из буфера сканера,
// без копий и без окна (совместимо с прежним Events).
func readAllEvents(f *os.File, since int) (ReadResult, error) {
	var res ReadResult
	sc := newLineScanner(f)
	line, idx := 0, 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if idx < since {
			idx++
			continue
		}
		ev, err := decodeEvent(raw, line)
		if err != nil {
			return ReadResult{}, err
		}
		res.Events = append(res.Events, ev)
		res.Bytes += int64(len(raw))
		idx++
	}
	if err := sc.Err(); err != nil {
		return ReadResult{}, fmt.Errorf("journal: чтение: %w", err)
	}
	res.Total = idx
	res.First = idx - len(res.Events)
	res.Next = idx
	return res, nil
}

// readEventWindow — чтение с окном: строки вне окна не декодируются вовсе, в
// памяти остаётся только окно. Tail=true — окно едет за концом журнала и
// проход доходит до последнего события (Total точный). Иначе окно
// заполняется с позиции since, и проход обрывается на заполнении: хвост
// догружается следующим вызовом с since=Next (Total — нижняя оценка).
func readEventWindow(f *os.File, since int, limits ReadLimits) (ReadResult, error) {
	window := newLineWindow(limits.MaxBytes, limits.MaxEvents)
	size := fileSize(f)
	sc := newLineScanner(f)
	line, idx := 0, 0
	scanned := int64(0)
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		scanned += int64(len(raw)) + 1
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if idx < since {
			idx++
			continue
		}
		if !limits.Tail && window.full() {
			break
		}
		window.push(line, raw)
		idx++
	}
	if err := sc.Err(); err != nil {
		return ReadResult{}, fmt.Errorf("journal: чтение: %w", err)
	}
	more := false
	if !limits.Tail && window.full() {
		more = moreEventsAfter(sc, scanned, size)
	}
	events, kept, err := window.decode()
	if err != nil {
		return ReadResult{}, err
	}
	res := ReadResult{Events: events, Bytes: kept, Total: idx}
	if limits.Tail {
		res.First = idx - len(events)
		res.Next = idx
		res.Truncated = res.First > 0
		return res, nil
	}
	if since > idx {
		since = idx
	}
	res.First = since
	res.Next = since + len(events)
	res.Truncated = more
	return res, nil
}

// EventMeta — дешёвая проекция события: разбираются только нужные поля, тяжёлые
// payload пропускаются без аллокаций. Нужна потоковым проходам (summary
// рана, MaxItemIndex), где нужны ВСЕ события, но Events их в память не
// влезает.
type EventMeta struct {
	Type      string   `json:"type"`
	TS        string   `json:"ts"`
	Pipeline  string   `json:"pipeline"`
	Status    string   `json:"status"`
	Aborted   *float64 `json:"aborted"`
	ItemIndex *float64 `json:"item_index"`
}

// ScanResult — итог потокового прохода по журналу.
type ScanResult struct {
	// Total — событий разобрано (нижняя оценка, если Truncated).
	Total int
	// Bytes — байт журнала просмотрено.
	Bytes int64
	// Truncated — проход упёрся в maxMetaScanBytes, fn увидел не всё.
	Truncated bool
}

// ScanMeta прогоняет по журналу, отдавая EventMeta на каждое событие. Память
// O(1): события не накапливаются, в отличие от Events. Битый JSON и
// превышение maxMetaScanBytes возвращаются ошибкой/Truncated — вызывающий сам
// решает, что показывать.
func (r *Reader) ScanMeta(fn func(EventMeta) error) (ScanResult, error) {
	f, err := os.Open(filepath.Join(r.Dir, "journal.jsonl"))
	if err != nil {
		return ScanResult{}, err
	}
	defer f.Close()
	var res ScanResult
	sc := newLineScanner(f)
	line := 0
	for sc.Scan() {
		raw := sc.Bytes()
		line++
		res.Bytes += int64(len(raw)) + 1
		if res.Bytes > metaScanByteBudget {
			res.Truncated = true
			break
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var meta EventMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return res, fmt.Errorf("journal: строка %d: %w", line, err)
		}
		res.Total++
		if fn == nil {
			continue
		}
		if err := fn(meta); err != nil {
			return res, err
		}
	}
	if err := sc.Err(); err != nil {
		return res, fmt.Errorf("journal: чтение: %w", err)
	}
	return res, nil
}

func (r *Reader) ContextSnapshot() (map[string]interface{}, error) {
	raw, err := os.ReadFile(filepath.Join(r.Dir, "context.json"))
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func newLineScanner(f *os.File) *bufio.Scanner {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), maxJournalEventSize)
	return sc
}

func decodeEvent(raw []byte, line int) (map[string]interface{}, error) {
	var ev map[string]interface{}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, fmt.Errorf("journal: строка %d: %w", line, err)
	}
	return ev, nil
}

func fileSize(f *os.File) int64 {
	info, err := f.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// moreEventsAfter — есть ли ещё события в непрочитанном остатке файла (для
// head-окна: нужно ли клиенту догружать хвост по since=Next). Сканер уже не
// используется после проверки ошибок, поэтому «заглянуть» вперёд безопасно.
func moreEventsAfter(sc *bufio.Scanner, scanned, size int64) bool {
	if scanned >= size {
		return false
	}
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) > 0 {
			return true
		}
	}
	return false
}

type rawLine struct {
	line int
	data []byte
}

// lineWindow — кольцо сырых строк журнала: держит не больше maxBytes байт и
// maxEvents строк, вытесняя самые старые.
type lineWindow struct {
	items     []rawLine
	head      int
	bytes     int64
	maxBytes  int64
	maxEvents int
}

func newLineWindow(maxBytes int64, maxEvents int) *lineWindow {
	return &lineWindow{maxBytes: maxBytes, maxEvents: maxEvents}
}

func (w *lineWindow) len() int { return len(w.items) - w.head }

// full — бюджет окна исчерпан.
func (w *lineWindow) full() bool {
	if w.maxEvents > 0 && w.len() >= w.maxEvents {
		return true
	}
	return w.maxBytes > 0 && w.bytes >= w.maxBytes
}

// push — кладёт строку в окно, вытесняя самые старые под потолки. Строка
// больше всего бюджета остаётся в окне одна: иначе ответ был бы пустым.
func (w *lineWindow) push(line int, data []byte) {
	for w.maxEvents > 0 && w.len() >= w.maxEvents {
		w.dropOldest()
	}
	for w.maxBytes > 0 && w.len() > 0 && w.bytes+int64(len(data)) > w.maxBytes {
		w.dropOldest()
	}
	w.items = append(w.items, rawLine{line: line, data: append([]byte(nil), data...)})
	w.bytes += int64(len(data))
}

func (w *lineWindow) dropOldest() {
	if w.head >= len(w.items) {
		return
	}
	w.bytes -= int64(len(w.items[w.head].data))
	w.items[w.head] = rawLine{}
	w.head++
	if w.head > 64 && w.head*2 >= len(w.items) {
		w.items = append(w.items[:0], w.items[w.head:]...)
		w.head = 0
	}
}

// decode — разбирает только удержанные строки (строки вне окна не
// декодировались вовсе) и отдаёт их в порядке журнала.
func (w *lineWindow) decode() ([]map[string]interface{}, int64, error) {
	window := w.items[w.head:]
	events := make([]map[string]interface{}, 0, len(window))
	var kept int64
	for _, ln := range window {
		ev, err := decodeEvent(ln.data, ln.line)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, ev)
		kept += int64(len(ln.data))
	}
	return events, kept, nil
}
