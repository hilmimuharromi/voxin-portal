package cms

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/auth"
	"github.com/hilmimuharromi/voxin-portal/internal/core"
	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

// ---------- dashboard ----------

type DashVM struct {
	Counts   map[string]int
	Review   []store.Article
	Mine     []store.Article
	Recent   []store.Article
	Top      []store.Article
	Views7   int
	Uniq7    int
	Today    int
	PubToday int
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)
	site, err := s.App.Site(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	st := s.App.Store
	vm := DashVM{}
	vm.Counts, _ = st.StatusCounts(ctx, site.ID)
	vm.Review, _, _ = st.Articles(ctx, store.Query{SiteID: site.ID, Statuses: []string{"review"}, OrderBy: "updated", Limit: 10})
	vm.Mine, _, _ = st.Articles(ctx, store.Query{SiteID: site.ID, Statuses: []string{"draft", "review", "scheduled"}, AuthorID: u.ID, OrderBy: "updated", Limit: 6})
	vm.Recent, _, _ = st.Articles(ctx, store.Query{SiteID: site.ID, Limit: 8})
	if auth.CanPublish(u.Role) {
		vm.Top, _ = st.TopArticles(ctx, site.ID, 7, 5)
		days, _ := st.DailyStats(ctx, site.ID, 7)
		for i, d := range days {
			vm.Views7 += d.Views
			vm.Uniq7 += d.Uniques
			if i == len(days)-1 {
				vm.Today = d.Views
			}
		}
	}
	start := time.Now().In(s.App.Loc)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, s.App.Loc)
	_, vm.PubToday, _ = st.Articles(ctx, store.Query{SiteID: site.ID, Since: &start, Limit: 1})
	s.render(w, r, "dashboard", "Ruang redaksi", "home", vm)
}

// ---------- list ----------

type ListVM struct {
	Items  []store.Article
	Total  int
	Status string
	Type   string
	Q      string
	Mine   bool
	Counts map[string]int
	Views  map[int64]int
	Page   int
	Pages  int
	Prev   string
	Next   string
}

func (s *Server) articles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)
	site, _ := s.App.Site(ctx)
	q := r.URL.Query()
	vm := ListVM{Status: q.Get("status"), Type: q.Get("type"), Q: strings.TrimSpace(q.Get("q")), Mine: q.Get("mine") == "1"}
	vm.Page, _ = strconv.Atoi(q.Get("hal"))
	if vm.Page < 1 {
		vm.Page = 1
	}
	query := store.Query{SiteID: site.ID, Type: vm.Type, Search: vm.Q, OrderBy: "updated", Limit: 30, Offset: (vm.Page - 1) * 30}
	if vm.Status != "" {
		query.Statuses = []string{vm.Status}
	} else {
		query.Statuses = []string{"draft", "review", "scheduled", "published", "unpublished"}
	}
	if vm.Mine || !auth.CanPublish(u.Role) {
		query.AuthorID = u.ID
		vm.Mine = true
	}
	items, total, err := s.App.Store.Articles(ctx, query)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	vm.Items, vm.Total = items, total
	vm.Pages = (total + 29) / 30
	link := func(p int) string {
		v := r.URL.Query()
		v.Set("hal", strconv.Itoa(p))
		return "/articles?" + v.Encode()
	}
	if vm.Page > 1 {
		vm.Prev = link(vm.Page - 1)
	}
	if vm.Page < vm.Pages {
		vm.Next = link(vm.Page + 1)
	}
	vm.Counts, _ = s.App.Store.StatusCounts(ctx, site.ID)
	var ids []int64
	for _, a := range items {
		ids = append(ids, a.ID)
	}
	vm.Views, _ = s.App.Store.ArticleViews(ctx, site.ID, ids, 0)
	s.render(w, r, "articles", "Artikel", "articles", vm)
}

