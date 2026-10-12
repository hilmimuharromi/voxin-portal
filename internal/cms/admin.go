package cms

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hilmimuharromi/voxin-portal/internal/auth"
	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

// ---------- regions ----------

func (s *Server) regions(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	regs, err := s.App.Store.Regions(r.Context(), site.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, r, "regions", "Daerah", "regions", regs)
}

func (s *Server) regionSave(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		back(w, r, "/regions", "Nama daerah wajib diisi.", true)
		return
	}
	slug := text.Slugify(firstNonEmpty(r.FormValue("slug"), name))
	_, err := s.App.Store.SaveRegion(r.Context(), site.ID, store.Region{ID: id, Slug: slug, Name: name, Province: strings.TrimSpace(r.FormValue("province")),
		Description: strings.TrimSpace(r.FormValue("description")), Partner: r.FormValue("partner") == "1"})
	if err != nil {
		back(w, r, "/regions", "Gagal menyimpan (slug mungkin sudah dipakai): "+err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	back(w, r, "/regions", "Daerah disimpan.", false)
}

func (s *Server) regionLogo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		back(w, r, "/regions", "Pilih file logo.", true)
		return
	}
	data, _ := io.ReadAll(io.LimitReader(f, 5<<20))
	f.Close()
	saved, err := s.App.Media.Save(data)
	if err != nil {
		back(w, r, "/regions", err.Error(), true)
		return
	}
	p := saved.Path
	if v := saved.Variants["400"]; v != "" {
		p = v
	}
	_ = s.App.Store.SetRegionLogo(r.Context(), pathID(r), p)
	s.App.Cache.Clear()
	back(w, r, "/regions", "Logo disimpan.", false)
}

