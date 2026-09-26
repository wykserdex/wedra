package pipeline

import "testing"

func TestLooksLikeHostPath(t *testing.T) {
	cases := map[string]bool{
		`C:\Users\me\file.txt`:  true,
		`C:/Users/me/file.txt`:  true,
		`/etc/passwd`:           true,
		`//server/share/x`:      true,
		`../../etc/passwd`:      true,
		`data/../../escape`:     true,
		`sub/dir/file.txt`:      false,
		`./file.txt`:            false,
		`file.txt`:              false,
		``:                      false,
		`glob/*.txt`:            false,
		`multi\nline`:           false,
		`..`:                    true,
		`a/..`:                  true,
		`C:`:                    true,
		`relative/ok/path.json`: false,
	}
	for in, want := range cases {
		if got := looksLikeHostPath(in); got != want {
			t.Errorf("looksLikeHostPath(%q) = %v, ожидалось %v", in, got, want)
		}
	}
}

func TestStaticInputKey(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"input.path", "path", true},
		{"input.a.b", "a", true},
		{"input.", "", false},
		{"steps.s1.out", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := staticInputKey(c.in)
		if ok != c.wantOK || got != c.want {
			t.Errorf("staticInputKey(%q) = (%q,%v), ожидалось (%q,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}
