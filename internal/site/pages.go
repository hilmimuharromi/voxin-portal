package site

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

const perPage = 15

// ---------- home ----------

type CatBlock struct {
	Cat   store.Category
	Lead  store.Article
	Items []store.Article
}

type HomeVM struct {
	Hero      *store.Article
	Side      []store.Article
	Intinya   []store.Article
	Regions   []store.Region
	RegionArt []store.Article
	Facts     []store.Article
	Blocks    []CatBlock
	Social    []store.Article
	Latest    []store.Article
}

func (s *Server) home(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	st := s.App.Store
	latest, _, err := st.Articles(ctx, store.Query{SiteID: site.ID, Limit: 30})
	if err != nil {
		return s.fail(err)
	}
	vm := HomeVM{}
	used := map[int64]bool{}
	for i := range latest {
		if latest[i].ImagePath != "" && latest[i].Type != "kerja_sama" {
			vm.Hero = &latest[i]
			used[latest[i].ID] = true
			break
		}
	}
	if vm.Hero == nil && len(latest) > 0 {
		vm.Hero = &latest[0]
		used[latest[0].ID] = true
	}
	for _, a := range latest {
		if len(vm.Side) == 3 {
			break
		}
		if !used[a.ID] && a.Type != "kerja_sama" {
			vm.Side = append(vm.Side, a)
			used[a.ID] = true
		}
	}
	vm.Intinya, _ = st.Popular(ctx, site.ID, 1, 3)
	if len(vm.Intinya) < 3 {
		vm.Intinya = nil
		for _, a := range latest {
			if len(vm.Intinya) == 3 {
				break
			}
			vm.Intinya = append(vm.Intinya, a)
		}
	}
	regs, _ := st.Regions(ctx, site.ID)
	for _, rg := range regs {
		if rg.Articles > 0 {
			vm.Regions = append(vm.Regions, rg)
		}
	}
	for _, a := range latest {
		if a.RegionID > 0 && len(vm.RegionArt) < 4 {
			vm.RegionArt = append(vm.RegionArt, a)
		}
	}
	vm.Facts, _, _ = st.Articles(ctx, store.Query{SiteID: site.ID, Category: "cek-fakta", Limit: 3})
	cats, _ := st.Categories(ctx, site.ID)
	for _, c := range cats {
		if len(vm.Blocks) == 3 || c.Slug == "cek-fakta" {
			continue
		}
		list, _, _ := st.Articles(ctx, store.Query{SiteID: site.ID, Category: c.Slug, Limit: 4})
		if len(list) >= 2 {
			vm.Blocks = append(vm.Blocks, CatBlock{Cat: c, Lead: list[0], Items: list[1:]})
		}
	}
	for _, a := range latest {
		if len(vm.Social) == 5 {
			break
		}
		if a.ImagePath != "" && !used[a.ID] {
			vm.Social = append(vm.Social, a)
		}
	}
	for _, a := range latest {
		if !used[a.ID] && len(vm.Latest) < 10 {
			vm.Latest = append(vm.Latest, a)
		}
	}
	meta := Meta{Title: site.Name + " — " + site.Settings.Tagline, Canonical: s.App.Cfg.BaseURL + "/",
		JSONLD: map[string]any{"@context": "https://schema.org", "@type": "NewsMediaOrganization", "name": site.Name, "url": s.App.Cfg.BaseURL + "/",
			"slogan": site.Settings.Tagline, "sameAs": nonEmpty(site.Settings.Instagram, site.Settings.TikTok, site.Settings.Threads)}}
	return s.render(ctx, "home", "", meta, vm)
}

