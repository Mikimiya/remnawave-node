package middleware

import (
	"bytes"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Mikimiya/remnawave-node/pkg/logger"
	"github.com/gin-gonic/gin"
)

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// isHandlerOrXrayRoute returns true if the path is a handler or xray route
// that should be logged with request/response details even in production
func isHandlerOrXrayRoute(path string) bool {
	return strings.Contains(path, "/handler/") || strings.Contains(path, "/xray/start")
}

// Logger creates a logging middleware
func Logger(log *logger.Logger) gin.HandlerFunc {
	isDev := os.Getenv("NODE_ENV") == "development"

	return func(c *gin.Context) {
		// Start timer
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		var requestBody []byte
		var rw *responseWriter

		// Capture request/response bodies for:
		// 1. All routes in development mode
		// 2. Handler and xray/start routes in production (critical for debugging user operations)
		captureBody := isDev || isHandlerOrXrayRoute(path)

		if captureBody {
			// Read request body
			if c.Request.Body != nil {
				requestBody, _ = io.ReadAll(c.Request.Body)
				// Restore request body for handlers
				c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
			}

			// Wrap response writer to capture response body
			rw = &responseWriter{
				ResponseWriter: c.Writer,
				body:           bytes.NewBuffer(nil),
			}
			c.Writer = rw
		}

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get status code
		statusCode := c.Writer.Status()

		// Build path with query
		if raw != "" {
			path = path + "?" + raw
		}

		// Get client IP
		clientIP := c.ClientIP()

		// Log request
		if captureBody {
			// Log with request/response bodies (dev mode or handler/xray routes)
			reqBodyStr := string(requestBody)
			respBodyStr := rw.body.String()

			// Truncate long bodies
			const maxBodyLen = 4096
			if len(reqBodyStr) > maxBodyLen {
				reqBodyStr = reqBodyStr[:maxBodyLen] + "...(truncated)"
			}
			if len(respBodyStr) > maxBodyLen {
				respBodyStr = respBodyStr[:maxBodyLen] + "...(truncated)"
			}

			log.Infow("Request",
				"status", statusCode,
				"method", c.Request.Method,
				"path", path,
				"ip", clientIP,
				"latency", latency.String(),
				"request_body", reqBodyStr,
				"response_body", respBodyStr,
			)
		} else {
			// Production mode: minimal logging for non-handler routes
			log.Infow("Request",
				"status", statusCode,
				"method", c.Request.Method,
				"path", path,
				"ip", clientIP,
				"latency", latency.String(),
			)
		}
	}
}

// Recovery creates a recovery middleware
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorw("Panic recovered",
					"error", err,
					"path", c.Request.URL.Path,
				)
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}
