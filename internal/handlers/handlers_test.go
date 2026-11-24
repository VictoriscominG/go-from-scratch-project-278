package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"link-shortener/internal/storage"
)

// fakeStore — полная реализация storage.Store в памяти.
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

func (f *fakeStore) AddVisit(_ context.Context, _ int64) error { return nil }
func (f *fakeStore) Close()                                    {}

func setup(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := New(newFakeStore(), "https://short.io")
	r := gin.New()
	h.Register(r)
	return r
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func id(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestCreateLink(t *testing.T) {
	r := setup(t)

	w := do(r, http.MethodPost, "/api/links", `{"original_url": "https://example.com/x"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body)
	}
	var resp linkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ShortName == "" || resp.ShortURL == "" {
		t.Fatalf("short_name/short_url must be filled: %+v", resp)
	}
	if resp.ShortURL != "https://short.io/r/"+resp.ShortName {
		t.Fatalf("short_url mismatch: %s", resp.ShortURL)
	}
}

func TestCreateLinkWithCustomShortName(t *testing.T) {
	r := setup(t)

	w := do(r, http.MethodPost, "/api/links", `{"original_url": "https://example.com/x", "short_name": "mylink"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body)
	}
	var resp linkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ShortName != "mylink" {
		t.Fatalf("want short_name=mylink, got %s", resp.ShortName)
	}
}

func TestCreateLinkValidation(t *testing.T) {
	r := setup(t)

	cases := []struct {
		body string
		want int
	}{
		{`{}`, http.StatusBadRequest},                            // нет original_url
		{`{"original_url": "not-a-url"}`, http.StatusBadRequest}, // невалидный URL
		{`not json`, http.StatusBadRequest},                      // битый JSON
	}
	for _, tc := range cases {
		if w := do(r, http.MethodPost, "/api/links", tc.body); w.Code != tc.want {
			t.Errorf("body %q: want %d, got %d", tc.body, tc.want, w.Code)
		}
	}
}

func TestCreateDuplicateShortName(t *testing.T) {
	r := setup(t)
	do(r, http.MethodPost, "/api/links", `{"original_url": "https://a.io", "short_name": "abc"}`)
	w := do(r, http.MethodPost, "/api/links", `{"original_url": "https://b.io", "short_name": "abc"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

func TestListLinks(t *testing.T) {
	r := setup(t)
	do(r, http.MethodPost, "/api/links", `{"original_url": "https://a.io"}`)
	do(r, http.MethodPost, "/api/links", `{"original_url": "https://b.io"}`)

	w := do(r, http.MethodGet, "/api/links", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp []linkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 2 {
		t.Fatalf("want 2 links, got %d", len(resp))
	}
	for _, l := range resp {
		if l.ShortURL == "" {
			t.Fatalf("short_url must be filled: %+v", l)
		}
	}
}

func TestListLinksEmpty(t *testing.T) {
	r := setup(t)

	w := do(r, http.MethodGet, "/api/links", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp []linkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 0 {
		t.Fatalf("want empty list, got %d items", len(resp))
	}
}

func TestGetLinkByID(t *testing.T) {
	r := setup(t)
	w := do(r, http.MethodPost, "/api/links", `{"original_url": "https://a.io"}`)
	var created linkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	if w := do(r, http.MethodGet, "/api/links/"+id(created.ID), ""); w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/links/999", ""); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	if w := do(r, http.MethodGet, "/api/links/abc", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestUpdateLink(t *testing.T) {
	r := setup(t)
	w := do(r, http.MethodPost, "/api/links", `{"original_url": "https://a.io", "short_name": "old"}`)
	var created linkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	// Обновляем URL, short_name не трогаем.
	w = do(r, http.MethodPut, "/api/links/"+id(created.ID), `{"original_url": "https://new.io"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var updated linkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.OriginalURL != "https://new.io" || updated.ShortName != "old" {
		t.Fatalf("unexpected update: %+v", updated)
	}

	// Обновляем и short_name тоже.
	w = do(r, http.MethodPut, "/api/links/"+id(created.ID), `{"original_url": "https://new.io", "short_name": "new"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.ShortName != "new" || updated.ShortURL != "https://short.io/r/new" {
		t.Fatalf("unexpected update: %+v", updated)
	}

	// Несуществующий id → 404.
	if w := do(r, http.MethodPut, "/api/links/999", `{"original_url": "https://n.io"}`); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}

	// Чужой short_name → 409.
	do(r, http.MethodPost, "/api/links", `{"original_url": "https://c.io", "short_name": "taken"}`)
	if w := do(r, http.MethodPut, "/api/links/"+id(created.ID), `{"original_url": "https://n.io", "short_name": "taken"}`); w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

func TestDeleteLink(t *testing.T) {
	r := setup(t)
	w := do(r, http.MethodPost, "/api/links", `{"original_url": "https://a.io"}`)
	var created linkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	if w := do(r, http.MethodDelete, "/api/links/"+id(created.ID), ""); w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	// Повторное удаление → 404.
	if w := do(r, http.MethodDelete, "/api/links/"+id(created.ID), ""); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	// После удаления GET тоже даёт 404.
	if w := do(r, http.MethodGet, "/api/links/"+id(created.ID), ""); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestRedirect(t *testing.T) {
	r := setup(t)
	do(r, http.MethodPost, "/api/links", `{"original_url": "https://a.io/target", "short_name": "go"}`)

	req := httptest.NewRequest(http.MethodGet, "/r/go", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("want 301, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://a.io/target" {
		t.Fatalf("unexpected Location: %s", loc)
	}

	if w := do(r, http.MethodGet, "/r/missing", ""); w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func createN(t *testing.T, r *gin.Engine, n int) []linkResponse {
	t.Helper()
	out := make([]linkResponse, 0, n)
	for i := 0; i < n; i++ {
		w := do(r, http.MethodPost, "/api/links",
			fmt.Sprintf(`{"original_url": "https://ex.io/%d"}`, i))
		if w.Code != http.StatusCreated {
			t.Fatalf("create %d failed: %d %s", i, w.Code, w.Body)
		}
		var resp linkResponse
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		out = append(out, resp)
	}
	return out
}

func TestListLinksPagination(t *testing.T) {
	r := setup(t)
	createN(t, r, 12)

	w := do(r, http.MethodGet, "/api/links?range=[0,10]", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if cr := w.Header().Get("Content-Range"); cr != "links 0-10/12" {
		t.Fatalf("unexpected Content-Range: %q", cr)
	}
	var resp []linkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 10 {
		t.Fatalf("want 10 links, got %d", len(resp))
	}
	if resp[0].ID != 1 || resp[9].ID != 10 {
		t.Fatalf("unexpected first page: first ID=%d, last ID=%d", resp[0].ID, resp[9].ID)
	}

	w = do(r, http.MethodGet, "/api/links?range=%5B5,%2010%5D", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if cr := w.Header().Get("Content-Range"); cr != "links 5-10/12" {
		t.Fatalf("unexpected Content-Range: %q", cr)
	}
	resp = nil
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp) != 5 {
		t.Fatalf("want 5 links, got %d", len(resp))
	}
	if resp[0].ID != 6 || resp[4].ID != 10 {
		t.Fatalf("unexpected second page: first ID=%d, last ID=%d", resp[0].ID, resp[4].ID)
	}
}

func TestListLinksRangeClampedToTotal(t *testing.T) {
	r := setup(t)
	createN(t, r, 5)

	w := do(r, http.MethodGet, "/api/links?range=[0,10]", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if cr := w.Header().Get("Content-Range"); cr != "links 0-5/5" {
		t.Fatalf("unexpected Content-Range: %q", cr)
	}
	var resp []linkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 5 {
		t.Fatalf("want 5 links, got %d", len(resp))
	}
}

func TestListLinksInvalidRange(t *testing.T) {
	r := setup(t)
	createN(t, r, 3)

	cases := []struct {
		name string
		q    string
	}{
		{"end <= start", "range=%5B10,5%5D"},
		{"negative start", "range=%5B-1,5%5D"},
		{"single value", "range=%5B5%5D"},
		{"not numbers", "range=%5Ba,b%5D"},
		{"three values", "range=%5B5,10,15%5D"},
	}
	for _, tc := range cases {
		if w := do(r, http.MethodGet, "/api/links?"+tc.q, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", tc.name, w.Code)
		}
	}
}

func TestListLinksPageSizeCapped(t *testing.T) {
	r := setup(t)
	createN(t, r, 10)

	w := do(r, http.MethodGet, "/api/links?range=[0,500]", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var resp []linkResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) > maxPageSize {
		t.Fatalf("page size not capped: %d", len(resp))
	}
}