func (s *Server) articleNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)
	site, _ := s.App.Site(ctx)
	typ := r.FormValue("type")
	if store.TypeLabels[typ] == "" {
		typ = "berita"
	}
	id, err := s.App.Store.CreateArticle(ctx, store.Article{SiteID: site.ID, Type: typ, Status: "draft", AuthorID: u.ID, Dateline: "JAKARTA"})
	if err != nil {
		back(w, r, "/articles", err.Error(), true)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/articles/%d", id), http.StatusSeeOther)
}

// ---------- editor ----------

type EditorVM struct {
	A          store.Article
	Categories []store.Category
	Regions    []store.Region
	Authors    []store.User
	Problems   []string
	Versions   []store.Version
	CanEdit    bool
	CanPublish bool
	Types      []string
	TagsText   string
	Views      int
	PublicURL  string
}

func canEdit(u store.User, a store.Article) bool {
	if auth.CanPublish(u.Role) {
		return true
	}
	return a.AuthorID == u.ID && (a.Status == "draft" || a.Status == "review")
}

func (s *Server) loadArticle(w http.ResponseWriter, r *http.Request) (store.Article, store.Site, bool) {
	site, _ := s.App.Site(r.Context())
	a, err := s.App.Store.GetArticle(r.Context(), pathID(r))
	if err != nil || a.SiteID != site.ID {
		http.NotFound(w, r)
		return a, site, false
	}
	u := userFrom(r)
	if !auth.CanPublish(u.Role) && a.AuthorID != u.ID {
		http.Error(w, "Artikel ini bukan milikmu.", http.StatusForbidden)
		return a, site, false
	}
	return a, site, true
}

func (s *Server) editor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, site, ok := s.loadArticle(w, r)
	if !ok {
		return
	}
	u := userFrom(r)
	vm := EditorVM{A: a, CanEdit: canEdit(u, a), CanPublish: auth.CanPublish(u.Role), Types: store.ArticleTypes, Problems: core.PublishProblems(a)}
	vm.Categories, _ = s.App.Store.Categories(ctx, site.ID)
	vm.Regions, _ = s.App.Store.Regions(ctx, site.ID)
	users, _ := s.App.Store.ListUsers(ctx)
	for _, x := range users {
		if x.Active || x.ID == a.AuthorID {
			vm.Authors = append(vm.Authors, x)
		}
	}
	vm.Versions, _ = s.App.Store.Versions(ctx, a.ID, 10)
	var tags []string
	for _, t := range a.Tags {
		tags = append(tags, t.Name)
	}
	vm.TagsText = strings.Join(tags, ", ")
	if v, _ := s.App.Store.ArticleViews(ctx, site.ID, []int64{a.ID}, 0); v != nil {
		vm.Views = v[a.ID]
	}
	if a.Status == "published" || a.Status == "unpublished" {
		vm.PublicURL = s.App.Cfg.BaseURL + a.Path()
	}
	title := a.Title
	if title == "" {
		title = "Artikel baru"
	}
	s.render(w, r, "editor", title, "articles", vm)
}

// fromForm copies the editable fields from the form onto a.
func (s *Server) fromForm(r *http.Request, a *store.Article, u store.User) {
	f := r.Form
	if t := f.Get("type"); store.TypeLabels[t] != "" {
		a.Type = t
	}
	a.Title = strings.TrimSpace(f.Get("title"))
	a.Highlight = strings.TrimSpace(f.Get("highlight"))
	a.Summary = strings.TrimSpace(f.Get("summary"))
	a.BodyMD = strings.TrimSpace(strings.ReplaceAll(f.Get("body"), "\r\n", "\n"))
	a.Intinya = strings.TrimSpace(strings.ReplaceAll(f.Get("intinya"), "\r\n", "\n"))
	a.Dateline = strings.ToUpper(strings.TrimSpace(f.Get("dateline")))
	a.CategoryID, _ = strconv.ParseInt(f.Get("category_id"), 10, 64)
	a.RegionID, _ = strconv.ParseInt(f.Get("region_id"), 10, 64)
	if auth.CanPublish(u.Role) {
		if id, err := strconv.ParseInt(f.Get("author_id"), 10, 64); err == nil {
			a.AuthorID = id
		}
		a.EditorNote = strings.TrimSpace(f.Get("editor_note"))
	}
	a.SourceName = strings.TrimSpace(f.Get("source_name"))
	a.SourceURL = strings.TrimSpace(f.Get("source_url"))
	if a.SourceURL != "" && !strings.HasPrefix(a.SourceURL, "http") {
		a.SourceURL = ""
	}
	a.AIAssisted = f.Get("ai_assisted") == "1"
	if a.Highlight != "" && !strings.Contains(strings.ToLower(a.Title), strings.ToLower(a.Highlight)) {
		a.Highlight = ""
	}
}

