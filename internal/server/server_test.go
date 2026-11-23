package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"link-shortener/internal/storage"
)

// fakeStore — реализация storage.Store в памяти для тестов сервера.
type fakeStore struct {
	mu     sync.Mutex
	nextID int64
	links  map[int64]storage.Link
}

func newFakeStore() *fakeStore {
	return &fakeStore{nextID: 1, links: make(map[int64]storage.Link)}
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

func (f *fakeStore) ListLinks(_ context.Context) ([]storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.Link, 0, len(f.links))
	for _, l := range f.links {
		out = append(out, l)
	}
	return out, nil
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

func (f *fakeStore) AddVisit(_ context.Context, _ int64) error { return nil }
func (f *fakeStore) Close()                                    {}

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
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("want 301, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/target" {
		t.Fatalf("unexpected Location: %s", loc)
	}
}

func TestNotFoundForUnknownShortName(t *testing.T) {
	r := newTestServer(t, "https://short.io")

	w := do(r, http.MethodGet, "/missing")
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
