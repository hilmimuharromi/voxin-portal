package cms

import "testing"

func TestParseFooter(t *testing.T) {
	ft, err := parseFooter([]string{"Redaksi", "", "Bisnis", ""}, []string{"Kontak | /kontak\r\n\r\nEmail | mailto:a@b.id\n", "", "Iklan | https://voxin.id/iklan", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(ft.Columns) != 2 || len(ft.Columns[0].Links) != 2 || ft.Columns[0].Links[1].URL != "mailto:a@b.id" {
		t.Fatalf("%+v", ft)
	}
	for _, bad := range []string{"Jahat | javascript:alert(1)", "Tanpa url", "Proto | //evil.com"} {
		if _, err := parseFooter([]string{"A"}, []string{bad}); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
	if _, err := parseFooter([]string{"", ""}, []string{"", ""}); err == nil {
		t.Error("empty footer should fail")
	}
}

func TestAnalyticsInputs(t *testing.T) {
	if v, err := normGA4(" g-ab12cd34ef "); err != nil || v != "G-AB12CD34EF" {
		t.Fatalf("ga4 %q %v", v, err)
	}
	if v, _ := normGA4(`<script async src="https://www.googletagmanager.com/gtag/js?id=G-ZZ99XX11"></script>`); v != "G-ZZ99XX11" {
		t.Fatalf("snippet %q", v)
	}
	for _, bad := range []string{"UA-1234-1", "G-1');alert(1);//"} {
		if _, err := normGA4(bad); err == nil && bad == "UA-1234-1" {
			t.Errorf("%q should fail", bad)
		}
	}
	if v, err := verifyToken(`<meta name="google-site-verification" content="abcDEF_123-xyz456" />`, "G"); err != nil || v != "abcDEF_123-xyz456" {
		t.Fatalf("meta %q %v", v, err)
	}
	if _, err := verifyToken(`a" onload="x`, "G"); err == nil {
		t.Error("quote injection should fail")
	}
}
