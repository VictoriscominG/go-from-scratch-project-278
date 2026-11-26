package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"link-shortener/internal/handlers"
	"link-shortener/internal/storage"
)

func New(store storage.Store, baseURL string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	if err := router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		sentry.CaptureException(err)
		panic(fmt.Sprintf("invalid trusted proxies config: %v", err))
	}

	router.TrustedPlatform = gin.PlatformCloudflare

	router.Use(cors.New(cors.Config{
		AllowOrigins: allowedOrigins(),
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:       12 * time.Hour,
	}))

	router.Use(sentrygin.New(sentrygin.Options{}))
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	if os.Getenv("SENTRY_DEBUG") == "on" {
		router.GET("/debug/sentry", func(c *gin.Context) {
			sentry.CaptureException(errors.New("test error from link-shortener"))
			sentry.Flush(3 * time.Second)
			c.String(http.StatusOK, "sent")
		})
	}

	handlers.New(store, baseURL).Register(router)

	return router
}

func allowedOrigins() []string {
	origins := []string{"http://localhost:5173"}
	if base := os.Getenv("BASE_URL"); base != "" {
		origins = append(origins, base)
	}
	return origins
}
