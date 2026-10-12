package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Article types and the public label each one gets.
var ArticleTypes = []string{"berita", "kurasi", "kerja_sama", "opini"}

var TypeLabels = map[string]string{"berita": "Berita", "kurasi": "Kurasi", "kerja_sama": "Kerja Sama", "opini": "Opini"}

var StatusLabels = map[string]string{"draft": "Draft", "review": "Perlu ditinjau", "scheduled": "Terjadwal", "published": "Terbit", "unpublished": "Diturunkan"}

type Article struct {
	ID            int64
	SiteID        int64
	Type          string
	Status        string
	Title         string
	Slug          string
	Summary       string
	BodyMD        string
	Intinya       string
	Highlight     string
	CategoryID    int64
	CategorySlug  string
	CategoryName  string
	RegionID      int64
	RegionSlug    string
	RegionName    string
	Dateline      string
	AuthorID      int64
	AuthorName    string
	AuthorSlug    string
	AuthorTitle   string
	EditorID      int64
	EditorName    string
	ImageID       int64
	ImagePath     string
	ImageCredit   string
	ImageCaption  string
	ImageVariants string // raw JSON
	SourceName    string
	SourceURL     string
	AIAssisted    bool
	EngineRef     string
	UnpublishNote string
	EditorNote    string
	PublishedAt   *time.Time
	ScheduledAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Tags          []Tag
	Social        map[string]string
	Views         int // filled by some queries
}

type Tag struct{ Name, Slug string }

// Path is the public URL path: /{kategori}/{slug}.
func (a Article) Path() string {
	cat := a.CategorySlug
	if cat == "" {
		cat = "berita"
	}
	return "/" + cat + "/" + a.Slug
}

func (a Article) TypeLabel() string { return TypeLabels[a.Type] }

const articleSelect = `SELECT a.id, a.site_id, a.type, a.status, a.title, a.slug, a.summary, a.body_md, a.intinya, a.highlight,
	COALESCE(a.category_id,0), COALESCE(c.slug,''), COALESCE(c.name,''),
	COALESCE(a.region_id,0), COALESCE(r.slug,''), COALESCE(r.name,''), a.dateline,
	COALESCE(a.author_id,0), COALESCE(u.name,''), COALESCE(u.slug,''), COALESCE(u.title,''),
	COALESCE(a.editor_id,0), COALESCE(e.name,''),
	COALESCE(a.image_id,0), COALESCE(m.path,''), COALESCE(m.credit,''), COALESCE(m.caption,''), COALESCE(m.variants::text,'{}'),
	a.source_name, a.source_url, a.ai_assisted, a.engine_ref, a.unpublish_note, a.editor_note,
	a.published_at, a.scheduled_at, a.created_at, a.updated_at
	FROM articles a
	LEFT JOIN categories c ON c.id=a.category_id
	LEFT JOIN regions r ON r.id=a.region_id
	LEFT JOIN users u ON u.id=a.author_id
	LEFT JOIN users e ON e.id=a.editor_id
	LEFT JOIN media m ON m.id=a.image_id`

