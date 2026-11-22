package handlers

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"link-shortener/internal/storage"
)

// alphabet — символы для генерации коротких имён (base62).
const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// shortNameLength — длина генерируемого короткого имени.
const shortNameLength = 6

const errInternal = "internal error"

// createLinkRequest — тело запроса POST /api/links.
type createLinkRequest struct {
	OriginalURL string `json:"original_url"`
	ShortName   string `json:"short_name"`
}

// Handlers хранит зависимости хендлеров.
type Handlers struct {
	store storage.Store
}

// New собирает Handlers с хранилищем.
func New(store storage.Store) *Handlers {
	return &Handlers{store: store}
}

// Register регистрирует маршруты API.
func (h *Handlers) Register(r *gin.Engine) {
	r.POST("/api/links", h.CreateLink)
	r.GET("/:short_name", h.Redirect)
}

// CreateLink — POST /api/links.
// Принимает JSON с original_url (обязательно) и short_name (опционально,
// если не задан — генерируется случайно). Возвращает созданную ссылку.
func (h *Handlers) CreateLink(c *gin.Context) {
	var req createLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	if req.OriginalURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "original_url is required"})
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

	c.JSON(http.StatusCreated, gin.H{
		"id":           link.ID,
		"original_url": link.OriginalURL,
		"short_name":   link.ShortName,
		"created_at":   link.CreatedAt,
	})
}

// Redirect — GET /:short_name.
// Ищет ссылку по короткому имени и перенаправляет на оригинальный адрес.
// Визит записывается асинхронно, чтобы не замедлять редирект.
func (h *Handlers) Redirect(c *gin.Context) {
	shortName := c.Param("short_name")

	link, err := h.store.GetLinkByShortName(c.Request.Context(), shortName)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	// Пишем визит в горутине: редирект не должен ждать запись в БД.
	go func(linkID int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.store.AddVisit(ctx, linkID)
	}(link.ID)

	c.Redirect(http.StatusMovedPermanently, link.OriginalURL)
}

// generateShortName создаёт случайную строку длины n из base62-алфавита
// с криптографическим генератором случайности.
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