func parseTags(s string) []store.Tag {
	var out []store.Tag
	seen := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		slug := text.Slugify(t)
		if t == "" || slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, store.Tag{Name: t, Slug: slug})
		if len(out) == 10 {
			break
		}
	}
	return out
}

func (s *Server) editorSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, site, ok := s.loadArticle(w, r)
	if !ok {
		return
	}
	u := userFrom(r)
	self := fmt.Sprintf("/articles/%d", a.ID)
	if err := r.ParseForm(); err != nil {
		back(w, r, self, err.Error(), true)
		return
	}
	action := r.FormValue("action")
	editable := canEdit(u, a)
	if !editable && action != "unpublish" && action != "republish" {
		back(w, r, self, "Artikel ini sudah tidak bisa kamu ubah. Minta editor.", true)
		return
	}
	oldPath := a.Path()
	wasLive := a.Status == "published"
	if editable && action != "unpublish" {
		s.fromForm(r, &a, u)
		// slug: keep stable once live unless the editor changes it on purpose
		want := strings.TrimSpace(r.FormValue("slug"))
		if want == "" {
			want = a.Slug
		}
		if want == "" {
			want = a.Title
		}
		slug, err := s.App.UniqueSlug(ctx, site.ID, want, a.ID)
		if err != nil {
			back(w, r, self, err.Error(), true)
			return
		}
		a.Slug = slug
		if a.Title == "" {
			a.Slug = ""
		}
		if err := s.App.Store.SaveArticle(ctx, a, u.ID); err != nil {
			back(w, r, self, "Gagal menyimpan: "+err.Error(), true)
			return
		}
		_ = s.App.Store.SetTags(ctx, a.ID, parseTags(r.FormValue("tags")))
		_ = s.App.Store.SetSocial(ctx, a.ID, map[string]string{"instagram": r.FormValue("social_instagram"), "tiktok": r.FormValue("social_tiktok"), "threads": r.FormValue("social_threads")})
		if a.ImageID > 0 {
			_ = s.App.Store.UpdateMediaText(ctx, a.ImageID, strings.TrimSpace(r.FormValue("image_credit")), strings.TrimSpace(r.FormValue("image_caption")))
		}
		a, _ = s.App.Store.GetArticle(ctx, a.ID)
		if wasLive && a.Path() != oldPath {
			_ = s.App.Store.AddRedirect(ctx, site.ID, oldPath, a.ID)
		}
	}
	msg := "Tersimpan."
	switch action {
	case "review":
		_ = s.App.Store.SetStatus(ctx, a.ID, "review", "")
		msg = "Dikirim ke editor untuk ditinjau."
	case "draft":
		_ = s.App.Store.SetStatus(ctx, a.ID, "draft", "")
		msg = "Dikembalikan ke draft."
	case "publish", "schedule", "unpublish", "republish":
		if !auth.CanPublish(u.Role) {
			back(w, r, self, "Hanya editor atau pemred yang bisa menerbitkan.", true)
			return
		}
		if action != "unpublish" && a.AuthorID == 0 && a.Type == "kurasi" {
			// curated pieces (e.g. from the engine) are credited to the reviewing editor
			a.AuthorID = u.ID
			_ = s.App.Store.SaveArticle(ctx, a, u.ID)
			a, _ = s.App.Store.GetArticle(ctx, a.ID)
		}
		if action != "unpublish" {
			if p := core.PublishProblems(a); len(p) > 0 {
				back(w, r, self, "Belum bisa terbit: "+strings.Join(p, "; ")+".", true)
				return
			}
		}
		switch action {
		case "publish", "republish":
			if err := s.App.Store.Publish(ctx, a.ID, u.ID); err != nil {
				back(w, r, self, err.Error(), true)
				return
			}
			msg = "Terbit. Link: " + s.App.Cfg.BaseURL + a.Path()
		case "schedule":
			at, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("scheduled_at"), s.App.Loc)
			if err != nil || at.Before(time.Now().Add(time.Minute)) {
				back(w, r, self, "Isi tanggal dan jam terbit yang akan datang.", true)
				return
			}
			_ = s.App.Store.Schedule(ctx, a.ID, u.ID, at)
			msg = "Dijadwalkan terbit " + text.Waktu(at)
		case "unpublish":
			note := strings.TrimSpace(r.FormValue("unpublish_note"))
			if note == "" {
				back(w, r, self, "Tulis alasan penurunan; alasan ini tampil di halaman artikel.", true)
				return
			}
			_ = s.App.Store.SetStatus(ctx, a.ID, "unpublished", note)
			msg = "Artikel diturunkan. Halaman publik menampilkan alasannya."
		}
	case "delete":
		if a.Status != "draft" && a.Status != "review" {
			back(w, r, self, "Hanya draft yang bisa dihapus. Artikel terbit diturunkan, bukan dihapus.", true)
			return
		}
		_ = s.App.Store.DeleteArticle(ctx, a.ID)
		s.App.Cache.Clear()
		back(w, r, "/articles", "Draft dihapus.", false)
		return
	}
	s.App.Cache.Clear()
	back(w, r, self, msg, false)
}

