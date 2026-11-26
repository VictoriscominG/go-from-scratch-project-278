package storage

import (
	"context"
	"time"
)

type Link struct {
	ID          int64
	OriginalURL string
	ShortName   string
	CreatedAt   time.Time
}

type OptionalString struct {
	Value string
	Set   bool
}

type Visit struct {
	ID        int64     `json:"id"`
	LinkID    int64     `json:"link_id"`
	CreatedAt time.Time `json:"created_at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Referer   string    `json:"referer"`
	Status    int       `json:"status"`
}

type Store interface {
	CreateLink(ctx context.Context, originalURL, shortName string) (*Link, error)
	ListLinks(ctx context.Context, limit, offset int) ([]Link, error)
	CountLinks(ctx context.Context) (int64, error)
	GetLinkByID(ctx context.Context, id int64) (*Link, error)
	GetLinkByShortName(ctx context.Context, shortName string) (*Link, error)
	UpdateLink(ctx context.Context, id int64, originalURL string, shortName OptionalString) (*Link, error)
	DeleteLink(ctx context.Context, id int64) (bool, error)
	CreateVisit(ctx context.Context, v *Visit) error
	CountVisits(ctx context.Context) (int64, error)
	ListVisits(ctx context.Context, limit, offset int) ([]Visit, error)
	Close()
}
