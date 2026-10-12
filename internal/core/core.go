// Package core holds what the public site, the CMS and the engine API share:
// the app wiring, publishing rules, the page cache and the scheduler.
package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/config"
	"github.com/hilmimuharromi/voxin-portal/internal/media"
	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

type App struct {
	Store *store.Store
	Cfg   *config.Config
	Media media.Store
	Loc   *time.Location
	Cache *Cache
	Log   *slog.Logger
}

func New(st *store.Store, cfg *config.Config, log *slog.Logger) *App {
	loc, err := time.LoadLocation(cfg.TZ)
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	return &App{Store: st, Cfg: cfg, Media: media.Store{Dir: cfg.DataDir + "/media"}, Loc: loc, Cache: NewCache(60 * time.Second), Log: log}
}

func (a *App) Site(ctx context.Context) (store.Site, error) {
	return a.Store.SiteByKey(ctx, a.Cfg.Site)
}

// UniqueSlug returns base, or base-2, base-3… if taken on the site.
func (a *App) UniqueSlug(ctx context.Context, siteID int64, base string, exceptID int64) (string, error) {
	base = text.Slugify(base)
	if base == "" {
		base = "artikel"
	}
	slug := base
	for i := 2; i < 100; i++ {
		taken, err := a.Store.SlugTaken(ctx, siteID, slug, exceptID)
		if err != nil {
			return "", err
		}
		if !taken {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix()), nil
}

// PublishProblems lists what blocks an article from going live.
func PublishProblems(x store.Article) []string {
	var p []string
	if strings.TrimSpace(x.Title) == "" {
		p = append(p, "Judul kosong")
	}
	if x.Slug == "" {
		p = append(p, "Slug kosong")
	}
	if x.CategoryID == 0 {
		p = append(p, "Kategori belum dipilih")
	}
	if len(strings.Fields(x.BodyMD)) < 40 {
		p = append(p, "Isi terlalu pendek (minimal 40 kata)")
	}
	if x.AuthorID == 0 {
		p = append(p, "Penulis belum dipilih")
	}
	if x.ImageID > 0 && strings.TrimSpace(x.ImageCredit) == "" {
		p = append(p, "Kredit foto wajib diisi")
	}
	if x.Type == "kurasi" && strings.TrimSpace(x.SourceName) == "" {
		p = append(p, "Artikel kurasi wajib menyebut media sumber")
	}
	if x.Type == "kerja_sama" && x.RegionID == 0 {
		p = append(p, "Artikel kerja sama wajib dikaitkan ke daerah")
	}
	return p
}

// Run publishes scheduled articles and prunes analytics scratch data.
func (a *App) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	lastPrune := time.Time{}
	for {
		if n, err := a.Store.PublishDue(ctx); err != nil {
			a.Log.Warn("terbit terjadwal", "err", err)
		} else if n > 0 {
			a.Log.Info("artikel terjadwal terbit", "jumlah", n)
			a.Cache.Clear()
		}
		if time.Since(lastPrune) > 6*time.Hour {
			_ = a.Store.PruneSeen(ctx)
			lastPrune = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---------- page cache ----------

type cached struct {
	body  []byte
	ctype string
	until time.Time
}

// Cache keeps rendered public pages for a short time; any CMS write clears it.
type Cache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]cached
}

func NewCache(ttl time.Duration) *Cache { return &Cache{ttl: ttl, m: map[string]cached{}} }

func (c *Cache) Get(key string) ([]byte, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.until) {
		return nil, "", false
	}
	return e.body, e.ctype, true
}

func (c *Cache) Set(key string, body []byte, ctype string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 5000 {
		c.m = map[string]cached{}
	}
	c.m[key] = cached{body: body, ctype: ctype, until: time.Now().Add(c.ttl)}
}

func (c *Cache) Clear() {
	c.mu.Lock()
	c.m = map[string]cached{}
	c.mu.Unlock()
}
