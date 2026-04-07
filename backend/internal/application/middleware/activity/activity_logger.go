package middleware_activity

import (
	"context"
	"time"

	"backend/internal/features/activitylog"
	"backend/internal/utilities/logger"
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

type activityLogWriter interface {
	Create(ctx context.Context, entry *activitylog.ActivityLog) error
}

type activityLogEntry struct {
	role       string
	method     string
	path       string
	statusCode int
	durationMs int
	occurredAt time.Time
}

// ActivityLogger captures API request metrics asynchronously via a buffered
// channel so that DB writes never add latency to the HTTP response path.
type ActivityLogger struct {
	ch   chan activityLogEntry
	repo activityLogWriter
	done chan struct{}
}

// NewActivityLogger creates an ActivityLogger and starts its drain goroutine.
func NewActivityLogger(repo activityLogWriter) *ActivityLogger {
	al := &ActivityLogger{
		ch:   make(chan activityLogEntry, 2048),
		repo: repo,
		done: make(chan struct{}),
	}
	go al.drain()
	return al
}

func (al *ActivityLogger) drain() {
	for entry := range al.ch {
		log := &activitylog.ActivityLog{
			Role:       entry.role,
			EventType:  "api_request",
			Method:     entry.method,
			Path:       entry.path,
			StatusCode: entry.statusCode,
			DurationMs: entry.durationMs,
			OccurredAt: entry.occurredAt,
		}
		if err := al.repo.Create(context.Background(), log); err != nil {
			logger.Warn("activitylog: failed to write api_request entry: %v", err)
		}
	}
	close(al.done)
}

// Stop drains remaining entries and waits for the worker goroutine to finish.
// Call during graceful shutdown.
func (al *ActivityLogger) Stop() {
	close(al.ch)
	<-al.done
}

// Middleware returns a Gin handler that logs every API request by role group.
// Logging is fully off the request path — the response is written before any
// channel send occurs, and the send itself is non-blocking.
func (al *ActivityLogger) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next() // run all downstream handlers; response is written here

		role := "anonymous"
		if claims, ok := middleware.GetClaims(c); ok && claims.Role != "" {
			role = claims.Role
		}

		entry := activityLogEntry{
			role:       role,
			method:     c.Request.Method,
			path:       c.FullPath(), // Gin route template, e.g. /api/courses/:id
			statusCode: c.Writer.Status(),
			durationMs: int(time.Since(start).Milliseconds()),
			occurredAt: time.Now().UTC(),
		}

		// Non-blocking: drop the entry rather than stalling the goroutine if
		// the channel is full (e.g. under sustained extreme load).
		select {
		case al.ch <- entry:
		default:
			logger.Warn("activitylog: channel full, dropping entry for %s %s", entry.method, entry.path)
		}
	}
}
