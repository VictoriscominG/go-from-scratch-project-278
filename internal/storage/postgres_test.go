package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"link-shortener/internal/storage"
)

const migrationsDir = "../../db/migrations"

var nameSeq atomic.Uint64

func uniqueShortName(prefix string) string {
	return fmt.Sprintf("%s%d_%d", prefix, time.Now().UnixNano(), nameSeq.Add(1))
}

func newTestStore(t *testing.T) storage.Store {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	store, err := storage.NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCreateAndGetByShortName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	name := uniqueShortName("t")

	created, err := store.CreateLink(ctx, "https://example.com/page", name)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created link has zero id")
	}
	if created.OriginalURL != "https://example.com/page" || created.ShortName != name {
		t.Fatalf("unexpected link: %+v", created)
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("created_at is zero")
	}

	got, err := store.GetLinkByShortName(ctx, name)
	if err != nil {
		t.Fatalf("get by short name: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("want id %d, got %d", created.ID, got.ID)
	}
}

func TestCreateDuplicateShortName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	name := uniqueShortName("dup")

	if _, err := store.CreateLink(ctx, "https://a.io", name); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := store.CreateLink(ctx, "https://b.io", name); !errors.Is(err, storage.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestGetLinkByIDNotFound(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.GetLinkByID(context.Background(), 999999); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateLink(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateLink(ctx, "https://old.io", uniqueShortName("upd"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Смена short_name.
	newName := uniqueShortName("upd")
	updated, err := store.UpdateLink(ctx, created.ID, "https://new.io",
		storage.OptionalString{Value: newName, Set: true})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.OriginalURL != "https://new.io" || updated.ShortName != newName {
		t.Fatalf("unexpected updated link: %+v", updated)
	}

	// Короткое имя не задано — остаётся прежним.
	kept, err := store.UpdateLink(ctx, created.ID, "https://new2.io",
		storage.OptionalString{Value: "", Set: false})
	if err != nil {
		t.Fatalf("update without short name: %v", err)
	}
	if kept.ShortName != newName {
		t.Fatalf("short name changed unexpectedly: %q", kept.ShortName)
	}

	// Чужой short_name → ErrDuplicate.
	other := uniqueShortName("busy")
	if _, err := store.CreateLink(ctx, "https://c.io", other); err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := store.UpdateLink(ctx, created.ID, "https://d.io",
		storage.OptionalString{Value: other, Set: true}); !errors.Is(err, storage.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}

	// Несуществующий id → ErrNotFound.
	if _, err := store.UpdateLink(ctx, 999999, "https://x.io",
		storage.OptionalString{Value: "", Set: false}); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteLink(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateLink(ctx, "https://del.io", uniqueShortName("del"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	deleted, err := store.DeleteLink(ctx, created.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !deleted {
		t.Fatal("delete returned false for existing link")
	}

	// Повторное удаление: строки больше нет.
	deletedAgain, err := store.DeleteLink(ctx, created.ID)
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if deletedAgain {
		t.Fatal("second delete returned true for missing link")
	}

	if _, err := store.GetLinkByID(ctx, created.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestCreateVisitAndListVisits(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	created, err := store.CreateLink(ctx, "https://v.io", uniqueShortName("vis"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	before, err := store.CountVisits(ctx)
	if err != nil {
		t.Fatalf("count visits: %v", err)
	}

	v1 := &storage.Visit{LinkID: created.ID}
	if err := store.CreateVisit(ctx, v1); err != nil {
		t.Fatalf("create visit: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	v2 := &storage.Visit{LinkID: created.ID}
	if err := store.CreateVisit(ctx, v2); err != nil {
		t.Fatalf("create visit: %v", err)
	}

	after, err := store.CountVisits(ctx)
	if err != nil {
		t.Fatalf("count visits: %v", err)
	}
	if after-before < 2 {
		t.Fatalf("want at least 2 new visits, got %d", after-before)
	}

	visits, err := store.ListVisits(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list visits: %v", err)
	}
	if len(visits) < 2 {
		t.Fatalf("want at least 2 visits, got %d", len(visits))
	}
}

func TestListLinksAndCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	before, err := store.CountLinks(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	n1 := uniqueShortName("list")
	n2 := uniqueShortName("list")
	created1, err := store.CreateLink(ctx, "https://1.io", n1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created2, err := store.CreateLink(ctx, "https://2.io", n2)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created2.ID <= created1.ID { // дополнительная проверка: ID растут
		t.Fatalf("want created2.ID > created1.ID, got %d and %d", created2.ID, created1.ID)
	}

	after, err := store.CountLinks(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if after-before != 2 {
		t.Fatalf("want +2 links, got %d", after-before)
	}

	page, err := store.ListLinks(ctx, 1, int(before))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("want page of 1, got %d", len(page))
	}
	if page[0].ID != created1.ID {
		t.Fatalf("page ordering broken: first is id=%d (%q), want id=%d (%q)",
			page[0].ID, page[0].ShortName, created1.ID, created1.ShortName)
	}
}
