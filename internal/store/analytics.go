package store

import (
	"context"
	"time"

	"github.com/lib/pq"
)

// RecordView counts one page view; uniques are counted once per visitor hash per day.
func (s *Store) RecordView(ctx context.Context, siteID, articleID int64, day time.Time, source, visitor string) error {
	d := day.Format("2006-01-02")
	res, err := s.DB.ExecContext(ctx, `INSERT INTO pageview_seen(day, article_id, visitor) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, d, articleID, visitor)
	if err != nil {
		return err
	}
	uniq := 0
	if n, _ := res.RowsAffected(); n > 0 {
		uniq = 1
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO pageviews_daily(site_id, article_id, day, source, views, uniques) VALUES ($1,$2,$3,$4,1,$5)
		ON CONFLICT (site_id, article_id, day, source) DO UPDATE SET views=pageviews_daily.views+1, uniques=pageviews_daily.uniques+EXCLUDED.uniques`,
		siteID, articleID, d, source, uniq)
	return err
}

// PruneSeen drops visitor hashes older than two days (only needed for same-day uniques).
func (s *Store) PruneSeen(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM pageview_seen WHERE day < current_date - 2`)
	return err
}

type DayStat struct {
	Day     time.Time
	Views   int
	Uniques int
}

func (s *Store) DailyStats(ctx context.Context, siteID int64, days int) ([]DayStat, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT d::date, COALESCE(sum(p.views),0), COALESCE(sum(p.uniques),0)
		FROM generate_series(current_date - ($2::int - 1), current_date, '1 day') d
		LEFT JOIN pageviews_daily p ON p.day=d::date AND p.site_id=$1
		GROUP BY d ORDER BY d`, siteID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayStat
	for rows.Next() {
		var x DayStat
		if err := rows.Scan(&x.Day, &x.Views, &x.Uniques); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

type NamedStat struct {
	Key, Name string
	Views     int
	Uniques   int
}

func (s *Store) SourceStats(ctx context.Context, siteID int64, days int) ([]NamedStat, error) {
	return s.named(ctx, `SELECT source, source, sum(views), sum(uniques) FROM pageviews_daily WHERE site_id=$1 AND day > current_date - $2::int
		GROUP BY source ORDER BY 3 DESC`, siteID, days)
}

func (s *Store) RegionStats(ctx context.Context, siteID int64, days int) ([]NamedStat, error) {
	return s.named(ctx, `SELECT r.slug, r.name, sum(p.views), sum(p.uniques) FROM pageviews_daily p JOIN articles a ON a.id=p.article_id
		JOIN regions r ON r.id=a.region_id WHERE p.site_id=$1 AND p.day > current_date - $2::int GROUP BY r.slug, r.name ORDER BY 3 DESC`, siteID, days)
}

func (s *Store) CategoryStats(ctx context.Context, siteID int64, days int) ([]NamedStat, error) {
	return s.named(ctx, `SELECT c.slug, c.name, sum(p.views), sum(p.uniques) FROM pageviews_daily p JOIN articles a ON a.id=p.article_id
		JOIN categories c ON c.id=a.category_id WHERE p.site_id=$1 AND p.day > current_date - $2::int GROUP BY c.slug, c.name ORDER BY 3 DESC`, siteID, days)
}

func (s *Store) named(ctx context.Context, q string, args ...any) ([]NamedStat, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NamedStat
	for rows.Next() {
		var x NamedStat
		if err := rows.Scan(&x.Key, &x.Name, &x.Views, &x.Uniques); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// TopArticles lists articles by views in the window (with view counts).
func (s *Store) TopArticles(ctx context.Context, siteID int64, days, limit int) ([]Article, error) {
	rows, err := s.DB.QueryContext(ctx, articleSelect+`
		JOIN (SELECT article_id, sum(views) v FROM pageviews_daily WHERE site_id=$1 AND article_id>0 AND day > current_date - $2::int GROUP BY article_id) pv
		ON pv.article_id=a.id ORDER BY pv.v DESC LIMIT $3`, siteID, days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Article
	var ids []int64
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
		ids = append(ids, a.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	views, err := s.ArticleViews(ctx, siteID, ids, days)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Views = views[out[i].ID]
	}
	return out, nil
}

// ArticleViews returns total views per article id over the window (days<=0 = all time).
func (s *Store) ArticleViews(ctx context.Context, siteID int64, ids []int64, days int) (map[int64]int, error) {
	out := map[int64]int{}
	if len(ids) == 0 {
		return out, nil
	}
	q := `SELECT article_id, sum(views) FROM pageviews_daily WHERE site_id=$1 AND article_id = ANY($2)`
	args := []any{siteID, int64Array(ids)}
	if days > 0 {
		q += ` AND day > current_date - $3::int`
		args = append(args, days)
	}
	rows, err := s.DB.QueryContext(ctx, q+` GROUP BY article_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func int64Array(ids []int64) any { return pq.Array(ids) }
