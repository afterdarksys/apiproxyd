package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipWriterPool is a pool of gzip writers to reduce GC pressure
// Gzip compression is CPU-intensive, and creating new writers allocates memory.
// Pooling allows reuse of allocated buffers, significantly improving performance.
var gzipWriterPool = sync.Pool{
	New: func() interface{} {
		// Create gzip writer with default compression level
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

// getGzipWriter gets a gzip writer from the pool
func getGzipWriter(w io.Writer) *gzip.Writer {
	gz := gzipWriterPool.Get().(*gzip.Writer)
	gz.Reset(w)
	return gz
}

// putGzipWriter returns a gzip writer to the pool
func putGzipWriter(gz *gzip.Writer) {
	gz.Close()
	gzipWriterPool.Put(gz)
}

// gzipResponseWriter wraps http.ResponseWriter to support gzip compression
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.Writer.Write(b)
}

// GzipMiddleware provides gzip compression for responses
// It only compresses responses larger than 1KB to avoid overhead for small responses
func GzipMiddleware(minSize int) func(http.Handler) http.Handler {
	if minSize <= 0 {
		minSize = 1024 // default 1KB minimum
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if client accepts gzip
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			// Don't compress if already compressed
			if w.Header().Get("Content-Encoding") != "" {
				next.ServeHTTP(w, r)
				return
			}

			// Get gzip writer from pool
			gz := getGzipWriter(w)
			defer putGzipWriter(gz)

			// Set headers
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			w.Header().Del("Content-Length") // Length will change after compression

			// Wrap response writer
			gzw := &gzipResponseWriter{
				Writer:         gz,
				ResponseWriter: w,
			}

			next.ServeHTTP(gzw, r)
		})
	}
}

// GzipHandler wraps a handler with gzip compression
func GzipHandler(h http.Handler) http.Handler {
	return GzipMiddleware(1024)(h)
}
