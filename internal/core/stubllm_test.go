package core

// Заглушка LLM-провайдера для тестов, которым нужен успешный ответ.
//
// Раньше эти тесты включали `LLM_MOCK=1` через окружение процесса, и переменная
// доходила до плагина ТОЛЬКО потому, что манифест объявлял `LLM_MOCK` в
// permissions.secrets. Это была та же дверь, через которую переключатель
// достался бы и до обычного запуска, поэтому дверь закрыли — а вместе с ней
// упали и тесты.
//
// Правильный путь тот же, что у соседних тестов в llm_plugins_test.go: поднять
// локальный сервер и указать его через *_BASE_URL. Никакого нового канала в
// production-коде не появляется, а тест заодно проходит настоящий HTTP-путь
// плагина, а не заглушку внутри него.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// llmFixture описывает провайдера с двух сторон: как его зовут в окружении и в
// каком формате он ждёт ответ. Обе стороны нужны, и названия env у плагинов
// разные (у llm_openai это LLM_OAI_*, у llm_gemini — GEMINI_*), поэтому
// таблица, а не соглашение по имени.
type llmFixture struct {
	baseURLEnv string
	apiKeyEnv  string
	reply      func(text string) string
	prompt     func(body []byte) string
}

var llmFixtures = map[string]llmFixture{
	"gemini": {
		baseURLEnv: "GEMINI_BASE_URL",
		apiKeyEnv:  "GEMINI_API_KEY",
		reply: func(text string) string {
			return fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":%s}]}}]}`, jsonString(text))
		},
		prompt: geminiPrompt,
	},
	"openai": {
		baseURLEnv: "LLM_OAI_BASE_URL",
		apiKeyEnv:  "LLM_OAI_API_KEY",
		reply: func(text string) string {
			return fmt.Sprintf(`{"choices":[{"message":{"content":%s}}]}`, jsonString(text))
		},
		// openaiPrompt берёт ПОСЛЕДНЕЕ сообщение, а не первое: плагин
		// складывает system-сообщение ПЕРВЫМ, и взятие первого возвращало
		// системный промпт вместо пользовательского. Из-за этого эхо не
		// содержало текста черновика, и проверка цепочки падала вручную —
		// то есть заглушка врала, а тест это показывал.
		prompt: openaiPrompt,
	},
	"anthropic": {
		baseURLEnv: "ANTHROPIC_BASE_URL",
		apiKeyEnv:  "ANTHROPIC_API_KEY",
		reply: func(text string) string {
			return fmt.Sprintf(`{"content":[{"type":"text","text":%s}]}`, jsonString(text))
		},
		prompt: anthropicPrompt,
	},
}

// stubLLM поднимает локальный ответ провайдера и прописывает его адрес в
// окружение через t.Setenv (то есть автоматически откатывается).
//
// Пустой reply означает ЭХО: провайдер возвращает сам prompt с префиксом
// "эхо: ". Это сильнее, чем был mock с маркером "[mock:": тест цепочки может
// доказать, что второй шаг получил выход первого, потому что содержимое
// ответа буквально равно входу. С mock это выражалось подсчётом маркеров, то
// есть проверкой формы, а не содержимого.
func stubLLM(t *testing.T, provider, reply string) {
	t.Helper()
	fx, ok := llmFixtures[provider]
	if !ok {
		t.Fatalf("заглушка не знает провайдера %q (известны: %s)", provider, strings.Join(knownProviders(), ", "))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		text := reply
		if text == "" {
			text = "эхо: " + fx.prompt(body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, fx.reply(text))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(fx.baseURLEnv, srv.URL)
	t.Setenv(fx.apiKeyEnv, "test-key")
	// Явно выключаем мок, даже если его больше нет в манифестах: тест не
	// должен зависеть от того, просочится ли переключатель в этот процесс.
	t.Setenv("LLM_MOCK", "")
}

func knownProviders() []string {
	out := make([]string, 0, len(llmFixtures))
	for name := range llmFixtures {
		out = append(out, name)
	}
	return out
}

func jsonString(s string) string {
	raw, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(raw)
}

func geminiPrompt(body []byte) string {
	var payload struct {
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Contents) == 0 || len(payload.Contents[0].Parts) == 0 {
		return ""
	}
	return payload.Contents[0].Parts[0].Text
}

// openaiPrompt берёт сообщение с ролью user, а не последнее: плагин кладёт
// system-сообщение ПЕРВЫМ, когда system задан (llm_openai, main.py), и
// последним — когда не задан. Взять «последнее» значило вернуть system там,
// где он есть, и ничего там, где его нет; в обоих случаях эхо получалось не
// тем, что проверяет тест. Роль user — единственный признак, который верен
// всегда.
func openaiPrompt(body []byte) string {
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, m := range payload.Messages {
		if m.Role == "user" {
			return m.Content
		}
	}
	if len(payload.Messages) > 0 {
		return payload.Messages[len(payload.Messages)-1].Content
	}
	return ""
}

func anthropicPrompt(body []byte) string {
	var payload struct {
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Messages) == 0 {
		return ""
	}
	last := payload.Messages[len(payload.Messages)-1].Content
	if len(last) == 0 {
		return ""
	}
	return last[0].Text
}
