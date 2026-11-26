package handlers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"link-shortener/internal/storage"
)

const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const shortNameLength = 6
const errInternal = "internal error"

type Handlers struct {
	store   storage.Store
	baseURL string
}

func New(store storage.Store, baseURL string) *Handlers {
	return &Handlers{store: store, baseURL: strings.TrimRight(baseURL, "/")}
}

func (h *Handlers) Register(r *gin.Engine) {
	r.POST("/api/links", h.CreateLink)
	r.GET("/api/links", h.ListLinks)
	r.GET("/api/links/:id", h.GetLink)
	r.PUT("/api/links/:id", h.UpdateLink)
	r.DELETE("/api/links/:id", h.DeleteLink)
	r.GET("/api/link_visits", h.ListVisits) // ← новый
	r.GET("/r/:short_name", h.Redirect)
}

// linkResponse — формат ответа API с полем short_url.
type linkResponse struct {
	ID          int64     `json:"id"`
	OriginalURL string    `json:"original_url"`
	ShortName   string    `json:"short_name"`
	ShortURL    string    `json:"short_url"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *Handlers) respond(link *storage.Link) linkResponse {
	return linkResponse{
		ID:          link.ID,
		OriginalURL: link.OriginalURL,
		ShortName:   link.ShortName,
		ShortURL:    h.baseURL + "/r/" + link.ShortName,
		CreatedAt:   link.CreatedAt,
	}
}

func (h *Handlers) respondList(links []storage.Link) []linkResponse {
	out := make([]linkResponse, 0, len(links))
	for i := range links {
		out = append(out, h.respond(&links[i]))
	}
	return out
}

type createLinkRequest struct {
	OriginalURL string `json:"original_url"`
	ShortName   string `json:"short_name"`
}

// CreateLink — POST /api/links.
func (h *Handlers) CreateLink(c *gin.Context) {
	var req createLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	u, err := url.Parse(req.OriginalURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}

	shortName := req.ShortName
	if shortName == "" {
		sn, err := generateShortName(shortNameLength)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
			return
		}
		shortName = sn
	}

	link, err := h.store.CreateLink(c.Request.Context(), req.OriginalURL, shortName)
	if errors.Is(err, storage.ErrDuplicate) {
		c.JSON(http.StatusConflict, gin.H{"error": "short_name already exists"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	c.JSON(http.StatusCreated, h.respond(link))
}

// maxPageSize — верхняя граница размера страницы, чтобы клиент не выкачивал
// всю таблицу одним запросом.
const maxPageSize = 100

// ListLinks — GET /api/links. Опционально принимает ?range=[start,end]:
// OFFSET = start, LIMIT = end - start. Возвращает заголовок Content-Range.
func (h *Handlers) ListLinks(c *gin.Context) {
	total, err := h.store.CountLinks(c.Request.Context())
	if err != nil {
		log.Printf("count links: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	limit, offset := -1, 0 // по умолчанию — все записи
	rawRange := c.Query("range")
	if rawRange != "" {
		start, end, err := parseRange(rawRange)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if end-start > maxPageSize {
			end = start + maxPageSize
		}
		limit, offset = int(end-start), int(start)
	}

	links, err := h.store.ListLinks(c.Request.Context(), limit, offset)
	if err != nil {
		log.Printf("list links: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	// Конец диапазона для заголовка: запрошенный end либо фактический
	// (если записей меньше, чем просили).
	rangeEnd := offset + limit
	if limit < 0 || rangeEnd > int(total) {
		rangeEnd = int(total)
	}
	c.Header("Content-Range", fmt.Sprintf("links %d-%d/%d", offset, rangeEnd, total))

	c.JSON(http.StatusOK, h.respondList(links))
}

func (h *Handlers) ListVisits(c *gin.Context) {
	total, err := h.store.CountVisits(c.Request.Context())
	if err != nil {
		log.Printf("count visits: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	limit, offset := -1, 0
	rawRange := c.Query("range")
	if rawRange != "" {
		start, end, err := parseRange(rawRange)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if end-start > maxPageSize {
			end = start + maxPageSize
		}
		limit, offset = int(end-start), int(start)
	}

	visits, err := h.store.ListVisits(c.Request.Context(), limit, offset)
	if err != nil {
		log.Printf("list visits: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	rangeEnd := offset + limit
	if limit < 0 || rangeEnd > int(total) {
		rangeEnd = int(total)
	}
	c.Header("Content-Range", fmt.Sprintf("link_visits %d-%d/%d", offset, rangeEnd, total))

	c.JSON(http.StatusOK, visits)
}

// GetLink — GET /api/links/:id.
func (h *Handlers) GetLink(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}

	link, err := h.store.GetLinkByID(c.Request.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	c.JSON(http.StatusOK, h.respond(link))
}

// UpdateLink — PUT /api/links/:id.
// original_url обязателен; short_name можно не передавать — тогда он не меняется.
func (h *Handlers) UpdateLink(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}

	var req createLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	u, err := url.Parse(req.OriginalURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}

	link, err := h.store.UpdateLink(c.Request.Context(), id,
		req.OriginalURL, storage.OptionalString{Value: req.ShortName, Set: req.ShortName != ""})
	switch {
	case errors.Is(err, storage.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
	case errors.Is(err, storage.ErrDuplicate):
		c.JSON(http.StatusConflict, gin.H{"error": "short_name already exists"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
	default:
		c.JSON(http.StatusOK, h.respond(link))
	}
}

// DeleteLink — DELETE /api/links/:id.
func (h *Handlers) DeleteLink(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}

	deleted, err := h.store.DeleteLink(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) Redirect(c *gin.Context) {
	link, err := h.store.GetLinkByShortName(c.Request.Context(), c.Param("short_name"))
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	if err != nil {
		log.Printf("get link by short_name: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	status := http.StatusFound // 302
	visit := storage.Visit{
		LinkID:    link.ID,
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Referer:   c.GetHeader("Referer"),
		Status:    status,
	}
	// Записываем визит до редиректа: после c.Redirect ответ уже уходит клиенту.
	if err := h.store.CreateVisit(c.Request.Context(), &visit); err != nil {
		log.Printf("create visit: %v", err) // сбой аналитики не ломает редирект
	}

	c.Redirect(status, link.OriginalURL)
}

func (h *Handlers) parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

func generateShortName(n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b), nil
}

// parseRange разбирает параметр вида "[5, 10]" (скобки и пробелы допустимы).
func parseRange(s string) (start, end int64, err error) {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.ReplaceAll(s, " ", "")

	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, errors.New("range must contain two values")
	}
	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid range start")
	}
	end, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, errors.New("invalid range end")
	}
	if start < 0 || end <= start {
		return 0, 0, errors.New("range must satisfy 0 <= start < end")
	}
	return start, end, nil
}
