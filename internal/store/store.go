// Package store holds the PostgreSQL schema (embedded migrations) and queries.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var ErrNotFound = errors.New("tidak ditemukan")

type Store struct{ DB *sql.DB }

func Open(ctx context.Context, url string) (*Store, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("tidak bisa konek ke PostgreSQL (%s): %w", redact(url), err)
	}
	s := &Store{DB: db}
	if err := s.migrate(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func redact(u string) string {
	if i := strings.Index(u, "@"); i > 0 {
		if j := strings.Index(u, "://"); j > 0 && j < i {
			return u[:j+3] + "***" + u[i:]
		}
	}
	return u
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, _ := migrationFS.ReadFile("migrations/" + name)
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrasi %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ---------- sites ----------

type SiteSettings struct {
	Tagline     string `json:"tagline"`
	About       string `json:"about"`
	Handle      string `json:"handle"`
	Instagram   string `json:"instagram"`
	TikTok      string `json:"tiktok"`
	Threads     string `json:"threads"`
	Company     string `json:"company"`
	Address     string `json:"address"`
	PressStatus string `json:"press_status"`
	Email       string `json:"email"`
	EngineToken string `json:"engine_token"`
	GA4         string `json:"ga4"`
}

type Site struct {
	ID       int64
	Key      string
	Name     string
	BaseURL  string
	Settings SiteSettings
}

func (s *Store) SiteByKey(ctx context.Context, key string) (Site, error) {
	var st Site
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id, key, name, base_url, settings FROM sites WHERE key=$1`, key).Scan(&st.ID, &st.Key, &st.Name, &st.BaseURL, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	_ = json.Unmarshal(raw, &st.Settings)
	return st, err
}

func (s *Store) SaveSiteSettings(ctx context.Context, id int64, name string, set SiteSettings) error {
	b, _ := json.Marshal(set)
	_, err := s.DB.ExecContext(ctx, `UPDATE sites SET name=$2, settings=$3 WHERE id=$1`, id, name, b)
	return err
}

// ---------- users ----------

type User struct {
	ID           int64
	Email        string
	Name         string
	Slug         string
	PasswordHash string
	Role         string
	Title        string
	Region       string
	UKWLevel     string
	UKWNumber    string
	Bio          string
	Photo        string
	Active       bool
	CreatedAt    time.Time
}

const userCols = `id, email, name, slug, password_hash, role, title, region, ukw_level, ukw_number, bio, photo, active, created_at`

func scanUser(sc interface{ Scan(...any) error }) (User, error) {
	var u User
	err := sc.Scan(&u.ID, &u.Email, &u.Name, &u.Slug, &u.PasswordHash, &u.Role, &u.Title, &u.Region, &u.UKWLevel, &u.UKWNumber, &u.Bio, &u.Photo, &u.Active, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, u User) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO users(email, name, slug, password_hash, role, title, region, ukw_level, ukw_number, bio)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		strings.ToLower(u.Email), u.Name, u.Slug, u.PasswordHash, u.Role, u.Title, u.Region, u.UKWLevel, u.UKWNumber, u.Bio).Scan(&id)
	return id, err
}

func (s *Store) UpdateUserProfile(ctx context.Context, u User) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET name=$2, title=$3, region=$4, ukw_level=$5, ukw_number=$6, bio=$7, role=$8, active=$9 WHERE id=$1`,
		u.ID, u.Name, u.Title, u.Region, u.UKWLevel, u.UKWNumber, u.Bio, u.Role, u.Active)
	return err
}

func (s *Store) SetUserPassword(ctx context.Context, id int64, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, id, hash)
	return err
}

func (s *Store) SetUserPhoto(ctx context.Context, id int64, path string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET photo=$2 WHERE id=$1`, id, path)
	return err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email=$1`, strings.ToLower(strings.TrimSpace(email))))
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
}

func (s *Store) UserBySlug(ctx context.Context, slug string) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE slug=$1 AND active`, slug))
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY active DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------- categories & regions ----------

type Category struct {
	ID       int64
	Slug     string
	Name     string
	Position int
	InNav    bool
}

func (s *Store) Categories(ctx context.Context, siteID int64) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, slug, name, position, in_nav FROM categories WHERE site_id=$1 ORDER BY position, name`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.Position, &c.InNav); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type Region struct {
	ID          int64
	Slug        string
	Name        string
	Province    string
	Description string
	Logo        string
	Partner     bool
	Articles    int
}

