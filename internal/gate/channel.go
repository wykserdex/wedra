package gate

import (
	"errors"
	"io"
	"sync/atomic"
)

// ErrClosed — ввод гейта закрыт (Close). Семантика v0.23: EOF — это стоп,
// не молчаливое accept.
var ErrClosed = errors.New("ввод гейта закрыт (EOF)")

// Decision — структурированное решение гейта (v0.24, GUI/API):
// действие + правки полей формы (ключ — полный путь из события gate_wait).
type Decision struct {
	Action string                 `json:"action"`
	Edits  map[string]interface{} `json:"edits"`
}

type signal struct {
	d   Decision
	eof bool
}

// ChannelUI — GateUI поверх канала (v0.24: гейт из браузера).
// Service.runStructured вызывает WaitDecision (блокирует ран), решение
// приходит из API: POST /api/runs/<id>/gate → SendDecision.
// Close() — EOF (вкладку закрыли до решения, ран убивают и т.п.) — терминален.
//
// ReadLine здесь не используется: построчный поток не подходит для браузера
// (нельзя «печатать в промпт») — структурированный обмен вместо N строк.
// Терминальный путь Service.Run не тронут.
//
// Повторные решения поддерживаются: сервис после gate_retry снова входит в
// WaitDecision (retry-цикл ≤5, как в терминале). state = «сигнал в очереди»:
// двойной submit/Close пока pending → false (API ответит 409), отправка в
// канал невозможна без выигранного CAS — буфер никогда не затыкается.
// EOF после потребления остаётся терминальным (state=1 навсегда).
type ChannelUI struct {
	state int32 // 0 = ожидает решение, 1 = сигнал в очереди / терминально закрыто
	ch    chan signal
}

func NewChannelUI() *ChannelUI {
	return &ChannelUI{ch: make(chan signal, 1)}
}

// SendDecision — поставить решение. false: уже есть pending сигнал или EOF.
func (c *ChannelUI) SendDecision(d Decision) bool {
	for {
		if atomic.LoadInt32(&c.state) == 1 {
			return false
		}
		if atomic.CompareAndSwapInt32(&c.state, 0, 1) {
			c.ch <- signal{d: d}
			return true
		}
	}
}

// Close — EOF. false: уже есть pending сигнал или EOF.
func (c *ChannelUI) Close() bool {
	for {
		if atomic.LoadInt32(&c.state) == 1 {
			return false
		}
		if atomic.CompareAndSwapInt32(&c.state, 0, 1) {
			c.ch <- signal{eof: true}
			return true
		}
	}
}

// WaitDecision — блокируется до SendDecision/Close. После потребления
// решения (не EOF) возвращает канал в режим ожидания — сервис сможет
// дождаться следующего решения в retry-цикле.
func (c *ChannelUI) WaitDecision() (Decision, error) {
	s := <-c.ch
	if s.eof {
		return Decision{}, ErrClosed
	}
	atomic.StoreInt32(&c.state, 0)
	return s.d, nil
}

// ReadLine — построчный режим в ChannelUI не используется (Service.Run идёт
// через StructuredUI). Если терминальный путь всё же достанется ему —
// немедленный EOF (стоп по семантике v0.23), а не захват чужого stdin.
func (c *ChannelUI) ReadLine() (string, error) { return "", io.EOF }
