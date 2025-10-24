package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAdminResolver_GetAdminID(t *testing.T) {
	// Create a temporary YAML file for testing
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "admins.yaml")
	yamlContent := `"10.0.1.5": 123456
"10.0.1.8": 789012
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test YAML file: %v", err)
	}

	// Create resolver and load config
	resolver := &AdminResolver{
		ipToID:   make(map[string]int),
		loaded:   false,
		yamlPath: yamlPath,
	}

	if err := resolver.loadConfig(yamlPath); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	tests := []struct {
		name          string
		remoteAddr    string
		expectedID    int
		expectedFound bool
		xForwardedFor string
		xRealIP       string
	}{
		{
			name:          "Valid IP - RemoteAddr",
			remoteAddr:    "10.0.1.5:12345",
			expectedID:    123456,
			expectedFound: true,
		},
		{
			name:          "Valid IP - X-Forwarded-For",
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "10.0.1.8",
			expectedID:    789012,
			expectedFound: true,
		},
		{
			name:          "Valid IP - X-Real-IP",
			remoteAddr:    "192.168.1.1:12345",
			xRealIP:       "10.0.1.5",
			expectedID:    123456,
			expectedFound: true,
		},
		{
			name:          "Unknown IP",
			remoteAddr:    "192.168.1.1:12345",
			expectedID:    0,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr

			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			adminID, found := resolver.GetAdminID(req)

			if found != tt.expectedFound {
				t.Errorf("GetAdminID() found = %v, want %v", found, tt.expectedFound)
			}

			if found && adminID != tt.expectedID {
				t.Errorf("GetAdminID() adminID = %v, want %v", adminID, tt.expectedID)
			}
		})
	}
}

func TestAdminResolver_IsLoaded(t *testing.T) {
	resolver := &AdminResolver{
		ipToID: make(map[string]int),
		loaded: false,
	}

	if resolver.IsLoaded() {
		t.Error("IsLoaded() should return false for unloaded config")
	}

	resolver.loaded = true

	if !resolver.IsLoaded() {
		t.Error("IsLoaded() should return true for loaded config")
	}
}

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name          string
		remoteAddr    string
		xForwardedFor string
		xRealIP       string
		expectedIP    string
	}{
		{
			name:       "RemoteAddr only",
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "192.168.1.1",
		},
		{
			name:          "X-Forwarded-For single IP",
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "10.0.1.5",
			expectedIP:    "10.0.1.5",
		},
		{
			name:          "X-Forwarded-For multiple IPs",
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "10.0.1.5, 192.168.1.2, 192.168.1.3",
			expectedIP:    "10.0.1.5",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "192.168.1.1:12345",
			xRealIP:    "10.0.1.8",
			expectedIP: "10.0.1.8",
		},
		{
			name:          "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr:    "192.168.1.1:12345",
			xForwardedFor: "10.0.1.5",
			xRealIP:       "10.0.1.8",
			expectedIP:    "10.0.1.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				RemoteAddr: tt.remoteAddr,
				Header:     make(http.Header),
			}

			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			ip := extractClientIP(req)

			if ip != tt.expectedIP {
				t.Errorf("extractClientIP() = %v, want %v", ip, tt.expectedIP)
			}
		})
	}
}
