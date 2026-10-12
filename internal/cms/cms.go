// Package cms is the newsroom back office (cms.voxin.id): login per person,
// article editor, regions, static pages, team, analytics and settings.
package cms

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/auth"
	"github.com/hilmimuharromi/voxin-portal/internal/core"
	"github.com/hilmimuharromi/voxin-portal/internal/site"
	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

//go:embed templates/*.html
var tplFS embed.FS

//go:embed static
var staticFS embed.FS

const cookieName = "vxp_session"
const sessionTTL = 30 * 24 * time.Hour

type Server struct {
	App   *core.App
	sess  auth.Sessions
	pages map[string]*template.Template
}

var pageFiles = []string{"login", "dashboard", "articles", "editor", "regions", "pages", "page_edit", "team", "user_edit", "analytics", "settings"}

func New(app *core.App) (*Server, error) {
	s := &Server{App: app, sess: auth.Sessions{Key: []byte(app.Cfg.SecretKey)}, pages: map[string]*template.Template{}}
	base, err := template.New("base").Funcs(s.funcs()).ParseFS(tplFS, "templates/layout.html")
	if err != nil {
		return nil, err
	}
	for _, p := range pageFiles {
		t, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseFS(tplFS, "templates/"+p+".html"); err != nil {
			return nil, fmt.Errorf("template %s: %w", p, err)
		}
		s.pages[p] = t
	}
	return s, nil
}

func (s *Server) funcs() template.FuncMap {
	return template.FuncMap{
		"md": func(m string) template.HTML { return template.HTML(text.Markdown(m)) },
		"waktu": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return text.Waktu(t.In(s.App.Loc))
		},
		"lalu": func(t time.Time) string { return text.Lalu(t.In(s.App.Loc), time.Now().In(s.App.Loc)) },
		"laluP": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return text.Lalu(t.In(s.App.Loc), time.Now().In(s.App.Loc))
		},
		"statusLabel": func(st string) string { return store.StatusLabels[st] },
		"typeLabel":   func(t string) string { return store.TypeLabels[t] },
		"roleLabel":   func(r string) string { return auth.RoleLabels[r] },
		"canPublish":  auth.CanPublish,
		"canManage":   auth.CanManage,
		"publicURL":   func(p string) string { return s.App.Cfg.BaseURL + p },
		"localInput": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.In(s.App.Loc).Format("2006-01-02T15:04")
		},
		"num":       func(n int) string { return fmtNum(n) },
		"hasPrefix": strings.HasPrefix,
		"add1":      func(i int) int { return i + 1 },
		"list":      func(v ...string) []string { return v },
		"contains":  strings.Contains,
		"words":     func(t string) int { return len(strings.Fields(t)) },
		"jsStr":     func(v string) template.JS { b, _ := json.Marshal(v); return template.JS(b) },
	}
}

// fmtNum: 12345 -> "12.345".
func fmtNum(n int) string {
	str := strconv.Itoa(n)
	neg := strings.HasPrefix(str, "-")
	str = strings.TrimPrefix(str, "-")
	var out []byte
	for i := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, str[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// EnsureAdmin creates the first account from env when there are no users.
func (s *Server) EnsureAdmin(ctx context.Context) error {
	n, err := s.App.Store.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	if s.App.Cfg.AdminEmail == "" || s.App.Cfg.AdminPassword == "" {
		return errors.New("belum ada akun: isi PORTAL_ADMIN_EMAIL dan PORTAL_ADMIN_PASSWORD di .env untuk akun pertama")
	}
	h, err := auth.HashPassword(s.App.Cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = s.App.Store.CreateUser(ctx, store.User{Email: s.App.Cfg.AdminEmail, Name: "Pemimpin Redaksi", Slug: "pemimpin-redaksi", PasswordHash: h, Role: "admin", Title: "Pemimpin Redaksi"})
	if err == nil {
		s.App.Log.Info("akun pertama dibuat", "email", s.App.Cfg.AdminEmail)
	}
	return err
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(http.FileServerFS(site.StaticFS()), 86400)))
	cstatic, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /cms-static/", http.StripPrefix("/cms-static/", cacheFor(http.FileServerFS(cstatic), 3600)))
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("POST /api/engine/articles", s.engineArticle)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	a := http.NewServeMux()
	a.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(s.App.Media.Dir))))
	a.HandleFunc("POST /logout", s.logout)
	a.HandleFunc("GET /{$}", s.dashboard)
	a.HandleFunc("GET /articles", s.articles)
	a.HandleFunc("POST /articles/new", s.articleNew)
	a.HandleFunc("GET /articles/{id}", s.editor)
	a.HandleFunc("POST /articles/{id}", s.editorSave)
	a.HandleFunc("POST /articles/{id}/preview", s.editorPreview)
	a.HandleFunc("POST /articles/{id}/image", s.editorImage)
	a.HandleFunc("GET /regions", s.requireRole("pemred", s.regions))
	a.HandleFunc("POST /regions", s.requireRole("pemred", s.regionSave))
	a.HandleFunc("POST /regions/{id}/logo", s.requireRole("pemred", s.regionLogo))
	a.HandleFunc("POST /regions/{id}/delete", s.requireRole("pemred", s.regionDelete))
	a.HandleFunc("GET /pages", s.requireRole("pemred", s.pagesList))
	a.HandleFunc("GET /pages/{slug}", s.requireRole("pemred", s.pageEdit))
	a.HandleFunc("POST /pages/{slug}", s.requireRole("pemred", s.pageSave))
	a.HandleFunc("GET /team", s.requireRole("pemred", s.team))
	a.HandleFunc("POST /team", s.requireRole("pemred", s.teamCreate))
	a.HandleFunc("GET /team/{id}", s.userEdit)
	a.HandleFunc("POST /team/{id}", s.userSave)
	a.HandleFunc("POST /team/{id}/photo", s.userPhoto)
	a.HandleFunc("POST /team/{id}/password", s.userPassword)
	a.HandleFunc("GET /analytics", s.requireRole("editor", s.analytics))
	a.HandleFunc("GET /settings", s.requireRole("pemred", s.settings))
	a.HandleFunc("POST /settings", s.requireRole("pemred", s.settingsSave))
	a.HandleFunc("POST /settings/token", s.requireRole("pemred", s.settingsToken))
	mux.Handle("/", s.requireLogin(a))
	return s.sameOrigin(securityHeaders(mux))
}

