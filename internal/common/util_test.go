package common

import (
	"testing"
	"unicode/utf8"
)

// v0.15: Truncate не должен резать UTF-8 символ пополам.
// До правки s[:n] на кириллице давал битые байты.
func TestTruncateRuneSafe(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello world", 5, "hello…"},
		{"hi", 10, "hi"},
		{"абвгд", 5, "аб…"}, // 5-й байт падает в середину «в»
		{"аbв", 4, "аb…"},   // микс ascii/кириллица
		{"😀😀", 5, "😀…"},     // эмодзи по 4 байта
	}
	for _, c := range cases {
		got := Truncate(c.in, c.n)
		if !utf8.ValidString(got) {
			t.Fatalf("in=%q n=%d: результат не валидный UTF-8: %q", c.in, c.n, got)
		}
		if got != c.want {
			t.Fatalf("in=%q n=%d: got %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