func (s *Store) Regions(ctx context.Context, siteID int64) ([]Region, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id, r.slug, r.name, r.province, r.description, r.logo, r.partner,
		(SELECT count(*) FROM articles a WHERE a.region_id=r.id AND a.status='published')
		FROM regions r WHERE r.site_id=$1 ORDER BY r.partner DESC, r.name`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Region
	for rows.Next() {
		var r Region
		if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.Province, &r.Description, &r.Logo, &r.Partner, &r.Articles); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RegionBySlug(ctx context.Context, siteID int64, slug string) (Region, error) {
	var r Region
	err := s.DB.QueryRowContext(ctx, `SELECT id, slug, name, province, description, logo, partner FROM regions WHERE site_id=$1 AND slug=$2`, siteID, slug).
		Scan(&r.ID, &r.Slug, &r.Name, &r.Province, &r.Description, &r.Logo, &r.Partner)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) SaveRegion(ctx context.Context, siteID int64, r Region) (int64, error) {
	if r.ID > 0 {
		_, err := s.DB.ExecContext(ctx, `UPDATE regions SET slug=$3, name=$4, province=$5, description=$6, partner=$7 WHERE id=$1 AND site_id=$2`,
			r.ID, siteID, r.Slug, r.Name, r.Province, r.Description, r.Partner)
		return r.ID, err
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO regions(site_id, slug, name, province, description, partner) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		siteID, r.Slug, r.Name, r.Province, r.Description, r.Partner).Scan(&id)
	return id, err
}

func (s *Store) SetRegionLogo(ctx context.Context, id int64, path string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE regions SET logo=$2 WHERE id=$1`, id, path)
	return err
}

func (s *Store) DeleteRegion(ctx context.Context, siteID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM regions WHERE id=$1 AND site_id=$2`, id, siteID)
	return err
}

// ---------- media ----------

type Media struct {
	ID       int64
	Path     string
	Width    int
	Height   int
	Variants map[string]string
	Credit   string
	Caption  string
}

func (m Media) Variant(w string) string {
	if v := m.Variants[w]; v != "" {
		return v
	}
	return m.Path
}

func (s *Store) CreateMedia(ctx context.Context, siteID int64, m Media, by int64) (int64, error) {
	v, _ := json.Marshal(m.Variants)
	var uid any
	if by > 0 {
		uid = by
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO media(site_id, path, width, height, variants, credit, caption, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		siteID, m.Path, m.Width, m.Height, v, m.Credit, m.Caption, uid).Scan(&id)
	return id, err
}

func (s *Store) GetMedia(ctx context.Context, id int64) (Media, error) {
	var m Media
	var v []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id, path, width, height, variants, credit, caption FROM media WHERE id=$1`, id).
		Scan(&m.ID, &m.Path, &m.Width, &m.Height, &v, &m.Credit, &m.Caption)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	_ = json.Unmarshal(v, &m.Variants)
	return m, err
}

func (s *Store) UpdateMediaText(ctx context.Context, id int64, credit, caption string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE media SET credit=$2, caption=$3 WHERE id=$1`, id, credit, caption)
	return err
}

// ---------- pages ----------

type Page struct {
	Slug      string
	Title     string
	BodyMD    string
	Position  int
	UpdatedAt time.Time
}

func (s *Store) Pages(ctx context.Context, siteID int64) ([]Page, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT slug, title, body_md, position, updated_at FROM pages WHERE site_id=$1 ORDER BY position, title`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.Slug, &p.Title, &p.BodyMD, &p.Position, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) PageBySlug(ctx context.Context, siteID int64, slug string) (Page, error) {
	var p Page
	err := s.DB.QueryRowContext(ctx, `SELECT slug, title, body_md, position, updated_at FROM pages WHERE site_id=$1 AND slug=$2`, siteID, slug).
		Scan(&p.Slug, &p.Title, &p.BodyMD, &p.Position, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Store) SavePage(ctx context.Context, siteID int64, p Page) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO pages(site_id, slug, title, body_md, position) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (site_id, slug) DO UPDATE SET title=EXCLUDED.title, body_md=EXCLUDED.body_md, updated_at=now()`,
		siteID, p.Slug, p.Title, p.BodyMD, p.Position)
	return err
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
