package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"link-shortener/internal/storage"
)

// fakeStore — реализация storage.Store в памяти для тестов сервера.
type fakeStore struct {
	mu          sync.Mutex
	nextID      int64
	links       map[int64]storage.Link
	nextVisitID int64
	visits      []storage.Visit
}

func newFakeStore() *fakeStore {
	return &fakeStore{nextID: 1, nextVisitID: 1, links: make(map[int64]storage.Link)}
}

func (f *fakeStore) CreateLink(_ context.Context, originalURL, shortName string) (*storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.links {
		if l.ShortName == shortName {
			return nil, storage.ErrDuplicate
		}
	}
	l := storage.Link{ID: f.nextID, OriginalURL: originalURL, ShortName: shortName, CreatedAt: time.Now()}
	f.nextID++
	f.links[l.ID] = l
	return &l, nil
}

func (f *fakeStore) ListLinks(_ context.Context, limit, offset int) ([]storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := make([]storage.Link, 0, len(f.links))
	for _, l := range f.links {
		all = append(all, l)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	if limit < 0 {
		return all, nil
	}
	lo := min(offset, len(all))
	hi := min(offset+limit, len(all))
	return all[lo:hi], nil
}

func (f *fakeStore) CountLinks(_ context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.links)), nil
}

func (f *fakeStore) GetLinkByID(_ context.Context, id int64) (*storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return &l, nil
}

func (f *fakeStore) GetLinkByShortName(_ context.Context, shortName string) (*storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.links {
		if l.ShortName == shortName {
			cp := l
			return &cp, nil
		}
	}
	return nil, storage.ErrNotFound
}

func (f *fakeStore) UpdateLink(_ context.Context, id int64, originalURL string, shortName storage.OptionalString) (*storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	if shortName.Set {
		for _, other := range f.links {
			if other.ID != id && other.ShortName == shortName.Value {
				return nil, storage.ErrDuplicate
			}
		}
	}
	l.OriginalURL = originalURL
	if shortName.Set {
		l.ShortName = shortName.Value
	}
	f.links[id] = l
	return &l, nil
}

func (f *fakeStore) DeleteLink(_ context.Context, id int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.links[id]; !ok {
		return false, nil
	}
	delete(f.links, id)
	return true, nil
}

func (f *fakeStore) CreateVisit(_ context.Context, v *storage.Visit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v.ID = f.nextVisitID
	f.nextVisitID++
	v.CreatedAt = time.Now()
	f.visits = append(f.visits, *v)
	return nil
}

func (f *fakeStore) CountVisits(_ context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.visits)), nil
}

func (f *fakeStore) ListVisits(_ context.Context, limit, offset int) ([]storage.Visit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit < 0 {
		return append([]storage.Visit{}, f.visits...), nil
	}
	lo := min(offset, len(f.visits))
	hi := min(offset+limit, len(f.visits))
	return append([]storage.Visit{}, f.visits[lo:hi]...), nil
}

func (f *fakeStore) Close() {}

func newTestServer(t *testing.T, baseURL string) *gin.Engine {
	t.Helper()
	return New(newFakeStore(), baseURL)
}

func do(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPing(t *testing.T) {
	r := newTestServer(t, "https://short.io")

	w := do(r, http.MethodGet, "/ping")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if w.Body.String() != "pong" {
		t.Fatalf("want pong, got %q", w.Body.String())
	}
}

func TestRedirectThroughServer(t *testing.T) {
	store := newFakeStore()
	if _, err := store.CreateLink(context.Background(), "https://example.com/target", "go"); err != nil {
		t.Fatal(err)
	}
	r := New(store, "https://short.io")

	w := do(r, http.MethodGet, "/r/go")
	if w.Code != http.StatusFound { // теперь 302
		t.Fatalf("want 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/target" {
		t.Fatalf("unexpected Location: %s", loc)
	}

	// Бонус: через серверный тест проверим, что визит записался.
	w = do(r, http.MethodGet, "/api/link_visits")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var visits []storage.Visit
	if err := json.Unmarshal(w.Body.Bytes(), &visits); err != nil {
		t.Fatal(err)
	}
	if len(visits) != 1 {
		t.Fatalf("want 1 visit, got %d", len(visits))
	}
	if visits[0].LinkID != 1 {
		t.Fatalf("want link_id=1, got %d", visits[0].LinkID)
	}
}

func TestNotFoundForUnknownShortName(t *testing.T) {
	r := newTestServer(t, "https://short.io")

	w := do(r, http.MethodGet, "/r/missing")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestDebugSentryRoute(t *testing.T) {
	t.Run("enabled when SENTRY_DEBUG=on", func(t *testing.T) {
		t.Setenv("SENTRY_DEBUG", "on")
		r := New(newFakeStore(), "https://short.io")

		w := do(r, http.MethodGet, "/debug/sentry")
		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", w.Code)
		}
		if w.Body.String() != "sent" {
			t.Fatalf("want sent, got %q", w.Body.String())
		}
	})

	t.Run("disabled without SENTRY_DEBUG", func(t *testing.T) {
		t.Setenv("SENTRY_DEBUG", "")
		r := New(newFakeStore(), "https://short.io")

		w := do(r, http.MethodGet, "/debug/sentry")
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", w.Code)
		}
	})
}
