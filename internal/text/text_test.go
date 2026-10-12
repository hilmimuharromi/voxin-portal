package text

import (
	"strings"
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{"Harga Beras Naik 10%!": "harga-beras-naik-10", "  Cek Fakta: Hoaks?  ": "cek-fakta-hoaks", "Café Résumé": "cafe-resume"}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownSafe(t *testing.T) {
	md := "Halo **tebal** dan *miring*.\n\n## Judul\n\n- satu\n- dua\n\n1. a\n2. b\n\n> kutipan\n\n<script>alert(1)</script> [link](https://x.id) [jahat](javascript:alert(1))"
	h := Markdown(md)
	for _, want := range []string{"<strong>tebal</strong>", "<em>miring</em>", "<h2>Judul</h2>", "<ul>\n<li>satu</li>", "<ol>\n<li>a</li>", "<blockquote><p>kutipan</p></blockquote>",
		"&lt;script&gt;", `<a href="https://x.id" target="_blank" rel="noopener nofollow">link</a>`} {
		if !strings.Contains(h, want) {
			t.Errorf("tidak ada %q di:\n%s", want, h)
		}
	}
	if strings.Contains(h, "<script>") || strings.Contains(h, `href="javascript`) {
		t.Fatalf("HTML berbahaya lolos:\n%s", h)
	}
}

func TestDates(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	d := time.Date(2026, 10, 8, 9, 12, 0, 0, loc)
	if got := Tanggal(d); got != "Kamis, 8 Oktober 2026" {
		t.Error(got)
	}
	if got := Waktu(d); got != "Kamis, 8 Okt 2026 · 09.12 WIB" {
		t.Error(got)
	}
	if got := Lalu(d, d.Add(3*time.Hour)); got != "3 jam lalu" {
		t.Error(got)
	}
	pre, mid, post := SplitHighlight("Google Luncurkan SynthID", "synthid")
	if pre != "Google Luncurkan " || mid != "SynthID" || post != "" {
		t.Error(pre, mid, post)
	}
}
