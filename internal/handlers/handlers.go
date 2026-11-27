package handlers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

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
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
	}
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

type createLinkPayload struct {
	OriginalURL string `json:"original_url" binding:"required,url"`
	ShortName   string `json:"short_name" binding:"omitempty,min=3,max=32"`
}

// CreateLink — POST /api/links.
func (h *Handlers) CreateLink(c *gin.Context) {
	var payload createLinkPayload
	if !bindPayload(c, &payload) {
		return
	}

	if !h.validateURL(c, payload.OriginalURL) {
		return
	}

	shortName, ok := resolveShortName(c, payload.ShortName)
	if !ok {
		return
	}

	link, err := h.store.CreateLink(c.Request.Context(), payload.OriginalURL, shortName)
	if errors.Is(err, storage.ErrDuplicate) {
		respondShortNameTaken(c)
		return
	}

	if err != nil {
		log.Printf("create link: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
		return
	}

	c.JSON(http.StatusCreated, h.respond(link))
}

// maxPageSize — верхняя граница размера страницы, чтобы клиент не выкачивал
// всю таблицу одним запросом.
const maxPageSize = 100

func (h *Handlers) ListLinks(c *gin.Context) {
	total, err := h.store.CountLinks(c.Request.Context())
	if err != nil {
		log.Printf("count links: %v", err)
		respondInternal(c)
		return
	}

	limit, offset, ok := resolvePaging(c, total)
	if !ok {
		return
	}

	links, err := h.store.ListLinks(c.Request.Context(), limit, offset)
	if err != nil {
		log.Printf("list links: %v", err)
		respondInternal(c)
		return
	}

	setContentRange(c, "links", offset, limit, int(total))
	c.JSON(http.StatusOK, h.respondList(links))
}

func (h *Handlers) ListVisits(c *gin.Context) {
	total, err := h.store.CountVisits(c.Request.Context())
	if err != nil {
		log.Printf("count visits: %v", err)
		respondInternal(c)
		return
	}

	limit, offset, ok := resolvePaging(c, total)
	if !ok {
		return
	}

	visits, err := h.store.ListVisits(c.Request.Context(), limit, offset)
	if err != nil {
		log.Printf("list visits: %v", err)
		respondInternal(c)
		return
	}

	setContentRange(c, "link_visits", offset, limit, int(total))
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
		respondNotFound(c)
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

	var payload createLinkPayload
	if !bindPayload(c, &payload) {
		return
	}

	if !h.validateURL(c, payload.OriginalURL) {
		return
	}

	link, err := h.store.UpdateLink(c.Request.Context(), id,
		payload.OriginalURL,
		storage.OptionalString{Value: payload.ShortName, Set: payload.ShortName != ""})
	switch {
	case errors.Is(err, storage.ErrNotFound):
		respondNotFound(c)
	case errors.Is(err, storage.ErrDuplicate):
		respondShortNameTaken(c)
	case err != nil:
		log.Printf("update link: %v", err)
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
		respondInternal(c)
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
		respondNotFound(c)
		return
	}
	if err != nil {
		log.Printf("get link by short_name: %v", err)
		respondInternal(c)
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

// bindPayload разбирает JSON и при ошибке валидации сам отдаёт ответ.
// Возвращает false, если ответ уже отправлен.
func bindPayload(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		var verr validator.ValidationErrors
		if errors.As(err, &verr) {
			errs := make(map[string]string, len(verr))
			for _, e := range verr {
				errs[e.Field()] = e.Error()
			}
			c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": errs})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return false
	}
	return true
}

// validateURL проверяет original_url и при ошибке сам отдаёт 422.
// Возвращает false, если ответ уже отправлен.
func (h *Handlers) validateURL(c *gin.Context, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"errors": gin.H{"original_url": "must be an absolute http(s) URL"},
		})
		return false
	}
	return true
}

// resolvePaging вычисляет limit/offset по параметру ?range.
// Без range возвращает все записи (limit = total). При некорректном range
// сам отвечает 400 и возвращает ok=false.
func resolvePaging(c *gin.Context, total int64) (limit, offset int, ok bool) {
	rawRange := c.Query("range")
	if rawRange == "" {
		rawRange = c.GetHeader("Range")
	}
	if rawRange == "" {
		return int(total), 0, true // без range — все записи
	}

	start, end, err := parseRange(rawRange)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return 0, 0, false
	}
	if int64(start) >= total {
		return 0, int(start), true // пустое окно
	}
	if end-start > maxPageSize {
		end = start + maxPageSize
	}
	return int(end - start), int(start), true
}

// setContentRange ставит заголовок Content-Range: "<resource> start-end/total".
// Если окно пустое или запрошенный диапазон за пределами total,
// используется формат RFC 9110: "<resource> */total".
func setContentRange(c *gin.Context, resource string, offset, limit, total int) {
	if limit <= 0 || offset >= total {
		c.Header("Content-Range", fmt.Sprintf("%s */%d", resource, total))
		return
	}
	rangeEnd := offset + limit
	if rangeEnd > total {
		rangeEnd = total
	}
	c.Header("Content-Range", fmt.Sprintf("%s %d-%d/%d", resource, offset, rangeEnd, total))
}

func respondInternal(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": errInternal})
}

func respondNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
}

func respondShortNameTaken(c *gin.Context) {
	c.JSON(http.StatusUnprocessableEntity, gin.H{
		"errors": gin.H{"short_name": "short name already in use"},
	})
}

// resolveShortName возвращает переданное имя или генерирует новое.
// При ошибке генерации сам отвечает 500 и возвращает ok=false.
func resolveShortName(c *gin.Context, requested string) (string, bool) {
	if requested != "" {
		return requested, true
	}
	sn, err := generateShortName(shortNameLength)
	if err != nil {
		log.Printf("generate short name: %v", err)
		respondInternal(c)
		return "", false
	}
	return sn, true
}
