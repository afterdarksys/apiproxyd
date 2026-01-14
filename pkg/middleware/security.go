package middleware

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// SecurityConfig holds security middleware configuration
type SecurityConfig struct {
	MaxRequestBodySize  int64
	MaxResponseBodySize int64
	AllowedHosts        []string
	BlockPrivateIPs     bool
}

// BodySizeLimiter limits request body size to prevent memory exhaustion
func BodySizeLimiter(maxSize int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxSize {
				http.Error(w, fmt.Sprintf("Request body too large (max %d bytes)", maxSize), http.StatusRequestEntityTooLarge)
				return
			}

			// Wrap the body to enforce the limit even if Content-Length is not set
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)

			next.ServeHTTP(w, r)
		})
	}
}

// SSRFProtection prevents Server-Side Request Forgery attacks
type SSRFProtection struct {
	allowedHosts  map[string]bool
	blockPrivate  bool
}

// NewSSRFProtection creates a new SSRF protection middleware
func NewSSRFProtection(allowedHosts []string, blockPrivate bool) *SSRFProtection {
	allowed := make(map[string]bool)
	for _, host := range allowedHosts {
		allowed[strings.ToLower(host)] = true
	}

	return &SSRFProtection{
		allowedHosts: allowed,
		blockPrivate: blockPrivate,
	}
}

// ValidateURL checks if a URL is safe to request
func (s *SSRFProtection) ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow HTTP and HTTPS
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}

	// Check if host is in allowed list (if allowlist is configured)
	if len(s.allowedHosts) > 0 {
		hostname := strings.ToLower(u.Hostname())
		if !s.allowedHosts[hostname] {
			return fmt.Errorf("host not allowed: %s", hostname)
		}
	}

	// Block private IP addresses if configured
	if s.blockPrivate {
		if err := s.checkPrivateIP(u.Hostname()); err != nil {
			return err
		}
	}

	return nil
}

// checkPrivateIP checks if a hostname resolves to a private IP
func (s *SSRFProtection) checkPrivateIP(hostname string) error {
	// Parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("private IP addresses are not allowed: %s", hostname)
		}
		return nil
	}

	// Resolve hostname
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %w", err)
	}

	// Check all resolved IPs
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("hostname resolves to private IP: %s -> %s", hostname, ip)
		}
	}

	return nil
}

// isPrivateIP checks if an IP is private/internal
func isPrivateIP(ip net.IP) bool {
	// Check for loopback
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // link-local
		"127.0.0.0/8",    // loopback
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local
	}

	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil && network.Contains(ip) {
			return true
		}
	}

	return false
}

// InputSanitizer sanitizes and validates input
func InputSanitizer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate Content-Type for POST/PUT requests
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			ct := r.Header.Get("Content-Type")
			// Only allow JSON for API requests
			if !strings.Contains(ct, "application/json") && ct != "" {
				http.Error(w, "Invalid Content-Type (must be application/json)", http.StatusUnsupportedMediaType)
				return
			}
		}

		// Remove potentially dangerous headers
		r.Header.Del("X-Forwarded-Host")

		next.ServeHTTP(w, r)
	})
}

// SecureHeaders adds security headers to responses
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// XSS protection
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// HSTS (only if using HTTPS)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content Security Policy
		w.Header().Set("Content-Security-Policy", "default-src 'self'")

		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	})
}

// limitedResponseWriter wraps http.ResponseWriter to limit response size
type limitedResponseWriter struct {
	http.ResponseWriter
	written int64
	limit   int64
}

func (lrw *limitedResponseWriter) Write(b []byte) (int, error) {
	if lrw.written+int64(len(b)) > lrw.limit {
		return 0, fmt.Errorf("response size limit exceeded")
	}
	n, err := lrw.ResponseWriter.Write(b)
	lrw.written += int64(n)
	return n, err
}

// ResponseSizeLimiter limits response body size
func ResponseSizeLimiter(maxSize int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lrw := &limitedResponseWriter{
				ResponseWriter: w,
				limit:          maxSize,
			}
			next.ServeHTTP(lrw, r)
		})
	}
}

// RecoveryMiddleware recovers from panics and returns a safe error
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the error (in production, use proper logging)
				fmt.Printf("Panic recovered: %v\n", err)

				// Return generic error (don't leak internal details)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// limitedReader wraps io.Reader to enforce size limits
type limitedReader struct {
	r io.Reader
	n int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, fmt.Errorf("read limit exceeded")
	}
	if int64(len(p)) > l.n {
		p = p[0:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

// LimitReader returns a Reader that reads from r but stops with an error after n bytes
func LimitReader(r io.Reader, n int64) io.Reader {
	return &limitedReader{r: r, n: n}
}
