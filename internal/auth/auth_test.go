package auth

import (
	"testing"
	"time"
)

func TestPasswordAndSession(t *testing.T) {
	h, err := HashPassword("rahasia-123")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "rahasia-123") || CheckPassword(h, "salah-123") {
		t.Fatal("cek password salah")
	}
	if _, err := HashPassword("pendek"); err == nil {
		t.Fatal("password pendek harus ditolak")
	}
	s := Sessions{Key: []byte("0123456789abcdef")}
	tok := s.Issue(7, h, time.Hour)
	uid, exp, sig, ok := s.Parse(tok)
	if !ok || uid != 7 || !s.Verify(uid, exp, sig, h) {
		t.Fatal("sesi valid ditolak")
	}
	if s.Verify(uid, exp, sig, h+"x") {
		t.Fatal("ganti password harus membatalkan sesi")
	}
	if _, _, _, ok := s.Parse("7.1.abc"); ok {
		t.Fatal("sesi kedaluwarsa diterima")
	}
}
