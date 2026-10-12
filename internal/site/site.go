// Package site serves the public news site (voxin.id): server-rendered HTML,
// cached for a minute, with SEO feeds and cookie-less view counting.
package site

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/core"
	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

//go:embed templates/*.html
var tplFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	App *core.App
	tpl *template.Template
}

func New(app *core.App) (*Server, error) {
	s := &Server{App: app}
	t, err := template.New("").Funcs(s.funcs()).ParseFS(tplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s.tpl = t
	return s, nil
}

func (s *Server) funcs() template.FuncMap {
	return template.FuncMap{
		"md":      func(m string) template.HTML { return template.HTML(text.Markdown(m)) }, // escaped inside Markdown
		"tanggal": func(t time.Time) string { return text.Tanggal(t.In(s.App.Loc)) },
		"waktu": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return text.Waktu(t.In(s.App.Loc))
		},
		"lalu": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return text.Lalu(t.In(s.App.Loc), time.Now().In(s.App.Loc))
		},
		"img":      imgURL,
		"srcset":   srcset,
		"hl":       func(title, hl string) template.HTML { return highlight(title, hl) },
		"excerpt":  func(s string, n int) string { return text.Excerpt(text.Plain(s), n) },
		"lines":    text.Lines,
		"add":      func(a, b int) int { return a + b },
		"minutes":  text.ReadMinutes,
		"abs":      func(p string) string { return s.App.Cfg.BaseURL + p },
		"upper":    strings.ToUpper,
		"pathEsc":  url.PathEscape,
		"queryEsc": url.QueryEscape,
	}
}

func highlight(title, hl string) template.HTML {
	pre, mid, post := text.SplitHighlight(title, hl)
	e := template.HTMLEscapeString
	if mid == "" {
		return template.HTML(e(title))
	}
	return template.HTML(e(pre) + "<mark>" + e(mid) + "</mark>" + e(post))
}

type variants map[string]string

func parseVariants(raw string) variants {
	v := variants{}
	_ = json.Unmarshal([]byte(raw), &v)
	return v
}

// imgURL picks the variant closest to the wanted width.
func imgURL(path, rawVariants string, width int) string {
	if path == "" {
		return ""
	}
	v := parseVariants(rawVariants)
	best := ""
	for _, w := range []int{400, 800, 1200} {
		if p := v[strconv.Itoa(w)]; p != "" && w >= width {
			best = p
			break
		}
	}
	if best == "" {
		best = path
	}
	return "/media/" + best
}

func srcset(path, rawVariants string) string {
	v := parseVariants(rawVariants)
	var parts []string
	for _, w := range []int{400, 800, 1200} {
		if p := v[strconv.Itoa(w)]; p != "" {
			parts = append(parts, fmt.Sprintf("/media/%s %dw", p, w))
		}
	}
	return strings.Join(parts, ", ")
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	assets := http.StripPrefix("/static/", longCache(http.FileServerFS(static)))
	uploads := http.StripPrefix("/media/", longCache(noDir(http.FileServer(http.Dir(s.App.Media.Dir)))))
	mux.HandleFunc("POST /v", s.view)
	mux.HandleFunc("GET /robots.txt", s.cached(s.robots))
	mux.HandleFunc("GET /rss.xml", s.cached(s.rss))
	mux.HandleFunc("GET /sitemap.xml", s.cached(s.sitemap))
	mux.HandleFunc("GET /sitemap-news.xml", s.cached(s.sitemapNews))
	mux.HandleFunc("GET /{$}", s.cached(s.home))
	mux.HandleFunc("GET /cari", s.cached(s.search))
	mux.HandleFunc("GET /daerah", s.cached(s.regions))
	mux.HandleFunc("GET /daerah/{slug}", s.cached(s.region))
	mux.HandleFunc("GET /penulis/{slug}", s.cached(s.author))
	mux.HandleFunc("GET /tag/{slug}", s.cached(s.tag))
	mux.HandleFunc("GET /{first}", s.cached(s.pageOrCategory))
	mux.HandleFunc("GET /{cat}/{slug}", s.cached(s.article))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { s.notFound(w, r) })
	// files are dispatched before the router: /{cat}/{slug} would also match them
	return securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/static/") && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			assets.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/media/") && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			uploads.ServeHTTP(w, r)
		default:
			mux.ServeHTTP(w, r)
		}
	}))
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		h.ServeHTTP(w, r)
	})
}

func longCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}

func noDir(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// response is what a page handler produces; cached() stores it.
type response struct {
	status int
	ctype  string
	body   []byte
	loc    string // redirect target
}

type pageFn func(r *http.Request) response

// cached serves from the page cache, or renders and stores 200 responses.
func (s *Server) cached(fn pageFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.RequestURI()
		if body, ctype, ok := s.App.Cache.Get(key); ok {
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Cache-Control", "public, max-age=60")
			w.Header().Set("X-Cache", "HIT")
			w.Write(body)
			return
		}
		res := fn(r)
		if res.loc != "" {
			http.Redirect(w, r, res.loc, res.status)
			return
		}
		if res.status == 0 {
			res.status = http.StatusOK
		}
		if res.status == http.StatusOK {
			s.App.Cache.Set(key, res.body, res.ctype)
			w.Header().Set("Cache-Control", "public, max-age=60")
		}
		w.Header().Set("Content-Type", res.ctype)
		w.WriteHeader(res.status)
		w.Write(res.body)
	}
}

