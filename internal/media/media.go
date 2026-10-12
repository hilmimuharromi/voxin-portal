// Package media stores uploaded images and makes smaller JPEG variants
// (400/800/1200 px wide) so phones on 4G don't download the original.
package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var Widths = []int{400, 800, 1200}

type Saved struct {
	Path     string
	Width    int
	Height   int
	Variants map[string]string
}

type Store struct{ Dir string } // data/media

// Save writes the original and its variants under media/YYYY/MM/.
func (s Store) Save(data []byte) (Saved, error) {
	ctype := http.DetectContentType(data)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[ctype]
	if ext == "" {
		return Saved{}, fmt.Errorf("file bukan gambar JPG/PNG/WebP (%s)", ctype)
	}
	now := time.Now()
	base := filepath.Join(now.Format("2006"), now.Format("01"), strconv.FormatInt(now.UnixNano(), 36))
	if err := os.MkdirAll(filepath.Join(s.Dir, filepath.Dir(base)), 0o755); err != nil {
		return Saved{}, err
	}
	out := Saved{Path: filepath.ToSlash(base + ext), Variants: map[string]string{}}
	if err := os.WriteFile(filepath.Join(s.Dir, out.Path), data, 0o644); err != nil {
		return Saved{}, err
	}
	if ext == ".webp" {
		// stdlib cannot decode WebP: keep the original only
		out.Width, out.Height = webpSize(data)
		return out, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return out, nil // keep the original; variants are an optimisation
	}
	b := img.Bounds()
	out.Width, out.Height = b.Dx(), b.Dy()
	for _, w := range Widths {
		if out.Width <= w {
			continue
		}
		h := out.Height * w / out.Width
		small := Resize(img, w, h)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, small, &jpeg.Options{Quality: 82}); err != nil {
			continue
		}
		p := filepath.ToSlash(fmt.Sprintf("%s-%d.jpg", base, w))
		if err := os.WriteFile(filepath.Join(s.Dir, p), buf.Bytes(), 0o644); err == nil {
			out.Variants[strconv.Itoa(w)] = p
		}
	}
	return out, nil
}

// Resize scales down with an area-average (box) filter.
func Resize(src image.Image, w, h int) *image.RGBA {
	sb := src.Bounds()
	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(image.Rect(0, 0, sb.Dx(), sb.Dy()))
		draw.Draw(rgba, rgba.Bounds(), src, sb.Min, draw.Src)
	}
	sw, sh := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, (y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, (x+1)*sw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n uint32
			for yy := y0; yy < y1; yy++ {
				off := yy*rgba.Stride + x0*4
				for xx := x0; xx < x1; xx++ {
					r += uint32(rgba.Pix[off])
					g += uint32(rgba.Pix[off+1])
					b += uint32(rgba.Pix[off+2])
					a += uint32(rgba.Pix[off+3])
					off += 4
					n++
				}
			}
			o := y*dst.Stride + x*4
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), uint8(a/n)
		}
	}
	return dst
}

func webpSize(b []byte) (int, int) {
	if len(b) < 30 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0
	}
	switch string(b[12:16]) {
	case "VP8 ":
		return int(b[26]) | int(b[27]&0x3f)<<8, int(b[28]) | int(b[29]&0x3f)<<8
	case "VP8L":
		bits := uint32(b[21]) | uint32(b[22])<<8 | uint32(b[23])<<16 | uint32(b[24])<<24
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1
	case "VP8X":
		return 1 + int(b[24]) | int(b[25])<<8 | int(b[26])<<16, 1 + int(b[27]) | int(b[28])<<8 | int(b[29])<<16
	}
	return 0, 0
}

var client = &http.Client{Timeout: 40 * time.Second}

// Download fetches an image URL (max 15 MB).
func Download(ctx context.Context, url string) ([]byte, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, errors.New("URL gambar tidak valid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "voxin-portal/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gambar HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 15<<20+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 15<<20 {
		return nil, errors.New("gambar lebih dari 15 MB")
	}
	return b, nil
}
