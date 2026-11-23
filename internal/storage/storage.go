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

type Store interface {
	CreateLink(ctx context.Context, originalURL, shortName string) (*Link, error)
	ListLinks(ctx context.Context) ([]Link, error)
	GetLinkByID(ctx context.Context, id int64) (*Link, error)
	GetLinkByShortName(ctx context.Context, shortName string) (*Link, error)
	UpdateLink(ctx context.Context, id int64, originalURL string, shortName OptionalString) (*Link, error)
	DeleteLink(ctx context.Context, id int64) (bool, error)
	AddVisit(ctx context.Context, linkID int64) error
	Close()
}
