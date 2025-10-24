package auth

import (
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// AdminResolver resolves client IP addresses to admin member IDs
type AdminResolver struct {
	mu       sync.RWMutex
	ipToID   map[string]int
	loaded   bool
	yamlPath string
}

// NewAdminResolver creates a new admin resolver
// It attempts to load admins.yaml from:
// 1. Path specified in ADMINS_YAML_PATH env variable
// 2. Current working directory
func NewAdminResolver() *AdminResolver {
	resolver := &AdminResolver{
		ipToID:   make(map[string]int),
		loaded:   false,
		yamlPath: "",
	}

	var yamlPath string

	// Check if path is specified via environment variable
	if envPath := os.Getenv("ADMINS_YAML_PATH"); envPath != "" {
		yamlPath = envPath
		log.Printf("Using admins.yaml path from ADMINS_YAML_PATH: %s", yamlPath)
	} else {
		// Use current working directory
		cwd, err := os.Getwd()
		if err != nil {
			log.Printf("Warning: Cannot determine working directory: %v", err)
			return resolver
		}
		yamlPath = filepath.Join(cwd, "admins.yaml")
		log.Printf("Looking for admins.yaml in current working directory: %s", yamlPath)
	}

	// Try to load the config
	if err := resolver.loadConfig(yamlPath); err != nil {
		log.Printf("ERROR: admins.yaml not loaded from %s: %v", yamlPath, err)
		log.Printf("IMPORTANT: Approve/reject actions will be BLOCKED until admins.yaml is present at: %s", yamlPath)
	} else {
		resolver.yamlPath = yamlPath
		log.Printf("SUCCESS: Loaded admin IP mappings from: %s (%d entries)", yamlPath, len(resolver.ipToID))
	}

	return resolver
}

// loadConfig loads the YAML configuration file
func (r *AdminResolver) loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config map[string]int
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ipToID = config
	r.loaded = true

	return nil
}

// Reload reloads the admin configuration from disk
func (r *AdminResolver) Reload() error {
	if r.yamlPath == "" {
		return nil // No config file to reload
	}
	return r.loadConfig(r.yamlPath)
}

// IsLoaded returns true if the config file was successfully loaded
func (r *AdminResolver) IsLoaded() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loaded
}

// GetAdminID resolves the client IP from the request to an admin member ID
// Returns (adminID, found)
func (r *AdminResolver) GetAdminID(req *http.Request) (int, bool) {
	ip := extractClientIP(req)

	r.mu.RLock()
	defer r.mu.RUnlock()

	adminID, found := r.ipToID[ip]
	if !found {
		log.Printf("Warning: Cannot determine admin ID for IP: %s", ip)
	}

	return adminID, found
}

// GetClientIP returns the client IP address from the request
func (r *AdminResolver) GetClientIP(req *http.Request) string {
	return extractClientIP(req)
}

// extractClientIP extracts the real client IP from the request
// Handles X-Forwarded-For and X-Real-IP headers for reverse proxy scenarios
func extractClientIP(req *http.Request) string {
	// Try X-Forwarded-For first (for reverse proxies)
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if ip := parseFirstIP(xff); ip != "" {
			return ip
		}
	}

	// Try X-Real-IP
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr // Return as-is if split fails
	}

	return ip
}

// parseFirstIP extracts the first IP from a comma-separated list
func parseFirstIP(xff string) string {
	for i := 0; i < len(xff); i++ {
		if xff[i] == ',' {
			return xff[:i]
		}
	}
	return xff
}
