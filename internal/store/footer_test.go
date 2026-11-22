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

func TestImageFocus(t *testing.T) {
	a := Article{ImageW: 1080, ImageH: 1350}
	if !a.ImagePortrait() || a.ImageFocus() != "50% 15%" || a.ImageHeightFor(1200) != 1500 {
		t.Fatalf("portrait: %v %q %d", a.ImagePortrait(), a.ImageFocus(), a.ImageHeightFor(1200))
	}
	a = Article{ImageW: 1600, ImageH: 900, ImageFocusRaw: "40% 10%"}
	if a.ImageFocus() != "40% 10%" {
		t.Fatal(a.ImageFocus())
	}
	for _, bad := range []string{"50%", "101% 2%", "50% 30%; color:red", "a b"} {
		if ValidFocus(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if (Article{}).ImageHeightFor(1200) != 675 {
		t.Fatal("fallback 16:9")
	}
}
