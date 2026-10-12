package cms

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/media"
	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

// EnginePayload is what voxin-engine's "Terbitkan di voxin.id" webhook sends.
type EnginePayload struct {
	Title    string   `json:"title"`
	Slug     string   `json:"slug"`
	Meta     string   `json:"meta_description"`
	BodyMD   string   `json:"body_markdown"`
	BodyHTML string   `json:"body_html"`
	ImageURL string   `json:"image_url"`
	Credit   string   `json:"image_credit"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Sources  string   `json:"sources"`
}

func jsonOut(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// engineArticle turns an engine article into a Kurasi draft waiting for an editor.
// It never publishes: an editor still has to review it in the CMS.
func (s *Server) engineArticle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": err.Error()})
		return
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	want := site.Settings.EngineToken
	if want == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
		jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "token engine salah atau belum dibuat di Pengaturan CMS"})
		return
	}
	var p EnginePayload
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&p); err != nil {
		jsonOut(w, 400, map[string]string{"error": "JSON tidak valid: " + err.Error()})
		return
	}
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" || strings.TrimSpace(p.BodyMD) == "" {
		jsonOut(w, 400, map[string]string{"error": "judul dan isi wajib ada"})
		return
	}
	ref := "engine:" + text.Slugify(firstNonEmpty(p.Slug, p.Title))
	// same story sent twice: update the draft, or report the live article
	if old, err := s.App.Store.ArticleByEngineRef(ctx, site.ID, ref); err == nil {
		if old.Status == "published" || old.Status == "scheduled" {
			jsonOut(w, 200, map[string]any{"id": old.ID, "url": s.App.Cfg.BaseURL + old.Path(), "status": old.Status, "cms_url": s.cmsURL(old.ID)})
			return
		}
		old.Title, old.Summary, old.BodyMD = p.Title, text.Excerpt(p.Meta, 200), p.BodyMD
		_ = s.App.Store.SaveArticle(ctx, old, 0)
		jsonOut(w, 200, map[string]any{"id": old.ID, "url": s.App.Cfg.BaseURL + old.Path(), "status": old.Status, "cms_url": s.cmsURL(old.ID)})
		return
	}
	a := store.Article{SiteID: site.ID, Type: "kurasi", Status: "review", Title: p.Title, Summary: text.Excerpt(strings.TrimSpace(p.Meta), 200),
		BodyMD: strings.TrimSpace(p.BodyMD), Dateline: "JAKARTA", AIAssisted: true, EngineRef: ref,
		SourceName: strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p.Sources), "Sumber:"))}
	cats, _ := s.App.Store.Categories(ctx, site.ID)
	wantCat := text.Slugify(p.Category)
	for _, c := range cats {
		if c.Slug == wantCat || strings.EqualFold(c.Name, p.Category) {
			a.CategoryID, a.CategorySlug = c.ID, c.Slug
		}
	}
	a.Slug, err = s.App.UniqueSlug(ctx, site.ID, firstNonEmpty(p.Slug, p.Title), 0)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if p.ImageURL != "" {
		dctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		data, err := media.Download(dctx, p.ImageURL)
		cancel()
		if err == nil {
			if id, err := s.saveImage(r, site.ID, 0, data, strings.TrimSpace(p.Credit), ""); err == nil {
				a.ImageID = id
			} else {
				s.App.Log.Warn("gambar dari engine", "err", err)
			}
		} else {
			s.App.Log.Warn("unduh gambar engine", "url", p.ImageURL, "err", err)
		}
	}
	id, err := s.App.Store.CreateArticle(ctx, a)
	if err != nil {
		jsonOut(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var tags []string
	for _, t := range p.Tags {
		if t = strings.TrimSpace(t); t != "" && !strings.EqualFold(t, "voxin") {
			tags = append(tags, t)
		}
	}
	_ = s.App.Store.SetTags(ctx, id, parseTags(strings.Join(tags, ",")))
	x, _ := s.App.Store.GetArticle(ctx, id)
	s.App.Log.Info("artikel dari engine masuk antrean editor", "id", id, "judul", p.Title)
	jsonOut(w, http.StatusCreated, map[string]any{"id": id, "url": s.App.Cfg.BaseURL + x.Path(), "status": "review", "cms_url": s.cmsURL(id)})
}

func (s *Server) cmsURL(id int64) string {
	return s.App.Cfg.CMSURL + "/articles/" + itoa(id)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
