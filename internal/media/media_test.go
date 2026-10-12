package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveMakesVariants(t *testing.T) {
	data, err := os.ReadFile("/tmp/mock/img1.jpg")
	if err != nil {
		t.Skip("contoh gambar tidak ada")
	}
	dir := t.TempDir()
	s, err := Store{Dir: dir}.Save(data)
	if err != nil {
		t.Fatal(err)
	}
	if s.Width == 0 {
		t.Fatal("ukuran tidak terbaca")
	}
	for w, p := range s.Variants {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("varian %s hilang: %v", w, err)
		}
	}
	if _, err := (Store{Dir: dir}).Save([]byte("bukan gambar")); err == nil {
		t.Fatal("bukan gambar harus ditolak")
	}
	t.Logf("%dx%d variants=%v", s.Width, s.Height, s.Variants)
}
