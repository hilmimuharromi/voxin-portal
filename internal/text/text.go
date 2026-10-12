// Package text has the small text helpers the portal needs: a safe Markdown
// renderer, slugs, and Indonesian date formatting.
package text

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var accents = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "ß", "ss")

// Slugify makes "Harga Beras Naik 10%" -> "harga-beras-naik-10".
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range accents.Replace(strings.ToLower(strings.TrimSpace(s))) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// drop accents and other scripts
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 90 {
		out = strings.TrimRight(out[:90], "-")
		if i := strings.LastIndex(out, "-"); i > 50 {
			out = out[:i]
		}
	}
	return out
}

var (
	reBold   = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	reItalic = regexp.MustCompile(`(^|[\s(])\*([^*\n]+)\*`)
	reLink   = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^)\s]+|/[^)\s]*)\)`)
	reOL     = regexp.MustCompile(`^\d+[.)]\s+`)
)

// inline escapes HTML first, then applies bold, italic and links.
func inline(s string) string {
	s = html.EscapeString(s)
	s = reLink.ReplaceAllStringFunc(s, func(m string) string {
		p := reLink.FindStringSubmatch(m)
		href := html.UnescapeString(p[2])
		ext := strings.HasPrefix(href, "http")
		attr := ""
		if ext {
			attr = ` target="_blank" rel="noopener nofollow"`
		}
		return `<a href="` + html.EscapeString(href) + `"` + attr + `>` + p[1] + `</a>`
	})
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = reItalic.ReplaceAllString(s, "$1<em>$2</em>")
	return s
}

// Markdown renders the subset editors use: ## / ### headings, paragraphs,
// - and 1. lists, > quotes, **bold**, *italic*, [links](url). Raw HTML is escaped.
func Markdown(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder
	var para []string
	list := ""
	var quote []string
	flushPara := func() {
		if len(para) > 0 {
			b.WriteString("<p>" + inline(strings.Join(para, " ")) + "</p>\n")
			para = nil
		}
	}
	flushList := func() {
		if list != "" {
			b.WriteString("</" + list + ">\n")
			list = ""
		}
	}
	flushQuote := func() {
		if len(quote) > 0 {
			b.WriteString("<blockquote><p>" + inline(strings.Join(quote, " ")) + "</p></blockquote>\n")
			quote = nil
		}
	}
	flushAll := func() { flushPara(); flushList(); flushQuote() }
	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		switch {
		case l == "":
			flushAll()
		case strings.HasPrefix(l, "### "):
			flushAll()
			b.WriteString("<h3>" + inline(strings.TrimSpace(l[4:])) + "</h3>\n")
		case strings.HasPrefix(l, "## "), strings.HasPrefix(l, "# "):
			flushAll()
			b.WriteString("<h2>" + inline(strings.TrimSpace(strings.TrimLeft(l, "#"))) + "</h2>\n")
		case strings.HasPrefix(l, "> "):
			flushPara()
			flushList()
			quote = append(quote, strings.TrimSpace(l[2:]))
		case strings.HasPrefix(l, "- "), strings.HasPrefix(l, "* "):
			flushPara()
			flushQuote()
			if list != "ul" {
				flushList()
				b.WriteString("<ul>\n")
				list = "ul"
			}
			b.WriteString("<li>" + inline(strings.TrimSpace(l[2:])) + "</li>\n")
		case reOL.MatchString(l):
			flushPara()
			flushQuote()
			if list != "ol" {
				flushList()
				b.WriteString("<ol>\n")
				list = "ol"
			}
			b.WriteString("<li>" + inline(reOL.ReplaceAllString(l, "")) + "</li>\n")
		default:
			flushList()
			flushQuote()
			para = append(para, l)
		}
	}
	flushAll()
	return b.String()
}

// Plain strips Markdown for meta descriptions and search snippets.
func Plain(md string) string {
	s := reLink.ReplaceAllString(md, "$1")
	s = strings.NewReplacer("**", "", "## ", "", "### ", "", "> ", "").Replace(s)
	s = reItalic.ReplaceAllString(s, "$1$2")
	return strings.Join(strings.Fields(s), " ")
}

// Excerpt cuts plain text at a word boundary.
func Excerpt(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}

var hari = []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
var bulan = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
var bulanPendek = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// Tanggal: "Kamis, 8 Oktober 2026".
func Tanggal(t time.Time) string {
	return fmt.Sprintf("%s, %d %s %d", hari[t.Weekday()], t.Day(), bulan[t.Month()-1], t.Year())
}

// Waktu: "Kamis, 8 Okt 2026 · 09.12 WIB".
func Waktu(t time.Time) string {
	zone, _ := t.Zone()
	if zone == "" || strings.HasPrefix(zone, "+") {
		zone = "WIB"
	}
	return fmt.Sprintf("%s, %d %s %d · %02d.%02d %s", hari[t.Weekday()], t.Day(), bulanPendek[t.Month()-1], t.Year(), t.Hour(), t.Minute(), zone)
}

// Lalu: "5 menit lalu", "3 jam lalu", else a short date.
func Lalu(t time.Time, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "baru saja"
	case d < time.Hour:
		return fmt.Sprintf("%d menit lalu", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d jam lalu", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d hari lalu", int(d.Hours()/24))
	}
	return fmt.Sprintf("%d %s %d", t.Day(), bulanPendek[t.Month()-1], t.Year())
}

// ReadMinutes estimates reading time at 200 words per minute.
func ReadMinutes(md string) int {
	n := len(strings.Fields(md)) / 200
	if n < 1 {
		return 1
	}
	return n
}

// SplitHighlight splits a title around a highlight phrase (case-insensitive).
func SplitHighlight(title, hl string) (pre, mid, post string) {
	if hl == "" {
		return title, "", ""
	}
	i := strings.Index(strings.ToLower(title), strings.ToLower(hl))
	if i < 0 {
		return title, "", ""
	}
	return title[:i], title[i : i+len(hl)], title[i+len(hl):]
}

// Lines splits text into non-empty trimmed lines without list numbering.
func Lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(reOL.ReplaceAllString(strings.TrimSpace(l), ""))
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(l, "- "), "• "))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