func scanArticle(sc interface{ Scan(...any) error }) (Article, error) {
	var a Article
	err := sc.Scan(&a.ID, &a.SiteID, &a.Type, &a.Status, &a.Title, &a.Slug, &a.Summary, &a.BodyMD, &a.Intinya, &a.Highlight,
		&a.CategoryID, &a.CategorySlug, &a.CategoryName, &a.RegionID, &a.RegionSlug, &a.RegionName, &a.Dateline,
		&a.AuthorID, &a.AuthorName, &a.AuthorSlug, &a.AuthorTitle, &a.EditorID, &a.EditorName,
		&a.ImageID, &a.ImagePath, &a.ImageCredit, &a.ImageCaption, &a.ImageVariants,
		&a.SourceName, &a.SourceURL, &a.AIAssisted, &a.EngineRef, &a.UnpublishNote, &a.EditorNote,
		&a.PublishedAt, &a.ScheduledAt, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func nullID(id int64) any {
	if id > 0 {
		return id
	}
	return nil
}

func (s *Store) CreateArticle(ctx context.Context, a Article) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO articles(site_id, type, status, title, slug, summary, body_md, intinya, highlight, category_id, region_id,
		dateline, author_id, image_id, source_name, source_url, ai_assisted, engine_ref)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id`,
		a.SiteID, a.Type, a.Status, a.Title, a.Slug, a.Summary, a.BodyMD, a.Intinya, a.Highlight, nullID(a.CategoryID), nullID(a.RegionID),
		a.Dateline, nullID(a.AuthorID), nullID(a.ImageID), a.SourceName, a.SourceURL, a.AIAssisted, a.EngineRef).Scan(&id)
	return id, err
}

// SaveArticle updates the editable fields and keeps a version of the text.
func (s *Store) SaveArticle(ctx context.Context, a Article, by int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldTitle, oldBody string
	if err := tx.QueryRowContext(ctx, `SELECT title, body_md FROM articles WHERE id=$1 FOR UPDATE`, a.ID).Scan(&oldTitle, &oldBody); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE articles SET type=$2, title=$3, slug=$4, summary=$5, body_md=$6, intinya=$7, highlight=$8, category_id=$9,
		region_id=$10, dateline=$11, author_id=$12, image_id=$13, source_name=$14, source_url=$15, ai_assisted=$16, editor_note=$17, updated_at=now() WHERE id=$1`,
		a.ID, a.Type, a.Title, a.Slug, a.Summary, a.BodyMD, a.Intinya, a.Highlight, nullID(a.CategoryID), nullID(a.RegionID),
		a.Dateline, nullID(a.AuthorID), nullID(a.ImageID), a.SourceName, a.SourceURL, a.AIAssisted, a.EditorNote)
	if err != nil {
		return err
	}
	if oldTitle != a.Title || oldBody != a.BodyMD {
		if _, err := tx.ExecContext(ctx, `INSERT INTO article_versions(article_id, title, body_md, changed_by) VALUES ($1,$2,$3,$4)`,
			a.ID, oldTitle, oldBody, nullID(by)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetArticle(ctx context.Context, id int64) (Article, error) {
	a, err := scanArticle(s.DB.QueryRowContext(ctx, articleSelect+` WHERE a.id=$1`, id))
	if err != nil {
		return a, err
	}
	return a, s.fillExtras(ctx, &a)
}

func (s *Store) ArticleByEngineRef(ctx context.Context, siteID int64, ref string) (Article, error) {
	return scanArticle(s.DB.QueryRowContext(ctx, articleSelect+` WHERE a.site_id=$1 AND a.engine_ref=$2 ORDER BY a.id DESC LIMIT 1`, siteID, ref))
}

// ArticleBySlug finds a public (published or taken-down) article.
func (s *Store) ArticleBySlug(ctx context.Context, siteID int64, slug string) (Article, error) {
	a, err := scanArticle(s.DB.QueryRowContext(ctx, articleSelect+` WHERE a.site_id=$1 AND a.slug=$2 AND a.status IN ('published','unpublished')`, siteID, slug))
	if err != nil {
		return a, err
	}
	return a, s.fillExtras(ctx, &a)
}

func (s *Store) SlugTaken(ctx context.Context, siteID int64, slug string, exceptID int64) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM articles WHERE site_id=$1 AND slug=$2 AND id<>$3`, siteID, slug, exceptID).Scan(&n)
	return n > 0, err
}

func (s *Store) fillExtras(ctx context.Context, a *Article) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT tag, slug FROM article_tags WHERE article_id=$1 ORDER BY tag`, a.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Name, &t.Slug); err != nil {
			rows.Close()
			return err
		}
		a.Tags = append(a.Tags, t)
	}
	rows.Close()
	a.Social = map[string]string{}
	rows, err = s.DB.QueryContext(ctx, `SELECT platform, url FROM article_social WHERE article_id=$1`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p, u string
		if err := rows.Scan(&p, &u); err != nil {
			return err
		}
		a.Social[p] = u
	}
	return rows.Err()
}

func (s *Store) SetTags(ctx context.Context, id int64, tags []Tag) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_tags WHERE article_id=$1`, id); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO article_tags(article_id, tag, slug) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, id, t.Name, t.Slug); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetSocial(ctx context.Context, id int64, links map[string]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_social WHERE article_id=$1`, id); err != nil {
		return err
	}
	for p, u := range links {
		if u = strings.TrimSpace(u); u != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO article_social(article_id, platform, url) VALUES ($1,$2,$3)`, id, p, u); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// Publish marks an article published now (keeps the first publish time on re-publish).
func (s *Store) Publish(ctx context.Context, id, editorID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE articles SET status='published', editor_id=$2, published_at=COALESCE(published_at, now()),
		scheduled_at=NULL, unpublish_note='', updated_at=now() WHERE id=$1`, id, nullID(editorID))
	return err
}

func (s *Store) Schedule(ctx context.Context, id, editorID int64, at time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE articles SET status='scheduled', editor_id=$2, scheduled_at=$3, updated_at=now() WHERE id=$1`, id, nullID(editorID), at)
	return err
}

func (s *Store) SetStatus(ctx context.Context, id int64, status, note string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE articles SET status=$2, unpublish_note=CASE WHEN $2='unpublished' THEN $3 ELSE unpublish_note END,
		scheduled_at=CASE WHEN $2='scheduled' THEN scheduled_at ELSE NULL END, updated_at=now() WHERE id=$1`, id, status, note)
	return err
}

