package middleware

import (
	"bytes"
	"io"
	"os"
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

		// In development mode, capture request and response bodies
		if isDev {
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
		if isDev {
			// Development mode: log with request/response bodies
			reqBodyStr := string(requestBody)
			respBodyStr := rw.body.String()

			// Truncate long bodies to avoid log spam
			const maxBodyLen = 2048
			if len(reqBodyStr) > maxBodyLen {
				reqBodyStr = reqBodyStr[:maxBodyLen] + "...(truncated)"
			}
			if len(respBodyStr) > maxBodyLen {
				respBodyStr = respBodyStr[:maxBodyLen] + "...(truncated)"
			}

			log.Debugw("Request",
				"status", statusCode,
				"method", c.Request.Method,
				"path", path,
				"ip", clientIP,
				"latency", latency.String(),
				"user-agent", c.Request.UserAgent(),
				"request_body", reqBodyStr,
				"response_body", respBodyStr,
			)
		} else {
			// Production mode: minimal logging
			log.Infow("Request",
				"status", statusCode,
				"method", c.Request.Method,
				"path", path,
				"ip", clientIP,
				"latency", latency.String(),
				"user-agent", c.Request.UserAgent(),
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