func nonEmpty(v ...string) []string {
	var out []string
	for _, x := range v {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// ---------- article ----------

type ArticleVM struct {
	A        store.Article
	Gone     bool
	Related  []store.Article
	Dateline string
	Body     string // markdown with the dateline in front of the first paragraph
	ShareURL string
	WAURL    string
}

func (s *Server) article(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	cat, slug := r.PathValue("cat"), r.PathValue("slug")
	a, err := s.App.Store.ArticleBySlug(ctx, site.ID, slug)
	if errors.Is(err, store.ErrNotFound) {
		if id, err := s.App.Store.Redirect(ctx, site.ID, "/"+cat+"/"+slug); err == nil {
			if x, err := s.App.Store.GetArticle(ctx, id); err == nil && x.Status == "published" {
				return response{status: http.StatusMovedPermanently, loc: x.Path()}
			}
		}
		return s.notFoundResp(ctx)
	}
	if err != nil {
		return s.fail(err)
	}
	if a.Path() != "/"+cat+"/"+slug {
		return response{status: http.StatusMovedPermanently, loc: a.Path()}
	}
	vm := ArticleVM{A: a, Gone: a.Status == "unpublished"}
	canonical := s.App.Cfg.BaseURL + a.Path()
	vm.ShareURL = canonical
	vm.WAURL = "https://wa.me/?text=" + url.QueryEscape(a.Title+" "+canonical)
	vm.Body = a.BodyMD
	if a.Dateline != "" {
		vm.Dateline = strings.ToUpper(strings.TrimSpace(a.Dateline)) + ", " + strings.ToUpper(site.Name)
		body := strings.TrimLeft(a.BodyMD, " \n")
		if !strings.HasPrefix(strings.ToUpper(body), strings.ToUpper(strings.TrimSpace(a.Dateline))) && !strings.HasPrefix(body, "#") {
			vm.Body = "**" + vm.Dateline + "** – " + body
		}
	}
	if vm.Gone {
		res := s.render(ctx, "article", a.CategorySlug, Meta{Title: a.Title, NoIndex: true, Canonical: canonical}, vm)
		if res.status == http.StatusOK {
			res.status = http.StatusGone
		}
		return res
	}
	rel, _, _ := s.App.Store.Articles(ctx, store.Query{SiteID: site.ID, Category: a.CategorySlug, ExcludeID: []int64{a.ID}, Limit: 3})
	vm.Related = rel
	img := ""
	if a.ImagePath != "" {
		img = s.App.Cfg.BaseURL + imgURL(a.ImagePath, a.ImageVariants, 1200)
	}
	desc := a.Summary
	if desc == "" {
		desc = text.Excerpt(text.Plain(a.BodyMD), 155)
	}
	ld := map[string]any{
		"@context": "https://schema.org", "@type": "NewsArticle", "headline": a.Title, "description": desc,
		"mainEntityOfPage": canonical, "inLanguage": "id-ID",
		"publisher": map[string]any{"@type": "NewsMediaOrganization", "name": site.Name, "url": s.App.Cfg.BaseURL + "/",
			"logo": map[string]any{"@type": "ImageObject", "url": s.App.Cfg.BaseURL + "/static/logo.png"}},
	}
	if a.PublishedAt != nil {
		ld["datePublished"] = a.PublishedAt.Format(time.RFC3339)
	}
	ld["dateModified"] = a.UpdatedAt.Format(time.RFC3339)
	if img != "" {
		ld["image"] = []string{img}
	}
	if a.AuthorName != "" {
		ld["author"] = []map[string]any{{"@type": "Person", "name": a.AuthorName, "url": s.App.Cfg.BaseURL + "/penulis/" + a.AuthorSlug}}
	} else {
		ld["author"] = []map[string]any{{"@type": "Organization", "name": "Redaksi " + site.Name}}
	}
	if a.CategoryName != "" {
		ld["articleSection"] = a.CategoryName
	}
	meta := Meta{Title: a.Title, Description: desc, Canonical: canonical, Image: img, Type: "article", JSONLD: ld}
	return s.render(ctx, "article", a.CategorySlug, meta, vm)
}

// ---------- lists ----------

type ListVM struct {
	Kind     string // kategori | daerah | penulis | tag | cari
	Title    string
	Intro    string
	Q        string
	Items    []store.Article
	Total    int
	Page     int
	Pages    int
	PrevURL  string
	NextURL  string
	Region   *store.Region
	Author   *store.User
	Filter   string // daerah: "", berita, kerja_sama
	BaseURL  string
	Liputan  int
	Partners int
}

func page(r *http.Request) int {
	p, _ := strconv.Atoi(r.URL.Query().Get("hal"))
	if p < 1 {
		p = 1
	}
	return p
}

func (vm *ListVM) paginate(base string, extra url.Values) {
	vm.Pages = (vm.Total + perPage - 1) / perPage
	link := func(p int) string {
		v := url.Values{}
		for k, x := range extra {
			v[k] = x
		}
		if p > 1 {
			v.Set("hal", strconv.Itoa(p))
		}
		if q := v.Encode(); q != "" {
			return base + "?" + q
		}
		return base
	}
	if vm.Page > 1 {
		vm.PrevURL = link(vm.Page - 1)
	}
	if vm.Page < vm.Pages {
		vm.NextURL = link(vm.Page + 1)
	}
}

func (s *Server) list(ctx context.Context, q store.Query, vm *ListVM, nav string, meta Meta, base string, extra url.Values) response {
	q.Limit, q.Offset = perPage, (vm.Page-1)*perPage
	items, total, err := s.App.Store.Articles(ctx, q)
	if err != nil {
		return s.fail(err)
	}
	if vm.Page > 1 && len(items) == 0 {
		return s.notFoundResp(ctx)
	}
	vm.Items, vm.Total = items, total
	vm.paginate(base, extra)
	if vm.Page > 1 {
		meta.Title += fmt.Sprintf(" · Halaman %d", vm.Page)
		meta.Canonical = s.App.Cfg.BaseURL + base + "?hal=" + strconv.Itoa(vm.Page)
	}
	return s.render(ctx, "list", nav, meta, vm)
}

func (s *Server) pageOrCategory(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	slug := r.PathValue("first")
	if p, err := s.App.Store.PageBySlug(ctx, site.ID, slug); err == nil {
		return s.render(ctx, "page", "", Meta{Title: p.Title, Canonical: s.App.Cfg.BaseURL + "/" + p.Slug}, p)
	}
	cats, _ := s.App.Store.Categories(ctx, site.ID)
	for _, c := range cats {
		if c.Slug == slug {
			vm := &ListVM{Kind: "kategori", Title: c.Name, Page: page(r)}
			return s.list(ctx, store.Query{SiteID: site.ID, Category: c.Slug}, vm, c.Slug,
				Meta{Title: "Berita " + c.Name + " Terkini", Description: "Kumpulan berita " + c.Name + " terbaru dari " + site.Name + ".", Canonical: s.App.Cfg.BaseURL + "/" + c.Slug}, "/"+c.Slug, nil)
		}
	}
	return s.notFoundResp(ctx)
}

func (s *Server) regions(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	regs, err := s.App.Store.Regions(ctx, site.ID)
	if err != nil {
		return s.fail(err)
	}
	return s.render(ctx, "regions", "daerah", Meta{Title: "Berita Daerah", Canonical: s.App.Cfg.BaseURL + "/daerah"}, regs)
}

func (s *Server) region(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	rg, err := s.App.Store.RegionBySlug(ctx, site.ID, r.PathValue("slug"))
	if err != nil {
		return s.notFoundResp(ctx)
	}
	filter := r.URL.Query().Get("jenis")
	if filter != "berita" && filter != "kerja_sama" {
		filter = ""
	}
	vm := &ListVM{Kind: "daerah", Title: rg.Name, Intro: rg.Description, Page: page(r), Region: &rg, Filter: filter}
	_, vm.Liputan, _ = s.App.Store.Articles(ctx, store.Query{SiteID: site.ID, Region: rg.Slug, Type: "berita", Limit: 1})
	extra := url.Values{}
	if filter != "" {
		extra.Set("jenis", filter)
	}
	return s.list(ctx, store.Query{SiteID: site.ID, Region: rg.Slug, Type: filter}, vm, "daerah",
		Meta{Title: "Berita " + rg.Name, Description: firstNonEmpty(rg.Description, "Berita, liputan, dan informasi dari "+rg.Name+"."), Canonical: s.App.Cfg.BaseURL + "/daerah/" + rg.Slug},
		"/daerah/"+rg.Slug, extra)
}

func (s *Server) author(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	u, err := s.App.Store.UserBySlug(ctx, r.PathValue("slug"))
	if err != nil {
		return s.notFoundResp(ctx)
	}
	vm := &ListVM{Kind: "penulis", Title: u.Name, Intro: u.Bio, Page: page(r), Author: &u}
	meta := Meta{Title: u.Name + " · " + firstNonEmpty(u.Title, "Redaksi") + " " + site.Name, Canonical: s.App.Cfg.BaseURL + "/penulis/" + u.Slug,
		JSONLD: map[string]any{"@context": "https://schema.org", "@type": "ProfilePage", "mainEntity": map[string]any{"@type": "Person", "name": u.Name, "jobTitle": u.Title}}}
	return s.list(ctx, store.Query{SiteID: site.ID, AuthorID: u.ID}, vm, "", meta, "/penulis/"+u.Slug, nil)
}

func (s *Server) tag(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	slug := r.PathValue("slug")
	name, err := s.App.Store.TagName(ctx, slug)
	if err != nil {
		return s.notFoundResp(ctx)
	}
	vm := &ListVM{Kind: "tag", Title: "#" + name, Page: page(r)}
	return s.list(ctx, store.Query{SiteID: site.ID, Tag: slug}, vm, "", Meta{Title: "Berita tentang " + name, Canonical: s.App.Cfg.BaseURL + "/tag/" + slug}, "/tag/"+slug, nil)
}

func (s *Server) search(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		q = string([]rune(q)[:100])
	}
	vm := &ListVM{Kind: "cari", Title: "Cari", Q: q, Page: page(r)}
	if q == "" {
		return s.render(ctx, "list", "", Meta{Title: "Cari berita", NoIndex: true}, vm)
	}
	vm.Title = "Hasil untuk “" + q + "”"
	return s.list(ctx, store.Query{SiteID: site.ID, Search: q}, vm, "", Meta{Title: "Cari: " + q, NoIndex: true}, "/cari", url.Values{"q": {q}})
}

func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