func (s *Server) editorPreview(w http.ResponseWriter, r *http.Request) {
	a, site, ok := s.loadArticle(w, r)
	if !ok {
		return
	}
	body := r.FormValue("body")
	if dl := strings.ToUpper(strings.TrimSpace(r.FormValue("dateline"))); dl != "" && !strings.HasPrefix(strings.TrimSpace(body), "#") {
		body = "**" + dl + ", " + strings.ToUpper(site.Name) + "** – " + strings.TrimLeft(body, " \n")
	}
	_ = a
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `%s<div class="tiny muted">%d kata · %d menit baca</div>`, text.Markdown(body), len(strings.Fields(r.FormValue("body"))), text.ReadMinutes(r.FormValue("body")))
}

func (s *Server) editorImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, site, ok := s.loadArticle(w, r)
	if !ok {
		return
	}
	u := userFrom(r)
	self := fmt.Sprintf("/articles/%d", a.ID)
	if !canEdit(u, a) {
		back(w, r, self, "Artikel ini sudah tidak bisa kamu ubah.", true)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		back(w, r, self, "File terlalu besar (maks 20 MB).", true)
		return
	}
	credit := strings.TrimSpace(r.FormValue("credit"))
	if credit == "" {
		back(w, r, self, "Kredit foto wajib diisi.", true)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		back(w, r, self, "Pilih file foto dulu.", true)
		return
	}
	data, _ := io.ReadAll(io.LimitReader(f, 20<<20))
	f.Close()
	id, err := s.saveImage(r, site.ID, u.ID, data, credit, strings.TrimSpace(r.FormValue("caption")))
	if err != nil {
		back(w, r, self, err.Error(), true)
		return
	}
	_ = s.App.Store.SetArticleImage(ctx, a.ID, id)
	s.App.Cache.Clear()
	back(w, r, self, "Foto utama diganti.", false)
}

func (s *Server) saveImage(r *http.Request, siteID, uid int64, data []byte, credit, caption string) (int64, error) {
	saved, err := s.App.Media.Save(data)
	if err != nil {
		return 0, err
	}
	if saved.Width > 0 && saved.Width < 600 {
		return 0, errors.New("foto terlalu kecil, minimal lebar 600 piksel supaya tidak pecah")
	}
	return s.App.Store.CreateMedia(r.Context(), siteID, store.Media{Path: saved.Path, Width: saved.Width, Height: saved.Height, Variants: saved.Variants, Credit: credit, Caption: caption}, uid)
}
