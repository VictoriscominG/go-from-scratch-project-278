package handlers

import (
	"context"
	"crypto/rand"
	"errors"
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

// ListLinks — GET /api/links.
func (h *Handlers) ListLinks(c *gin.Context) {
	links, err := h.store.ListLinks(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}
	c.JSON(http.StatusOK, h.respondList(links))
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

// Redirect — GET /:short_name.
func (h *Handlers) Redirect(c *gin.Context) {
	link, err := h.store.GetLinkByShortName(c.Request.Context(), c.Param("short_name"))
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	linkID := link.ID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.store.AddVisit(ctx, linkID)
	}()

	c.Redirect(http.StatusMovedPermanently, link.OriginalURL)
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