// ---------- shared layout data ----------

type Meta struct {
	Title       string
	Description string
	Canonical   string
	Image       string
	Type        string // website | article
	NoIndex     bool
	JSONLD      any
}

type Layout struct {
	Site       store.Site
	Categories []store.Category
	Today      string
	Ticker     []store.Article
	Nav        string // active nav slug
	Meta       Meta
	BaseURL    string
	Year       int
	Pages      []store.Page
	Body       any
}

func (s *Server) layout(ctx context.Context, nav string, meta Meta, body any) (Layout, error) {
	site, err := s.App.Site(ctx)
	if err != nil {
		return Layout{}, err
	}
	cats, _ := s.App.Store.Categories(ctx, site.ID)
	var navCats []store.Category
	for _, c := range cats {
		if c.InNav {
			navCats = append(navCats, c)
		}
	}
	ticker, _ := s.App.Store.Popular(ctx, site.ID, 1, 3)
	if len(ticker) == 0 {
		ticker, _, _ = s.App.Store.Articles(ctx, store.Query{SiteID: site.ID, Limit: 3})
	}
	pages, _ := s.App.Store.Pages(ctx, site.ID)
	now := time.Now().In(s.App.Loc)
	if meta.Canonical == "" {
		meta.Canonical = s.App.Cfg.BaseURL + "/"
	}
	if meta.Type == "" {
		meta.Type = "website"
	}
	if meta.Description == "" {
		meta.Description = site.Settings.About
	}
	return Layout{Site: site, Categories: navCats, Today: text.Tanggal(now), Ticker: ticker, Nav: nav, Meta: meta,
		BaseURL: s.App.Cfg.BaseURL, Year: now.Year(), Pages: pages, Body: body}, nil
}

func (s *Server) render(ctx context.Context, name, nav string, meta Meta, body any) response {
	l, err := s.layout(ctx, nav, meta, body)
	if err != nil {
		return s.fail(err)
	}
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, l); err != nil {
		return s.fail(err)
	}
	return response{status: http.StatusOK, ctype: "text/html; charset=utf-8", body: buf.Bytes()}
}

func (s *Server) fail(err error) response {
	s.App.Log.Error("halaman publik", "err", err)
	return response{status: http.StatusInternalServerError, ctype: "text/plain; charset=utf-8", body: []byte("Maaf, ada gangguan. Coba muat ulang sebentar lagi.")}
}

func (s *Server) notFoundResp(ctx context.Context) response {
	res := s.render(ctx, "notfound", "", Meta{Title: "Halaman tidak ditemukan", NoIndex: true}, nil)
	if res.status == http.StatusOK {
		res.status = http.StatusNotFound
	}
	return res
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	res := s.notFoundResp(r.Context())
	w.Header().Set("Content-Type", res.ctype)
	w.WriteHeader(res.status)
	w.Write(res.body)
}

// ---------- view counting ----------

// view is called by a tiny script on article pages (bots that don't run JS are skipped).
func (s *Server) view(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
	if err := r.ParseForm(); err != nil {
		return
	}
	id, _ := strconv.ParseInt(r.FormValue("a"), 10, 64)
	ua := r.UserAgent()
	if ua == "" || isBot(ua) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	site, err := s.App.Site(ctx)
	if err != nil {
		return
	}
	now := time.Now().In(s.App.Loc)
	h := sha256.Sum256([]byte(now.Format("2006-01-02") + "|" + clientIP(r) + "|" + ua + "|" + s.App.Cfg.SecretKey))
	if err := s.App.Store.RecordView(ctx, site.ID, id, now, Source(r.FormValue("ref"), s.App.Cfg.BaseURL), hex.EncodeToString(h[:12])); err != nil {
		s.App.Log.Warn("catat pageview", "err", err)
	}
}

func isBot(ua string) bool {
	ua = strings.ToLower(ua)
	for _, b := range []string{"bot", "crawl", "spider", "slurp", "facebookexternalhit", "preview", "headless", "lighthouse"} {
		if strings.Contains(ua, b) {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Source classifies a referrer: search | sosmed | langsung | lainnya.
func Source(ref, base string) string {
	if ref == "" {
		return "langsung"
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return "langsung"
	}
	host := strings.ToLower(u.Host)
	if bu, err := url.Parse(base); err == nil && bu.Host != "" && strings.EqualFold(bu.Host, host) {
		return "internal"
	}
	for _, x := range []string{"google.", "bing.", "yahoo.", "duckduckgo.", "yandex.", "ecosia."} {
		if strings.Contains(host, x) {
			return "search"
		}
	}
	for _, x := range []string{"instagram.", "facebook.", "fb.", "t.co", "twitter.", "x.com", "tiktok.", "threads.", "whatsapp.", "wa.me", "linkedin.", "youtube.", "t.me", "telegram."} {
		if strings.Contains(host, x) {
			return "sosmed"
		}
	}
	return "lainnya"
}

var errGone = errors.New("diturunkan")

// StaticFS exposes the public assets (fonts, css) so the CMS can reuse them.
func StaticFS() fs.FS { s, _ := fs.Sub(staticFS, "static"); return s }