func cacheFor(h http.Handler, sec int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", sec))
		h.ServeHTTP(w, r)
	})
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		h.ServeHTTP(w, r)
	})
}

// sameOrigin blocks cross-site form posts (CSRF). The engine API uses a token instead.
func (s *Server) sameOrigin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && !strings.HasPrefix(r.URL.Path, "/api/") {
			src := r.Header.Get("Origin")
			if src == "" {
				src = r.Header.Get("Referer")
			}
			u, err := url.Parse(src)
			if src == "" || err != nil || u.Host != r.Host {
				http.Error(w, "Permintaan ditolak (asal tidak cocok).", http.StatusForbidden)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

func userFrom(r *http.Request) store.User {
	u, _ := r.Context().Value(ctxKey{}).(store.User)
	return u
}

func (s *Server) requireLogin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err == nil {
			if uid, exp, sig, ok := s.sess.Parse(c.Value); ok {
				if u, err := s.App.Store.UserByID(r.Context(), uid); err == nil && u.Active && s.sess.Verify(uid, exp, sig, u.PasswordHash) {
					h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
					return
				}
			}
		}
		if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/media/") {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		http.Error(w, "Sesi habis, silakan masuk lagi.", http.StatusUnauthorized)
	})
}

func (s *Server) requireRole(min string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r)
		ok := map[string]func(string) bool{"editor": auth.CanPublish, "pemred": auth.CanManage}[min]
		if ok == nil || !ok(u.Role) {
			http.Error(w, "Halaman ini hanya untuk "+auth.RoleLabels[min]+" ke atas.", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

// ---------- rendering ----------

type Page struct {
	Title    string
	Nav      string
	User     store.User
	Site     store.Site
	Flash    string
	FlashErr bool
	BaseURL  string
	Review   int
	Body     any
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name, title, nav string, body any) {
	site, _ := s.App.Site(r.Context())
	p := Page{Title: title, Nav: nav, User: userFrom(r), Site: site, BaseURL: s.App.Cfg.BaseURL, Body: body}
	if counts, err := s.App.Store.StatusCounts(r.Context(), site.ID); err == nil {
		p.Review = counts["review"]
	}
	if c, err := r.Cookie("vxp_flash"); err == nil {
		if v, err := url.QueryUnescape(c.Value); err == nil {
			p.FlashErr = strings.HasPrefix(v, "!")
			p.Flash = strings.TrimPrefix(v, "!")
		}
		http.SetCookie(w, &http.Cookie{Name: "vxp_flash", Value: "", Path: "/", MaxAge: -1})
	}
	var buf bytes.Buffer
	if err := s.pages[name].ExecuteTemplate(&buf, "layout", p); err != nil {
		s.App.Log.Error("render cms", "page", name, "err", err)
		http.Error(w, "Gagal menampilkan halaman: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}

func flash(w http.ResponseWriter, msg string, isErr bool) {
	if isErr {
		msg = "!" + msg
	}
	http.SetCookie(w, &http.Cookie{Name: "vxp_flash", Value: url.QueryEscape(msg), Path: "/", MaxAge: 60, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func back(w http.ResponseWriter, r *http.Request, to, msg string, isErr bool) {
	if msg != "" {
		flash(w, msg, isErr)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// ---------- login ----------

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	site, _ := s.App.Site(r.Context())
	err := s.pages["login"].ExecuteTemplate(&buf, "login", map[string]any{"Next": r.URL.Query().Get("next"), "Err": r.URL.Query().Get("err") != "", "Site": site})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	next := r.FormValue("next")
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/"
	}
	u, err := s.App.Store.UserByEmail(r.Context(), r.FormValue("email"))
	if err != nil || !u.Active || !auth.CheckPassword(u.PasswordHash, r.FormValue("password")) {
		time.Sleep(600 * time.Millisecond)
		http.Redirect(w, r, "/login?err=1&next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.sess.Issue(u.ID, u.PasswordHash, sessionTTL), Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: s.App.Cfg.SecureCookie, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
