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

type Store interface {
	CreateLink(ctx context.Context, originalURL, shortName string) (*Link, error)
	GetLinkByShortName(ctx context.Context, shortName string) (*Link, error)
	AddVisit(ctx context.Context, linkID int64) error
	Close()
}
