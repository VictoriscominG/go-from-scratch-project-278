package server

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"

	"link-shortener/internal/handlers"
	"link-shortener/internal/storage"
)

func New(store storage.Store) *gin.Engine {
	router := gin.New()

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

	handlers.New(store).Register(router)

	return router
}
