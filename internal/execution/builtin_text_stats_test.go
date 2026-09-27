package execution

import (
	"testing"
)

// textStats обязан считать ровно так же, как community-плагин text_analyzer,
// иначе замена плагина на встроенный модуль тихо изменит числа в пайплайнах.
// Эталон взят с реального прогона Python-плагина на том же тексте.
func TestTextStatsMatchesPythonReference(t *testing.T) {
	cases := []struct {
		name                 string
		text                 string
		lines, words, unique int
		longest              string
	}{
		{
			name: "русский текст из демо",
			text: "Оркестратор цепочек\nКаждый шаг пишет свой журнал\nРешение остаётся за человеком",
			// Python: re.findall(r"\w+", re.UNICODE), unique = lower()
			lines: 3, words: 11, unique: 11, longest: "Оркестратор",
		},
		{
			name:  "текст из примера text_analyzer",
			text:  "Оркестратор цепочек\nКаждый шаг пишет журнал\nЖурнал доказуем",
			lines: 3, words: 8, unique: 7, longest: "Оркестратор",
		},
		{
			name:    "смешанный регистр считает unique по нижнему",
			text:    "Word word WORD",
			lines:   1,
			words:   3,
			unique:  1,
			longest: "Word", // max() берёт первый, не последний
		},
		{
			name:    "подчёркивание и цифры — часть слова",
			text:    "a_1 b2 c",
			lines:   1,
			words:   3,
			unique:  3,
			longest: "a_1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := textStats(tc.text)
			if err != nil {
				t.Fatalf("textStats: %v", err)
			}
			if got := out["lines"]; got != tc.lines {
				t.Errorf("lines = %v, ожидалось %d", got, tc.lines)
			}
			if got := out["words"]; got != tc.words {
				t.Errorf("words = %v, ожидалось %d", got, tc.words)
			}
			if got := out["unique_words"]; got != tc.unique {
				t.Errorf("unique_words = %v, ожидалось %d", got, tc.unique)
			}
			if got := out["longest_word"]; got != tc.longest {
				t.Errorf("longest_word = %v, ожидалось %q", got, tc.longest)
			}
		})
	}
}

func TestTextStatsRejectsEmptyText(t *testing.T) {
	for _, text := range []string{"", "   ", "\n\t "} {
		if _, err := textStats(text); err == nil {
			t.Errorf("пустой текст %q должен давать доменную ошибку, как у text_analyzer", text)
		}
	}
}

// Длина в rune, а не в байтах: иначе «Оркестратор» выиграл бы у любого ASCII-слова.
func TestTextStatsLongestCountsCodePoints(t *testing.T) {
	out, err := textStats("привет abc")
	if err != nil {
		t.Fatal(err)
	}
	if got := out["longest_word"]; got != "привет" {
		t.Errorf("longest_word = %v, ожидалось %q (6 рун против 3 байт)", got, "привет")
	}
}
