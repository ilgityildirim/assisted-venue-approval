package auth

import (
	"context"
	"net/http"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// AdminIDKey is the context key for the admin member ID
	AdminIDKey contextKey = "admin_id"
	// ClientIPKey is the context key for the client IP address
	ClientIPKey contextKey = "client_ip"
)

// AdminAuthMiddleware is a middleware that resolves admin ID from client IP
// If the admin ID cannot be resolved, it shows an unauthorized page
type AdminAuthMiddleware struct {
	resolver           *AdminResolver
	renderUnauthorized func(w http.ResponseWriter, ip string)
}

// NewAdminAuthMiddleware creates a new admin authentication middleware
func NewAdminAuthMiddleware(resolver *AdminResolver, renderUnauthorized func(w http.ResponseWriter, ip string)) *AdminAuthMiddleware {
	return &AdminAuthMiddleware{
		resolver:           resolver,
		renderUnauthorized: renderUnauthorized,
	}
}

// Handler wraps an HTTP handler with admin authentication
func (m *AdminAuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always get client IP first to display on unauthorized page
		clientIP := m.resolver.GetClientIP(r)

		// Check if config is loaded
		if !m.resolver.IsLoaded() {
			m.renderUnauthorized(w, clientIP)
			return
		}

		// Resolve admin ID
		adminID, found := m.resolver.GetAdminID(r)

		if !found {
			m.renderUnauthorized(w, clientIP)
			return
		}

		// Add admin ID and client IP to request context
		ctx := context.WithValue(r.Context(), AdminIDKey, adminID)
		ctx = context.WithValue(ctx, ClientIPKey, clientIP)

		// Continue to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetAdminIDFromContext retrieves the admin ID from the request context
func GetAdminIDFromContext(ctx context.Context) (int, bool) {
	adminID, ok := ctx.Value(AdminIDKey).(int)
	return adminID, ok
}

// GetClientIPFromContext retrieves the client IP from the request context
func GetClientIPFromContext(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(ClientIPKey).(string)
	return ip, ok
}
