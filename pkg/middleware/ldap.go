package middleware

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// LDAPConfig holds LDAP authentication configuration
type LDAPConfig struct {
	// Server configuration
	Server   string // LDAP server address (e.g., "ldap.example.com:389")
	UseTLS   bool   // Use TLS/LDAPS (port 636)
	UseSSL   bool   // Use StartTLS upgrade (port 389)
	Insecure bool   // Skip TLS certificate verification (not recommended for production)

	// Authentication configuration
	BindDN       string // DN for bind authentication (e.g., "cn=admin,dc=example,dc=com")
	BindPassword string // Password for bind DN
	BaseDN       string // Base DN for user searches (e.g., "ou=users,dc=example,dc=com")
	UserFilter   string // LDAP filter for user search (e.g., "(uid=%s)" or "(sAMAccountName=%s)")

	// Authorization (optional)
	RequireGroup string // Require membership in this group (e.g., "cn=api-users,ou=groups,dc=example,dc=com")

	// Connection pool settings
	PoolSize        int           // Connection pool size (default: 10)
	ConnMaxLifetime time.Duration // Max connection lifetime (default: 10 minutes)
	ConnTimeout     time.Duration // Connection timeout (default: 10 seconds)

	// Cache settings (to reduce LDAP queries)
	CacheEnabled bool          // Enable authentication result caching
	CacheTTL     time.Duration // Cache TTL for successful authentications (default: 5 minutes)
}

// LDAPAuthenticator handles LDAP authentication
type LDAPAuthenticator struct {
	config      *LDAPConfig
	connPool    chan *ldap.Conn
	authCache   map[string]*cachedAuth
	cacheMutex  sync.RWMutex
	stopCleaner chan struct{}
}

// cachedAuth stores cached authentication results
type cachedAuth struct {
	username  string
	expiresAt time.Time
}

