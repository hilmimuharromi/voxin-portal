package store

import "testing"

func TestFooterDefaults(t *testing.T) {
	var s SiteSettings
	cols := s.FooterCols("VOXIN")
	if len(cols) != 2 || cols[0].Title != "Redaksi" || cols[0].Links[4].Label != "Tentang VOXIN" {
		t.Fatalf("default cols: %+v", cols)
	}
	if got := s.CopyrightLine(2026, "VOXIN"); got != "© 2026 VOXIN." {
		t.Fatalf("copyright: %q", got)
	}
	if s.FooterNote() != DefaultFooterNote {
		t.Fatal("default note")
	}
	s.Footer.Note = "-"
	if s.FooterNote() != "" {
		t.Fatal("dash hides note")
	}
	s.Footer.Copyright = "Hak cipta {nama} {tahun}"
	if got := s.CopyrightLine(2027, "X"); got != "Hak cipta X 2027" {
		t.Fatalf("custom copyright: %q", got)
	}
	s.Instagram, s.X = "https://instagram.com/voxin", "https://x.com/voxin"
	if l := s.SocialLinks(); len(l) != 2 || l[0].Label != "Instagram" || l[1].Label != "X" {
		t.Fatalf("social: %+v", l)
	}
}
