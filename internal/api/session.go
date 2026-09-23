package api

// v0.9: сессия человека (модель Jupyter-токена). `wedra gui` генерирует
// секрет и печатает ссылку ?k=<secret> в терминал человека; первый заход
// ставит cookie (HttpOnly, SameSite=Strict). Мутирующие эндпоинты (запуск,
// решение гейта, отмена, сохранение пайплайна) без cookie → 401.
//
// Модель угроз (честно): защищает от того, что агент случайно или по
// инструкции одобрит свой гейт через доступные ему инструменты (curl, MCP).
// От злонамеренного процесса того же пользователя ОС не защищает.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"

	"wedra/internal/gate"
)

const sessionCookie = "wedra_session"

// NewSessionSecret — 128 бит из crypto/rand, hex.
func NewSessionSecret() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// EnableSession — включить требование сессии. Пустой секрет — выключить.
func (s *Server) EnableSession(secret string) {
	s.SessionSecret = secret
}

// sessionHash — то, что пишется в журнал вместо секрета (gate_decision.session).
func (s *Server) sessionHash() string {
	if s.SessionSecret == "" {
		return ""
	}
	h := sha256.Sum256([]byte("wedra-session:" + s.SessionSecret))
	return hex.EncodeToString(h[:6])
}

func (s *Server) validSecret(v string) bool {
	if s.SessionSecret == "" || v == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(v), []byte(s.SessionSecret)) == 1
}

// hasSession — сессия выключена (тесты, старое поведение) или cookie верный.
func (s *Server) hasSession(r *http.Request) bool {
	if s.SessionSecret == "" {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.validSecret(c.Value)
}

// requireSession — 401 без cookie. true — можно продолжать.
func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) bool {
	if s.hasSession(r) {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(401)
	w.Write([]byte(`{"error":"нужна сессия человека: откройте ссылку ?k=... из терминала wedra","code":"E_SESSION_REQUIRED"}` + "\n"))
	return false
}

// sessionHandshake — GET ...?k=<secret>: ставит cookie и редиректит на тот
// же путь без k (секрет не остаётся в адресной строке/истории).
// true — ответ уже записан.
func (s *Server) sessionHandshake(w http.ResponseWriter, r *http.Request) bool {
	if s.SessionSecret == "" || r.Method != "GET" {
		return false
	}
	k := r.URL.Query().Get("k")
	if k == "" {
		return false
	}
	if !s.validSecret(k) {
		http.Error(w, "неверный ключ сессии", 401)
		return true
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: s.SessionSecret, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	q := r.URL.Query()
	q.Del("k")
	u := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
	return true
}

// ── v0.9: встраивание (wedra mcp) ────────────────────────────────────────
// MCP-сервер исполняет раны сам, а решения гейтов и отмена приходят от
// человека через этот HTTP-сервер (с сессией). Регистрация внешнего рана
// делает его гейт/отмену доступными по /api/runs/<id>/gate|cancel.

// AttachRun — зарегистрировать гейт и/или отмену внешнего рана.
func (s *Server) AttachRun(id string, ui *gate.ChannelUI, cancel context.CancelFunc) {
	if ui != nil {
		s.setGate(id, ui)
	}
	if cancel != nil {
		s.setCancel(id, cancel)
	}
}

// DetachRun — ран завершён: гейт закрыт, отмена снята.
func (s *Server) DetachRun(id string) {
	s.clearGate(id)
	s.clearCancel(id)
}
