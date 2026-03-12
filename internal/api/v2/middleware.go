package v2

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// LoggingMiddleware logs all HTTP requests with details
type LoggingMiddleware struct {
	logger *log.Helper
	next   http.Handler
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware(logger log.Logger) func(http.Handler) http.Handler {
	helper := log.NewHelper(logger)
	return func(next http.Handler) http.Handler {
		return &LoggingMiddleware{
			logger: helper,
			next:   next,
		}
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

func (m *LoggingMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Read and log request body (for POST/PUT requests)
	var bodyBytes []byte
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
		bodyBytes, _ = io.ReadAll(r.Body)
		r.Body.Close()
		// Restore body for handler
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Log incoming request
	m.logger.Infof("→ %s %s", r.Method, r.URL.Path)
	if len(bodyBytes) > 0 && len(bodyBytes) < 2000 {
		m.logger.Debugf("  Body: %s", string(bodyBytes))
	}

	// Wrap response writer to capture status
	wrapped := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// Call next handler
	m.next.ServeHTTP(wrapped, r)

	// Log response
	duration := time.Since(start)
	if wrapped.statusCode >= 400 {
		m.logger.Warnf("← %s %s [%d] %v", r.Method, r.URL.Path, wrapped.statusCode, duration)
	} else {
		m.logger.Infof("← %s %s [%d] %v", r.Method, r.URL.Path, wrapped.statusCode, duration)
	}
}