// NewLDAPAuthenticator creates a new LDAP authenticator
func NewLDAPAuthenticator(config *LDAPConfig) (*LDAPAuthenticator, error) {
	// Set defaults
	if config.PoolSize <= 0 {
		config.PoolSize = 10
	}
	if config.ConnMaxLifetime == 0 {
		config.ConnMaxLifetime = 10 * time.Minute
	}
	if config.ConnTimeout == 0 {
		config.ConnTimeout = 10 * time.Second
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.UserFilter == "" {
		config.UserFilter = "(uid=%s)" // Default to uid attribute
	}

	auth := &LDAPAuthenticator{
		config:      config,
		connPool:    make(chan *ldap.Conn, config.PoolSize),
		authCache:   make(map[string]*cachedAuth),
		stopCleaner: make(chan struct{}),
	}

	// Pre-populate connection pool
	for i := 0; i < config.PoolSize; i++ {
		conn, err := auth.createConnection()
		if err != nil {
			// Close any connections we already created
			close(auth.connPool)
			for c := range auth.connPool {
				c.Close()
			}
			return nil, fmt.Errorf("failed to create LDAP connection pool: %w", err)
		}
		auth.connPool <- conn
	}

	// Start cache cleanup goroutine if caching is enabled
	if config.CacheEnabled {
		go auth.cleanupCache()
	}

	return auth, nil
}

// createConnection creates a new LDAP connection
func (a *LDAPAuthenticator) createConnection() (*ldap.Conn, error) {
	var conn *ldap.Conn
	var err error

	// Determine connection method
	if a.config.UseTLS {
		// Direct TLS connection (LDAPS, typically port 636)
		tlsConfig := &tls.Config{
			InsecureSkipVerify: a.config.Insecure,
			ServerName:         strings.Split(a.config.Server, ":")[0],
		}
		conn, err = ldap.DialTLS("tcp", a.config.Server, tlsConfig)
	} else {
		// Plain connection (typically port 389)
		conn, err = ldap.Dial("tcp", a.config.Server)
		if err != nil {
			return nil, err
		}

		// Upgrade to TLS if requested
		if a.config.UseSSL {
			tlsConfig := &tls.Config{
				InsecureSkipVerify: a.config.Insecure,
				ServerName:         strings.Split(a.config.Server, ":")[0],
			}
			err = conn.StartTLS(tlsConfig)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server %s: %w", a.config.Server, err)
	}

	// Set connection timeout
	conn.SetTimeout(a.config.ConnTimeout)

	return conn, nil
}

// getConnection gets a connection from the pool
func (a *LDAPAuthenticator) getConnection() (*ldap.Conn, error) {
	select {
	case conn := <-a.connPool:
		// Test if connection is still alive
		if err := conn.Bind(a.config.BindDN, a.config.BindPassword); err != nil {
			// Connection is dead, create a new one
			conn.Close()
			return a.createConnection()
		}
		return conn, nil
	case <-time.After(a.config.ConnTimeout):
		return nil, fmt.Errorf("timeout waiting for LDAP connection")
	}
}

// returnConnection returns a connection to the pool
func (a *LDAPAuthenticator) returnConnection(conn *ldap.Conn) {
	select {
	case a.connPool <- conn:
		// Connection returned to pool
	default:
		// Pool is full, close the connection
		conn.Close()
	}
}

// Authenticate authenticates a user against LDAP
func (a *LDAPAuthenticator) Authenticate(username, password string) error {
	// Check cache first
	if a.config.CacheEnabled {
		if a.checkCache(username, password) {
			return nil
		}
	}

	// Get connection from pool
	conn, err := a.getConnection()
	if err != nil {
		return fmt.Errorf("failed to get LDAP connection: %w", err)
	}
	defer a.returnConnection(conn)

	// Bind as service account
	if err := conn.Bind(a.config.BindDN, a.config.BindPassword); err != nil {
		return fmt.Errorf("LDAP service bind failed: %w", err)
	}

	// Search for user
	userFilter := fmt.Sprintf(a.config.UserFilter, ldap.EscapeFilter(username))
	searchRequest := ldap.NewSearchRequest(
		a.config.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, // No size limit
		0, // No time limit
		false,
		userFilter,
		[]string{"dn"}, // Only need DN
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return fmt.Errorf("LDAP search failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		return fmt.Errorf("user not found: %s", username)
	}

	if len(sr.Entries) > 1 {
		return fmt.Errorf("multiple users found for username: %s", username)
	}

	userDN := sr.Entries[0].DN

	// Authenticate as user
	if err := conn.Bind(userDN, password); err != nil {
		return fmt.Errorf("authentication failed for user %s: %w", username, err)
	}

	// Check group membership if required
	if a.config.RequireGroup != "" {
		if err := a.checkGroupMembership(conn, userDN); err != nil {
			return err
		}
	}

	// Cache successful authentication
	if a.config.CacheEnabled {
		a.cacheAuth(username, password)
	}

	return nil
}

// checkGroupMembership checks if user is member of required group
func (a *LDAPAuthenticator) checkGroupMembership(conn *ldap.Conn, userDN string) error {
	// Re-bind as service account for group search
	if err := conn.Bind(a.config.BindDN, a.config.BindPassword); err != nil {
		return fmt.Errorf("LDAP service bind failed: %w", err)
	}

	// Search for group membership
	groupFilter := fmt.Sprintf("(&(objectClass=groupOfNames)(member=%s))", ldap.EscapeFilter(userDN))
	searchRequest := ldap.NewSearchRequest(
		a.config.RequireGroup,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		groupFilter,
		[]string{"cn"},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return fmt.Errorf("group membership check failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		return fmt.Errorf("user is not a member of required group: %s", a.config.RequireGroup)
	}

	return nil
}

// checkCache checks if authentication is cached
func (a *LDAPAuthenticator) checkCache(username, password string) bool {
	a.cacheMutex.RLock()
	defer a.cacheMutex.RUnlock()

	cacheKey := fmt.Sprintf("%s:%s", username, password) // In production, use hashed password
	cached, ok := a.authCache[cacheKey]
	if !ok {
		return false
	}

	// Check if cached entry is still valid
	if time.Now().After(cached.expiresAt) {
		return false
	}

	return true
}

// cacheAuth caches a successful authentication
func (a *LDAPAuthenticator) cacheAuth(username, password string) {
	a.cacheMutex.Lock()
	defer a.cacheMutex.Unlock()

	cacheKey := fmt.Sprintf("%s:%s", username, password) // In production, use hashed password
	a.authCache[cacheKey] = &cachedAuth{
		username:  username,
		expiresAt: time.Now().Add(a.config.CacheTTL),
	}
}

// cleanupCache periodically removes expired cache entries
func (a *LDAPAuthenticator) cleanupCache() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.cacheMutex.Lock()
			now := time.Now()
			for key, cached := range a.authCache {
				if now.After(cached.expiresAt) {
					delete(a.authCache, key)
				}
			}
			a.cacheMutex.Unlock()
		case <-a.stopCleaner:
			return
		}
	}
}

// Close closes all LDAP connections and stops background goroutines
func (a *LDAPAuthenticator) Close() {
	// Stop cache cleaner
	if a.config.CacheEnabled {
		close(a.stopCleaner)
	}

	// Close all connections in pool
	close(a.connPool)
	for conn := range a.connPool {
		conn.Close()
	}
}

// LDAPAuthMiddleware returns an HTTP middleware for LDAP authentication
func LDAPAuthMiddleware(auth *LDAPAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Basic Auth credentials
			username, password, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="LDAP Authentication Required"`)
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Authenticate against LDAP
			if err := auth.Authenticate(username, password); err != nil {
				w.Header().Set("WWW-Authenticate", `Basic realm="LDAP Authentication Required"`)
				http.Error(w, "Authentication failed", http.StatusUnauthorized)
				return
			}

			// Add username to request context for downstream handlers
			// In production, use proper context values
			r.Header.Set("X-Authenticated-User", username)

			next.ServeHTTP(w, r)
		})
	}
}
