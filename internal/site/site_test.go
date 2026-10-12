package site

import "testing"

func TestSource(t *testing.T) {
	cases := map[string]string{
		"":                             "langsung",
		"https://www.google.com/":      "search",
		"https://l.instagram.com/?u=x": "sosmed",
		"https://t.co/abc":             "sosmed",
		"https://kompas.com/a":         "lainnya",
		"https://voxin.id/politik/x":   "internal",
	}
	for ref, want := range cases {
		if got := Source(ref, "https://voxin.id"); got != want {
			t.Errorf("%q -> %s, want %s", ref, got, want)
		}
	}
	if !isBot("Mozilla/5.0 (compatible; Googlebot/2.1)") || isBot("Mozilla/5.0 (iPhone)") {
		t.Error("deteksi bot salah")
	}
}
