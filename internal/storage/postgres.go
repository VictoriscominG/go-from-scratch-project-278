package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"link-shortener/internal/storage/sqlc"
)

var (
	ErrDuplicate = errors.New("short_name already exists")
	ErrNotFound  = errors.New("link not found")
)

type Postgres struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool, q: sqlc.New(pool)}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func toLink(row sqlc.Link) Link {
	return Link{
		ID:          row.ID,
		OriginalURL: row.OriginalURL,
		ShortName:   row.ShortName,
		CreatedAt:   row.CreatedAt,
	}
}

func (p *Postgres) CreateLink(ctx context.Context, originalURL, shortName string) (*Link, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	row, err := p.q.CreateLink(ctx, sqlc.CreateLinkParams{
		OriginalURL: originalURL,
		ShortName:   shortName,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	l := toLink(row)
	return &l, nil
}

func (p *Postgres) ListLinks(ctx context.Context, limit, offset int) ([]Link, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := p.q.ListLinksPage(ctx, sqlc.ListLinksPageParams{
		PageSize:  int32(limit),
		PageStart: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Link, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLink(r))
	}
	return out, nil
}

func (p *Postgres) CountLinks(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return p.q.CountLinks(ctx)
}

func (p *Postgres) GetLinkByID(ctx context.Context, id int64) (*Link, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	row, err := p.q.GetLinkByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l := toLink(row)
	return &l, nil
}

func (p *Postgres) GetLinkByShortName(ctx context.Context, shortName string) (*Link, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	row, err := p.q.GetLinkByShortName(ctx, shortName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l := toLink(row)
	return &l, nil
}

func (p *Postgres) UpdateLink(ctx context.Context, id int64, originalURL string, shortName OptionalString) (*Link, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	current, err := p.q.GetLinkByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	newShort := current.ShortName
	if shortName.Set {
		newShort = shortName.Value
	}

	row, err := p.q.UpdateLink(ctx, sqlc.UpdateLinkParams{
		ID:          id,
		OriginalURL: originalURL,
		ShortName:   newShort,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	l := toLink(row)
	return &l, nil
}

func (p *Postgres) DeleteLink(ctx context.Context, id int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := p.q.DeleteLink(ctx, id)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (p *Postgres) CreateVisit(ctx context.Context, v *Visit) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	row, err := p.q.AddVisit(ctx, sqlc.AddVisitParams{
		LinkID:    v.LinkID,
		Ip:        v.IP,
		UserAgent: v.UserAgent,
		Referer:   v.Referer,
		Status:    int32(v.Status),
	})
	if err != nil {
		return err
	}
	v.ID = row.ID
	v.CreatedAt = row.CreatedAt
	return nil
}

func (p *Postgres) CountVisits(ctx context.Context) (int64, error) {
	var n int64
	err := p.pool.QueryRow(ctx, `SELECT count(*) FROM link_visits`).Scan(&n)
	return n, err
}

func (p *Postgres) ListVisits(ctx context.Context, limit, offset int) ([]Visit, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, link_id, created_at, ip, user_agent, referer, status
		 FROM link_visits ORDER BY id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	visits := make([]Visit, 0, limit)
	for rows.Next() {
		var v Visit
		if err := rows.Scan(&v.ID, &v.LinkID, &v.CreatedAt,
			&v.IP, &v.UserAgent, &v.Referer, &v.Status); err != nil {
			return nil, err
		}
		visits = append(visits, v)
	}
	return visits, rows.Err()
}

func (p *Postgres) Close() {
	p.pool.Close()
}