func (s *Server) regionDelete(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	if err := s.App.Store.DeleteRegion(r.Context(), site.ID, pathID(r)); err != nil {
		back(w, r, "/regions", err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	back(w, r, "/regions", "Daerah dihapus. Artikelnya tetap ada, tanpa daerah.", false)
}

// ---------- static pages ----------

func (s *Server) pagesList(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	pages, err := s.App.Store.Pages(r.Context(), site.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, r, "pages", "Halaman wajib", "pages", pages)
}

func (s *Server) pageEdit(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	p, err := s.App.Store.PageBySlug(r.Context(), site.ID, r.PathValue("slug"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, "page_edit", p.Title, "pages", p)
}

func (s *Server) pageSave(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	p, err := s.App.Store.PageBySlug(r.Context(), site.ID, r.PathValue("slug"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p.Title = firstNonEmpty(strings.TrimSpace(r.FormValue("title")), p.Title)
	p.BodyMD = strings.TrimSpace(strings.ReplaceAll(r.FormValue("body"), "\r\n", "\n"))
	if err := s.App.Store.SavePage(r.Context(), site.ID, p); err != nil {
		back(w, r, "/pages/"+p.Slug, err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	back(w, r, "/pages/"+p.Slug, "Halaman disimpan.", false)
}

// ---------- team ----------

type TeamVM struct {
	Users []store.User
	Roles []string
}

func (s *Server) team(w http.ResponseWriter, r *http.Request) {
	users, err := s.App.Store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, r, "team", "Tim redaksi", "team", TeamVM{Users: users, Roles: assignable(userFrom(r))})
}

// assignable: admins can give any role; pemred everything except admin.
func assignable(u store.User) []string {
	if u.Role == "admin" {
		return auth.Roles
	}
	return auth.Roles[:len(auth.Roles)-1]
}

func roleAllowed(u store.User, role string) bool {
	for _, x := range assignable(u) {
		if x == role {
			return true
		}
	}
	return false
}

func (s *Server) teamCreate(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	role := r.FormValue("role")
	if name == "" || !strings.Contains(email, "@") || !roleAllowed(me, role) {
		back(w, r, "/team", "Isi nama, email yang valid, dan peran.", true)
		return
	}
	pw := strings.TrimSpace(r.FormValue("password"))
	generated := false
	if pw == "" {
		pw = auth.Token(9)
		generated = true
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		back(w, r, "/team", err.Error(), true)
		return
	}
	slug := text.Slugify(name)
	for i := 2; ; i++ {
		if _, err := s.App.Store.UserBySlug(r.Context(), slug); err != nil {
			break
		}
		slug = fmt.Sprintf("%s-%d", text.Slugify(name), i)
	}
	if _, err := s.App.Store.CreateUser(r.Context(), store.User{Email: email, Name: name, Slug: slug, PasswordHash: h, Role: role,
		Title: strings.TrimSpace(r.FormValue("title")), Region: strings.TrimSpace(r.FormValue("region"))}); err != nil {
		back(w, r, "/team", "Gagal menambah (email sudah terdaftar?): "+err.Error(), true)
		return
	}
	msg := "Akun " + name + " dibuat."
	if generated {
		msg += " Password sementara: " + pw + " (sampaikan langsung, lalu minta dia menggantinya)."
	}
	back(w, r, "/team", msg, false)
}

type UserVM struct {
	U      store.User
	Self   bool
	Manage bool
	Roles  []string
}

func (s *Server) userEdit(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	u, err := s.App.Store.UserByID(r.Context(), pathID(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if u.ID != me.ID && !auth.CanManage(me.Role) {
		http.Error(w, "Hanya pemred yang bisa mengubah profil orang lain.", http.StatusForbidden)
		return
	}
	s.render(w, r, "user_edit", u.Name, "team", UserVM{U: u, Self: u.ID == me.ID, Manage: auth.CanManage(me.Role), Roles: assignable(me)})
}

func (s *Server) userSave(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	u, err := s.App.Store.UserByID(r.Context(), pathID(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	self := fmt.Sprintf("/team/%d", u.ID)
	if u.ID != me.ID && !auth.CanManage(me.Role) {
		http.Error(w, "Tidak boleh.", http.StatusForbidden)
		return
	}
	u.Name = firstNonEmpty(strings.TrimSpace(r.FormValue("name")), u.Name)
	u.Title = strings.TrimSpace(r.FormValue("title"))
	u.Region = strings.TrimSpace(r.FormValue("region"))
	u.UKWLevel = r.FormValue("ukw_level")
	u.UKWNumber = strings.TrimSpace(r.FormValue("ukw_number"))
	u.Bio = strings.TrimSpace(r.FormValue("bio"))
	if auth.CanManage(me.Role) && u.ID != me.ID {
		if roleAllowed(me, r.FormValue("role")) {
			u.Role = r.FormValue("role")
		}
		u.Active = r.FormValue("active") == "1"
	}
	if err := s.App.Store.UpdateUserProfile(r.Context(), u); err != nil {
		back(w, r, self, err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	back(w, r, self, "Profil disimpan.", false)
}

func (s *Server) userPhoto(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id := pathID(r)
	self := fmt.Sprintf("/team/%d", id)
	if id != me.ID && !auth.CanManage(me.Role) {
		http.Error(w, "Tidak boleh.", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		back(w, r, self, "Pilih foto.", true)
		return
	}
	data, _ := io.ReadAll(io.LimitReader(f, 10<<20))
	f.Close()
	saved, err := s.App.Media.Save(data)
	if err != nil {
		back(w, r, self, err.Error(), true)
		return
	}
	p := saved.Path
	if v := saved.Variants["400"]; v != "" {
		p = v
	}
	_ = s.App.Store.SetUserPhoto(r.Context(), id, p)
	s.App.Cache.Clear()
	back(w, r, self, "Foto profil diganti.", false)
}

func (s *Server) userPassword(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id := pathID(r)
	self := fmt.Sprintf("/team/%d", id)
	u, err := s.App.Store.UserByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if id == me.ID {
		if !auth.CheckPassword(u.PasswordHash, r.FormValue("current")) {
			back(w, r, self, "Password lama salah.", true)
			return
		}
	} else if !auth.CanManage(me.Role) {
		http.Error(w, "Tidak boleh.", http.StatusForbidden)
		return
	}
	h, err := auth.HashPassword(r.FormValue("new"))
	if err != nil {
		back(w, r, self, err.Error(), true)
		return
	}
	_ = s.App.Store.SetUserPassword(r.Context(), id, h)
	if id == me.ID {
		// keep this device logged in with the new hash
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.sess.Issue(id, h, sessionTTL), Path: "/", MaxAge: int(sessionTTL.Seconds()),
			HttpOnly: true, Secure: s.App.Cfg.SecureCookie, SameSite: http.SameSiteLaxMode})
	}
	back(w, r, self, "Password diganti. Sesi di perangkat lain otomatis keluar.", false)
}

// ---------- settings ----------

type SettingsVM struct {
	S         store.Site
	EngineURL string
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	s.render(w, r, "settings", "Pengaturan", "settings", SettingsVM{S: site, EngineURL: s.App.Cfg.CMSURL + "/api/engine/articles"})
}

func (s *Server) settingsSave(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	f := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	set := site.Settings
	set.Tagline, set.About, set.Handle = f("tagline"), f("about"), f("handle")
	set.Instagram, set.TikTok, set.Threads = f("instagram"), f("tiktok"), f("threads")
	set.Company, set.Address, set.PressStatus, set.Email = f("company"), f("address"), f("press_status"), f("email")
	if err := s.App.Store.SaveSiteSettings(r.Context(), site.ID, firstNonEmpty(f("name"), site.Name), set); err != nil {
		back(w, r, "/settings", err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	back(w, r, "/settings", "Pengaturan disimpan.", false)
}

func (s *Server) settingsToken(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	set := site.Settings
	set.EngineToken = auth.Token(24)
	if err := s.App.Store.SaveSiteSettings(r.Context(), site.ID, site.Name, set); err != nil {
		back(w, r, "/settings", err.Error(), true)
		return
	}
	back(w, r, "/settings#engine", "Token baru dibuat. Salin ke Pengaturan engine (Token portal). Token lama tidak berlaku lagi.", false)
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
