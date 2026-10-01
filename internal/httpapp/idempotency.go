package httpapp

// API idempotency: an Idempotency-Key header on POST requests turns client
// retries into replays of the stored response instead of double applies.
// The reservation is a single INSERT ... ON CONFLICT in the store, so two
// concurrent identical requests can never both proceed.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/your-org/go-template/internal/httpx"
	"github.com/your-org/go-template/internal/kernel"
	"github.com/your-org/go-template/internal/observ"
)

const idempotencyHeader = "Idempotency-Key"

// bodyWriter captures the response so a completed reservation can replay it.
type bodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

// idempotency applies to routes registered with it. It enforces the header
// only when present, so existing clients keep working and clients that care
// about retries opt in.
func (a *App) idempotency() gin.HandlerFunc {
	if a.idempotencyStore == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		key := c.GetHeader(idempotencyHeader)
		if key == "" {
			c.Next()
			return
		}
		if len(key) > 200 {
			a.problem(c, http.StatusBadRequest, "idempotency key too long", nil)
			c.Abort()
			return
		}
		ctx := c.Request.Context()
		user := a.currentUser(c)

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			a.problem(c, http.StatusBadRequest, "unreadable body", err)
			c.Abort()
			return
		}
		_ = c.Request.Body.Close()
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])

		stored, already, err := a.idempotencyStore.ReserveIdempotency(ctx, kernel.IdempotencyRecord{
			Key: key, UserID: user.ID, Method: c.Request.Method, Path: c.Request.URL.Path,
			RequestHash: hash, ExpiresAt: time.Now().Add(a.cfg.IdempotencyTTL),
		})
		if err != nil {
			httpx.Fail(c, a.logger, err)
			c.Abort()
			return
		}
		if already {
			if stored.RequestHash != hash {
				a.problem(c, http.StatusUnprocessableEntity, "idempotency key reused with a different request", nil)
				c.Abort()
				return
			}
			if stored.Status == 0 {
				// The original request is still running.
				c.Header("Retry-After", "1")
				a.problem(c, http.StatusConflict, "request with this idempotency key is in flight", nil)
				c.Abort()
				return
			}
			observ.IdempotencyReplays.Inc()
			c.Header(idempotencyHeader, key)
			c.Header("Idempotency-Replayed", "true")
			c.Data(stored.Status, stored.ContentType, stored.ResponseBody)
			c.Abort()
			return
		}

		recorder := &bodyWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = recorder
		c.Next()

		completeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.idempotencyStore.CompleteIdempotency(completeCtx, key, user.ID,
			recorder.Status(), recorder.body.Bytes(), recorder.Header().Get("Content-Type"))
	}
}
