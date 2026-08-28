package core

// Контракт-тесты LLM-адаптеров (пак B из ТЗ v2).
// Плагины гоняются через реальный subprocess-протокол против локальных
// httptest-серверов, эмулирующих wire-формат каждого провайдера —
// никакой атрибутики, настоящий HTTP до localhost.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// llmFx — манифест продакшн-плагина из ../../plugins/<name> (рабочая папка теста — internal/core).
func llmFx(name string) *Manifest {
	dir := filepath.Join("..", "..", "plugins", name)
	m, err := NewEngine().LoadManifest(dir)
	if err != nil {
		return &Manifest{ID: name, Dir: dir, Runtime: Runtime{Type: "python", Entry: "main.py"}}
	}
	return m
}

// ── Gemini ──────────────────────────────────────────────────────────────

func TestGeminiOK(t *testing.T) {
	requirePython(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		// Go-бэкенд плагина экранирует кириллицу в \uXXXX — парсим, а не матчим подстроку
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("plugin отправил не-JSON: %v", err)
		}
		got := payload["contents"].([]interface{})[0].(map[string]interface{})["parts"].([]interface{})[0].(map[string]interface{})["text"]
		if got != "арбузы" {
			t.Errorf("prompt не долетел до провайдера: %v", got)
		}
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"Арбузы полезны, потому что..."}]}}]}`)
	}))
	defer srv.Close()

	t.Setenv("GEMINI_BASE_URL", srv.URL)
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("LLM_MOCK", "")

	res := execPlugin(llmFx("llm_gemini"),
		[]byte(`{"prompt":"арбузы","system":"Ты — копирайтер"}`), 5*time.Second)
	if !res.OK() {
		t.Fatalf("gemini ok ожидался: %+v", res)
	}
	if !strings.Contains(res.Output["text"].(string), "Арбузы") {
		t.Fatalf("text не из ответа провайдера: %v", res.Output)
	}
	if res.Output["model"] == "" {
		t.Fatal("output.model обязателен по манифесту")
	}
}

func TestGemini429IsRetryableDomain(t *testing.T) {
	requirePython(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t.Setenv("GEMINI_BASE_URL", srv.URL)
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("LLM_MOCK", "")

	res := execPlugin(llmFx("llm_gemini"), []byte(`{"prompt":"x"}`), 5*time.Second)
	if res.OK() || res.Platform || !res.Retryable || res.ErrCode != "http_429" {
		t.Fatalf("429 → доменная retryable ошибка: %+v", res)
	}
	if !res.shouldRetry() {
		t.Fatal("429 обязана ретраиться")
	}
}

func TestGemini401NotRetryable(t *testing.T) {
	requirePython(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	t.Setenv("GEMINI_BASE_URL", srv.URL)
	t.Setenv("GEMINI_API_KEY", "bad")
	t.Setenv("LLM_MOCK", "")

	res := execPlugin(llmFx("llm_gemini"), []byte(`{"prompt":"x"}`), 5*time.Second)
	if res.Retryable || res.shouldRetry() {
		t.Fatalf("401 — не retryable (ключ мёртв): %+v", res)
	}
}

func TestGeminiNoKey(t *testing.T) {
	requirePython(t)
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("LLM_MOCK", "")
	res := execPlugin(llmFx("llm_gemini"), []byte(`{"prompt":"x"}`), 5*time.Second)
	if res.OK() || res.Platform || res.ErrCode != "no_api_key" || res.Retryable {
		t.Fatalf("отсутствие ключа → понятная доменная ошибка, не traceback: %+v", res)
	}
}

// ── OpenAI-совместимый (Grok через env) ─────────────────────────────────

func TestOpenAICompatOK(t *testing.T) {
	requirePython(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer grok-key" {
			t.Errorf("нет Bearer-авторизации")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"доработанный текст про арбузы"}}]}`)
	}))
	defer srv.Close()

	t.Setenv("LLM_OAI_BASE_URL", srv.URL)
	t.Setenv("LLM_OAI_API_KEY", "grok-key")
	t.Setenv("LLM_MOCK", "")

	res := execPlugin(llmFx("llm_openai"),
		[]byte(`{"prompt":"черновик","system":"Ты — редактор"}`), 5*time.Second)
	if !res.OK() {
		t.Fatalf("openai-compat ok ожидался: %+v", res)
	}
	if !strings.Contains(res.Output["text"].(string), "арбузы") {
		t.Fatalf("text: %v", res.Output)
	}
}

// ── Anthropic ───────────────────────────────────────────────────────────

