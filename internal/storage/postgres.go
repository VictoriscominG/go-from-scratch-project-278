package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicate = errors.New("short_name already exists")
	ErrNotFound  = errors.New("link not found")
)

type Postgres struct {
	pool *pgxpool.Pool
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
	return &Postgres{pool: pool}, nil
}

// isUniqueViolation проверяет, что ошибка от Postgres — нарушение
// уникального ограничения (код 23505), например дубликат short_name.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	return false
}

func (p *Postgres) CreateLink(ctx context.Context, originalURL, shortName string) (*Link, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var l Link
	err := p.pool.QueryRow(ctx,
		`INSERT INTO links (original_url, short_name) VALUES ($1, $2)
		 RETURNING id, original_url, short_name, created_at`,
		originalURL, shortName,
	).Scan(&l.ID, &l.OriginalURL, &l.ShortName, &l.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return &l, nil
}

func (p *Postgres) GetLinkByShortName(ctx context.Context, shortName string) (*Link, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var l Link
	err := p.pool.QueryRow(ctx,
		`SELECT id, original_url, short_name, created_at
		 FROM links WHERE short_name = $1`,
		shortName,
	).Scan(&l.ID, &l.OriginalURL, &l.ShortName, &l.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &l, nil
}

func (p *Postgres) AddVisit(ctx context.Context, linkID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := p.pool.Exec(ctx,
		`INSERT INTO visits (link_id) VALUES ($1)`,
		linkID,
	)
	return err
}

func (p *Postgres) Close() {
	p.pool.Close()
}
