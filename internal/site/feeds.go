package site

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"github.com/hilmimuharromi/voxin-portal/internal/store"
	"github.com/hilmimuharromi/voxin-portal/internal/text"
)

func xmlResp(v any) response {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	enc := xml.NewEncoder(&b)
	enc.Indent("", " ")
	if err := enc.Encode(v); err != nil {
		return response{status: 500, ctype: "text/plain", body: []byte(err.Error())}
	}
	return response{status: http.StatusOK, ctype: "application/xml; charset=utf-8", body: b.Bytes()}
}

func (s *Server) robots(r *http.Request) response {
	body := fmt.Sprintf("User-agent: *\nAllow: /\nDisallow: /cari\n\nSitemap: %s/sitemap.xml\nSitemap: %s/sitemap-news.xml\n", s.App.Cfg.BaseURL, s.App.Cfg.BaseURL)
	return response{status: http.StatusOK, ctype: "text/plain; charset=utf-8", body: []byte(body)}
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title    string    `xml:"title"`
	Link     string    `xml:"link"`
	Desc     string    `xml:"description"`
	Language string    `xml:"language"`
	Self     rssLink   `xml:"atom:link"`
	Items    []rssItem `xml:"item"`
}

type rssLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title    string        `xml:"title"`
	Link     string        `xml:"link"`
	GUID     string        `xml:"guid"`
	PubDate  string        `xml:"pubDate"`
	Desc     string        `xml:"description"`
	Category string        `xml:"category,omitempty"`
	Encl     *rssEnclosure `xml:"enclosure,omitempty"`
}

type rssEnclosure struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
	Len  int    `xml:"length,attr"`
}

func (s *Server) rss(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	items, _, err := s.App.Store.Articles(ctx, store.Query{SiteID: site.ID, Limit: 40})
	if err != nil {
		return s.fail(err)
	}
	base := s.App.Cfg.BaseURL
	f := rssFeed{Version: "2.0", Atom: "http://www.w3.org/2005/Atom", Channel: rssChannel{Title: site.Name, Link: base + "/", Desc: site.Settings.About,
		Language: "id", Self: rssLink{Href: base + "/rss.xml", Rel: "self", Type: "application/rss+xml"}}}
	for _, a := range items {
		it := rssItem{Title: a.Title, Link: base + a.Path(), GUID: base + a.Path(), Desc: firstNonEmpty(a.Summary, text.Excerpt(text.Plain(a.BodyMD), 200)), Category: a.CategoryName}
		if a.PublishedAt != nil {
			it.PubDate = a.PublishedAt.Format(time.RFC1123Z)
		}
		if a.ImagePath != "" {
			it.Encl = &rssEnclosure{URL: base + imgURL(a.ImagePath, a.ImageVariants, 800), Type: "image/jpeg"}
		}
		f.Channel.Items = append(f.Channel.Items, it)
	}
	res := xmlResp(f)
	res.ctype = "application/rss+xml; charset=utf-8"
	return res
}

type urlset struct {
	XMLName xml.Name `xml:"urlset"`
	NS      string   `xml:"xmlns,attr"`
	News    string   `xml:"xmlns:news,attr,omitempty"`
	URLs    []smURL  `xml:"url"`
}

type smURL struct {
	Loc     string  `xml:"loc"`
	LastMod string  `xml:"lastmod,omitempty"`
	News    *smNews `xml:"news:news,omitempty"`
}

type smNews struct {
	Pub   smPub  `xml:"news:publication"`
	Date  string `xml:"news:publication_date"`
	Title string `xml:"news:title"`
}

type smPub struct {
	Name string `xml:"news:name"`
	Lang string `xml:"news:language"`
}

func (s *Server) sitemap(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	base := s.App.Cfg.BaseURL
	us := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	us.URLs = append(us.URLs, smURL{Loc: base + "/"})
	cats, _ := s.App.Store.Categories(ctx, site.ID)
	for _, c := range cats {
		us.URLs = append(us.URLs, smURL{Loc: base + "/" + c.Slug})
	}
	regs, _ := s.App.Store.Regions(ctx, site.ID)
	for _, rg := range regs {
		us.URLs = append(us.URLs, smURL{Loc: base + "/daerah/" + rg.Slug})
	}
	pages, _ := s.App.Store.Pages(ctx, site.ID)
	for _, p := range pages {
		us.URLs = append(us.URLs, smURL{Loc: base + "/" + p.Slug, LastMod: p.UpdatedAt.Format("2006-01-02")})
	}
	items, _, err := s.App.Store.Articles(ctx, store.Query{SiteID: site.ID, Limit: 5000})
	if err != nil {
		return s.fail(err)
	}
	for _, a := range items {
		us.URLs = append(us.URLs, smURL{Loc: base + a.Path(), LastMod: a.UpdatedAt.Format(time.RFC3339)})
	}
	return xmlResp(us)
}

// sitemapNews lists articles from the last 48 hours (Google News rule).
func (s *Server) sitemapNews(r *http.Request) response {
	ctx := r.Context()
	site, err := s.App.Site(ctx)
	if err != nil {
		return s.fail(err)
	}
	since := time.Now().Add(-48 * time.Hour)
	items, _, err := s.App.Store.Articles(ctx, store.Query{SiteID: site.ID, Since: &since, Limit: 1000})
	if err != nil {
		return s.fail(err)
	}
	us := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9", News: "http://www.google.com/schemas/sitemap-news/0.9"}
	for _, a := range items {
		if a.PublishedAt == nil {
			continue
		}
		us.URLs = append(us.URLs, smURL{Loc: s.App.Cfg.BaseURL + a.Path(), News: &smNews{Pub: smPub{Name: site.Name, Lang: "id"},
			Date: a.PublishedAt.Format(time.RFC3339), Title: a.Title}})
	}
	return xmlResp(us)
}
