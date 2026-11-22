package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"link-shortener/internal/storage"
)

// fakeStore — in-memory реализация storage.Store для тестов.
// map защищён мьютексом: хендлеры пишут визиты из горутин.
type fakeStore struct {
	mu     sync.Mutex
	links  map[string]storage.Link
	nextID int64
	visits int
}

func newFakeStore() *fakeStore {
	return &fakeStore{links: make(map[string]storage.Link), nextID: 1}
}

func (f *fakeStore) CreateLink(_ context.Context, originalURL, shortName string) (*storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.links[shortName]; exists {
		return nil, storage.ErrDuplicate
	}
	l := &storage.Link{
		ID:          f.nextID,
		OriginalURL: originalURL,
		ShortName:   shortName,
		CreatedAt:   time.Now(),
	}
	f.nextID++
	f.links[shortName] = *l
	return l, nil
}

func (f *fakeStore) GetLinkByShortName(_ context.Context, shortName string) (*storage.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.links[shortName]; ok {
		return &l, nil
	}
	return nil, storage.ErrNotFound
}

func (f *fakeStore) AddVisit(_ context.Context, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.visits++
	return nil
}

func (f *fakeStore) Close() {}

func TestPing(t *testing.T) {
	router := New(newFakeStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "pong", w.Body.String())
}

func TestCreateLink(t *testing.T) {
	router := New(newFakeStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/links",
		strings.NewReader(`{"original_url": "https://example.com/long"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"original_url":"https://example.com/long"`)
}

func TestCreateLinkInvalidURL(t *testing.T) {
	router := New(newFakeStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/links",
		strings.NewReader(`{"original_url": "not-a-url"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateLinkDuplicate(t *testing.T) {
	router := New(newFakeStore())

	body := `{"original_url": "https://example.com", "short_name": "taken"}`
	req := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), req) // первая — ок

	w := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req2)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRedirect(t *testing.T) {
	router := New(newFakeStore())

	// создаём ссылку
	req := httptest.NewRequest(http.MethodPost, "/api/links",
		strings.NewReader(`{"original_url": "https://example.com/long", "short_name": "abc"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), req)

	// переходим
	w := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/abc", nil)
	router.ServeHTTP(w, req2)

	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "https://example.com/long", w.Header().Get("Location"))
}

func TestRedirectNotFound(t *testing.T) {
	router := New(newFakeStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
