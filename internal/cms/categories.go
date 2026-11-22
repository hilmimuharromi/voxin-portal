package cms

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

// reservedSlugs are public URLs a rubrik must not shadow.
var reservedSlugs = map[string]bool{"daerah": true, "cari": true, "penulis": true, "tag": true, "static": true, "media": true,
	"v": true, "berita": true, "rss-xml": true, "sitemap-xml": true, "robots-txt": true, "sitemap-news-xml": true}

type CategoriesVM struct {
	Cats    []store.Category
	BaseURL string
}

func (s *Server) categories(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	cats, err := s.App.Store.CategoriesWithCounts(r.Context(), site.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, r, "categories", "Rubrik", "categories", CategoriesVM{Cats: cats, BaseURL: s.App.Cfg.BaseURL})
}

func (s *Server) categorySave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	site, _ := s.App.Site(ctx)
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		back(w, r, "/categories", "Nama rubrik wajib diisi.", true)
		return
	}
	slug := text.Slugify(firstNonEmpty(r.FormValue("slug"), name))
	if err := s.checkCategorySlug(r, site.ID, id, slug); err != nil {
		back(w, r, "/categories", err.Error(), true)
		return
	}
	pos, _ := strconv.Atoi(r.FormValue("position"))
	c := store.Category{ID: id, Slug: slug, Name: name, Description: strings.TrimSpace(r.FormValue("description")), Position: pos,
		InNav: r.FormValue("in_nav") == "1", OnHome: r.FormValue("on_home") == "1"}
	var oldSlug string
	if id > 0 {
		cats, _ := s.App.Store.Categories(ctx, site.ID)
		for _, x := range cats {
			if x.ID == id {
				oldSlug = x.Slug
				if pos == 0 {
					c.Position = x.Position
				}
			}
		}
	}
	if _, err := s.App.Store.SaveCategory(ctx, site.ID, c); err != nil {
		back(w, r, "/categories", "Gagal menyimpan: "+err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	msg := "Rubrik " + name + " disimpan."
	if id == 0 {
		msg = "Rubrik " + name + " ditambahkan. Sudah bisa dipilih di editor artikel."
	} else if oldSlug != "" && oldSlug != slug {
		msg += " Alamat lama /" + oldSlug + "/... otomatis dialihkan ke /" + slug + "/..."
	}
	back(w, r, "/categories", msg, false)
}

func (s *Server) checkCategorySlug(r *http.Request, siteID, id int64, slug string) error {
	if slug == "" {
		return fmt.Errorf("slug tidak valid; pakai huruf, angka, dan tanda hubung")
	}
	if reservedSlugs[slug] {
		return fmt.Errorf("slug %q sudah dipakai alamat situs; pilih yang lain", slug)
	}
	if _, err := s.App.Store.PageBySlug(r.Context(), siteID, slug); err == nil {
		return fmt.Errorf("slug %q sudah dipakai halaman wajib", slug)
	}
	cats, _ := s.App.Store.Categories(r.Context(), siteID)
	for _, c := range cats {
		if c.Slug == slug && c.ID != id {
			return fmt.Errorf("slug %q sudah dipakai rubrik %s", slug, c.Name)
		}
	}
	return nil
}

func (s *Server) categoryMove(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	dir := 1
	if r.FormValue("dir") == "up" {
		dir = -1
	}
	if err := s.App.Store.MoveCategory(r.Context(), site.ID, pathID(r), dir); err != nil {
		back(w, r, "/categories", err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	http.Redirect(w, r, "/categories", http.StatusSeeOther)
}

func (s *Server) categoryDelete(w http.ResponseWriter, r *http.Request) {
	site, _ := s.App.Site(r.Context())
	if err := s.App.Store.DeleteCategory(r.Context(), site.ID, pathID(r)); err != nil {
		back(w, r, "/categories", err.Error(), true)
		return
	}
	s.App.Cache.Clear()
	back(w, r, "/categories", "Rubrik dihapus. Artikelnya tetap ada tanpa rubrik (alamatnya jadi /berita/...); alamat lama dialihkan otomatis.", false)
}