func TestAnthropicOK(t *testing.T) {
	requirePython(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "ant-key" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("нет заголовков anthropic")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"content":[{"text":"ответ клода"}]}`)
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "ant-key")
	t.Setenv("LLM_MOCK", "")

	res := execPlugin(llmFx("llm_anthropic"), []byte(`{"prompt":"тема"}`), 5*time.Second)
	if !res.OK() {
		t.Fatalf("anthropic ok ожидался: %+v", res)
	}
	if res.Output["text"] != "ответ клода" {
		t.Fatalf("text: %v", res.Output)
	}
}

// ── Mock-режим: демо без ключей и без сети ──────────────────────────────

func TestMockModeNoKeyNoNetwork(t *testing.T) {
	requirePython(t)
	t.Setenv("LLM_MOCK", "1")
	t.Setenv("GEMINI_API_KEY", "") // демонстрация: ключ не нужен

	res := execPlugin(llmFx("llm_gemini"), []byte(`{"prompt":"арбузы"}`), 5*time.Second)
	if !res.OK() || !strings.HasPrefix(res.Output["text"].(string), "[mock:") {
		t.Fatalf("mock-режим: %+v", res)
	}
	// проходит enforce манифеста: text, model объявлены и возвращены
	if _, _, err := EnforceOutput(llmFx("llm_gemini"), res.Output); err != nil {
		t.Fatalf("mock-ответ не прошёл контракт: %v", err)
	}
}

// ── Валидация входа плагином ────────────────────────────────────────────

func TestLLMEmptyPromptIsDomainError(t *testing.T) {
	requirePython(t)
	t.Setenv("LLM_MOCK", "1")
	res := execPlugin(llmFx("llm_gemini"), []byte(`{"prompt":""}`), 5*time.Second)
	if res.OK() || res.Platform || res.ErrCode != "empty_prompt" {
		t.Fatalf("пустой prompt → доменная ошибка: %+v", res)
	}
}

func TestLLMBadJSONIsPlatform(t *testing.T) {
	requirePython(t)
	t.Setenv("LLM_MOCK", "1")
	res := execPlugin(llmFx("llm_openai"), []byte(`{oops`), 5*time.Second)
	if !res.Platform {
		t.Fatalf("битый JSON на входе → платформенная ошибка (exit 2): %+v", res)
	}
}

// ── E2E: весь пак B через раннер в mock-режиме ──────────────────────────
//   draft(gemini) → human_gate(правки человека) → refine(openai)
// Проверяет главный пользовательский сценарий ТЗ.

func TestLLMChainEndToEndMock(t *testing.T) {
	requirePython(t)
	t.Setenv("LLM_MOCK", "1")

	pf := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name: "t_llm_chain",
			Input: map[string]interface{}{
				"topic":         "арбузы",
				"draft_system":  "пиши коротко",
				"refine_system": "правь строго",
			},
			Steps: []Step{
				{ID: "draft", Plugin: filepath.Join("..", "..", "plugins", "llm_gemini"),
					OnError: "retry", Timeout: sec(10),
					Retry: &Retry{Attempts: 2, Delay: msec(1)}},
				{ID: "review", Plugin: "core/human_gate",
					Form: []FormField{{Field: "steps.draft.text", Editable: true, Type: "string"}}},
				{ID: "refine", Plugin: filepath.Join("..", "..", "plugins", "llm_openai"),
					OnError: "stop", Timeout: sec(10)},
			},
		},
	}

	opts := quietOpts(t)
	opts.Yes = true // человек принял черновик без правок
	stats, err := Run(pf, NewEngine(), opts)
	if err != nil {
		t.Fatalf("цепочка пака B упала: %v", err)
	}
	if stats.OK != 1 {
		t.Fatalf("статы: %+v", stats)
	}

	snap := readContextSnapshot(t, stats.RunDir)
	steps := snap["steps"].(map[string]interface{})

	draft := steps["draft"].(map[string]interface{})["text"].(string)
	if !strings.Contains(draft, "арбузы") {
		t.Fatalf("черновик: %q", draft)
	}
	// гейт МАТЕРИАЛИЗОВАЛ одобренный текст в свой неймспейс
	review := steps["review"].(map[string]interface{})["text"].(string)
	if review != draft {
		t.Fatalf("гейт не материализовал текст: review=%q draft=%q", review, draft)
	}
	// refine получил текст из гейта (prompt = steps.review.text), а не из draft напрямую
	refine := steps["refine"].(map[string]interface{})["text"].(string)
	if !strings.Contains(refine, "доработанный") {
		t.Fatalf("refine не отработал: %q", refine)
	}

	events := readEvents(t, stats.RunDir)
	if countEvents(events, "gate_decision") != 1 {
		t.Fatal("решение гейта должно быть в журнале")
	}
}

// ── E2E: человек ОТРЕДАКТИРОВАЛ текст — refine получает правку ─────────

func TestLLMChainHumanEditFlowsDownstream(t *testing.T) {
	requirePython(t)
	t.Setenv("LLM_MOCK", "1")
	// „aпринял с правкой”: на prompt правки — новый текст, на действие — accept
	newStdin(t, "\"отредактировано человеком\"\na\n")

	pf := &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:  "t_llm_edit",
			Input: map[string]interface{}{"topic": "x", "refine_system": "y"},
			Steps: []Step{
				{ID: "draft", Plugin: filepath.Join("..", "..", "plugins", "llm_gemini"),
					OnError: "stop", Timeout: sec(10)},
				{ID: "review", Plugin: "core/human_gate",
					Form: []FormField{{Field: "steps.draft.text", Editable: true, Type: "string"}}},
				{ID: "refine", Plugin: filepath.Join("..", "..", "plugins", "llm_openai"),
					OnError: "stop", Timeout: sec(10)},
			},
		},
	}
	stats, err := Run(pf, NewEngine(), quietOpts(t))
	if err != nil {
		t.Fatal(err)
	}
	if stats.OK != 1 {
		t.Fatalf("статы: %+v", stats)
	}

	snap := readContextSnapshot(t, stats.RunDir)
	steps := snap["steps"].(map[string]interface{})
	review := steps["review"].(map[string]interface{})["text"].(string)
	if review != "отредактировано человеком" {
		t.Fatalf("правка не легла в неймспейс гейта: %q", review)
	}
	// источник (draft) не затёрт
	draft := steps["draft"].(map[string]interface{})["text"].(string)
	if strings.Contains(draft, "отредактировано") {
		t.Fatal("правка человека затёрла исходный выход draft")
	}
	// refine видит ПРАВКУ (mock-ответ строится из prompt)
	refine := steps["refine"].(map[string]interface{})["text"].(string)
	if !strings.Contains(refine, "отредактировано человеком") {
		t.Fatalf("refine получил не правку: %q", refine)
	}
}