// PublishDue publishes scheduled articles whose time has come.
func (s *Store) PublishDue(ctx context.Context) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE articles SET status='published', published_at=COALESCE(published_at, scheduled_at), scheduled_at=NULL, updated_at=now()
		WHERE status='scheduled' AND scheduled_at <= now()`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) DeleteArticle(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM articles WHERE id=$1 AND status IN ('draft','review')`, id)
	return err
}

func (s *Store) AddRedirect(ctx context.Context, siteID int64, oldPath string, articleID int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO slug_redirects(site_id, old_path, article_id) VALUES ($1,$2,$3)
		ON CONFLICT (site_id, old_path) DO UPDATE SET article_id=EXCLUDED.article_id`, siteID, oldPath, articleID)
	return err
}

func (s *Store) Redirect(ctx context.Context, siteID int64, path string) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT article_id FROM slug_redirects WHERE site_id=$1 AND old_path=$2`, siteID, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

type Version struct {
	ID        int64
	Title     string
	BodyMD    string
	By        string
	CreatedAt time.Time
}

func (s *Store) Versions(ctx context.Context, articleID int64, limit int) ([]Version, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT v.id, v.title, v.body_md, COALESCE(u.name,'Sistem'), v.created_at FROM article_versions v
		LEFT JOIN users u ON u.id=v.changed_by WHERE v.article_id=$1 ORDER BY v.id DESC LIMIT $2`, articleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.Title, &v.BodyMD, &v.By, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Query filters article lists (public and CMS).
type Query struct {
	SiteID    int64
	Statuses  []string // empty = published only
	Type      string
	Category  string // slug
	Region    string // slug
	AuthorID  int64
	Tag       string // slug
	Search    string
	ExcludeID []int64
	Since     *time.Time
	Limit     int
	Offset    int
	OrderBy   string // "" = newest published, "updated" = last edited (CMS)
}

func (s *Store) Articles(ctx context.Context, q Query) ([]Article, int, error) {
	where := []string{"a.site_id=$1"}
	args := []any{q.SiteID}
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if len(q.Statuses) == 0 {
		where = append(where, "a.status='published'")
	} else {
		var ph []string
		for _, st := range q.Statuses {
			ph = append(ph, arg(st))
		}
		where = append(where, "a.status IN ("+strings.Join(ph, ",")+")")
	}
	if q.Type != "" {
		where = append(where, "a.type="+arg(q.Type))
	}
	if q.Category != "" {
		where = append(where, "c.slug="+arg(q.Category))
	}
	if q.Region != "" {
		where = append(where, "r.slug="+arg(q.Region))
	}
	if q.AuthorID > 0 {
		where = append(where, "a.author_id="+arg(q.AuthorID))
	}
	if q.Tag != "" {
		where = append(where, "EXISTS (SELECT 1 FROM article_tags t WHERE t.article_id=a.id AND t.slug="+arg(q.Tag)+")")
	}
	if q.Search != "" {
		p := arg("%" + q.Search + "%")
		where = append(where, "(a.title ILIKE "+p+" OR a.summary ILIKE "+p+" OR a.body_md ILIKE "+p+")")
	}
	for _, id := range q.ExcludeID {
		where = append(where, "a.id<>"+arg(id))
	}
	if q.Since != nil {
		where = append(where, "a.published_at>="+arg(*q.Since))
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM articles a LEFT JOIN categories c ON c.id=a.category_id LEFT JOIN regions r ON r.id=a.region_id WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "a.published_at DESC NULLS LAST, a.id DESC"
	if q.OrderBy == "updated" {
		order = "a.updated_at DESC"
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	query := articleSelect + " WHERE " + cond + " ORDER BY " + order + " LIMIT " + arg(q.Limit) + " OFFSET " + arg(q.Offset)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// Popular returns the most viewed published articles in the last `days`.
func (s *Store) Popular(ctx context.Context, siteID int64, days, limit int) ([]Article, error) {
	rows, err := s.DB.QueryContext(ctx, articleSelect+`
		JOIN (SELECT article_id, sum(views) v FROM pageviews_daily WHERE site_id=$1 AND article_id>0 AND day > current_date - $2::int
		      GROUP BY article_id) pv ON pv.article_id=a.id
		WHERE a.status='published' ORDER BY pv.v DESC LIMIT $3`, siteID, days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type TypeCount struct {
	Status string
	N      int
}

func (s *Store) StatusCounts(ctx context.Context, siteID int64) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT status, count(*) FROM articles WHERE site_id=$1 GROUP BY status`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// TagName returns the display name of a tag slug.
func (s *Store) TagName(ctx context.Context, slug string) (string, error) {
	var n string
	err := s.DB.QueryRowContext(ctx, `SELECT tag FROM article_tags WHERE slug=$1 LIMIT 1`, slug).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return n, err
}

func (s *Store) SetArticleImage(ctx context.Context, id, mediaID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE articles SET image_id=$2, updated_at=now() WHERE id=$1`, id, nullID(mediaID))
	return err
}
