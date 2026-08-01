package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── validateApp ────────────────────────────────────────────────────────────

func TestValidateApp(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want string // substring of error message, empty = no error
	}{
		{"valid minimal", App{ID: "myapp", Domain: "myapp.example.com", Port: 8080}, ""},
		{"empty id", App{ID: "", Domain: "myapp.example.com", Port: 8080}, "app ID is required"},
		{"empty domain", App{ID: "myapp", Domain: "", Port: 8080}, "app domain is required"},
		{"invalid port 0", App{ID: "myapp", Domain: "myapp.example.com", Port: 0}, "invalid port"},
		{"invalid port 70000", App{ID: "myapp", Domain: "myapp.example.com", Port: 70000}, "invalid port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateApp(tt.app)
			if tt.want == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("expected error containing %q, got %q", tt.want, err.Error())
				}
			}
		})
	}
}

// ─── validateTraefikConfig ──────────────────────────────────────────────────

func TestValidateTraefikConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   string
	}{
		{"valid", &Config{AdminEmail: "a@b.com", Domain: "example.com"}, ""},
		{"missing email", &Config{AdminEmail: "", Domain: "example.com"}, "admin_email is required"},
		{"missing domain", &Config{AdminEmail: "a@b.com", Domain: ""}, "domain is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTraefikConfig(tt.config)
			if tt.want == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.want)
				} else if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("expected error containing %q, got %q", tt.want, err.Error())
				}
			}
		})
	}
}

// ─── apr1Encode64 ───────────────────────────────────────────────────────────

func TestApr1Encode64(t *testing.T) {
	tests := []struct {
		src []byte
		n   int
	}{
		{[]byte("hello world"), 11},
		{[]byte{0x00, 0x01, 0x02}, 3},
		{[]byte{0xff, 0xee, 0xdd, 0xcc}, 4},
		{[]byte{}, 0},
	}
	for _, tt := range tests {
		result := apr1Encode64(tt.src, tt.n)
		if result == "" && tt.n > 0 {
			t.Errorf("apr1Encode64(%v, %d) returned empty", tt.src, tt.n)
		}
		// Verify it only uses the alphabet
		for _, c := range result {
			if !strings.ContainsRune(apr1Alphabet, c) {
				t.Errorf("apr1Encode64 returned char %c not in alphabet", c)
			}
		}
	}
}

// ─── HashAPR1 ───────────────────────────────────────────────────────────────

func TestHashAPR1(t *testing.T) {
	hash, err := HashAPR1("testpassword")
	if err != nil {
		t.Fatalf("HashAPR1 failed: %v", err)
	}
	if !strings.HasPrefix(hash, "$apr1$") {
		t.Errorf("hash should start with $apr1$, got %q", hash)
	}
	if len(hash) < 20 {
		t.Errorf("hash too short: %q", hash)
	}

	// Deterministic with same salt
	hash2, err := hashAPR1WithSalt("testpassword", "abcdefgh")
	if err != nil {
		t.Fatalf("hashAPR1WithSalt failed: %v", err)
	}
	hash3, err := hashAPR1WithSalt("testpassword", "abcdefgh")
	if err != nil {
		t.Fatalf("hashAPR1WithSalt failed: %v", err)
	}
	if hash2 != hash3 {
		t.Errorf("same salt should produce same hash:\n  %q\n  %q", hash2, hash3)
	}

	// Different passwords produce different hashes
	hash4, err := hashAPR1WithSalt("otherpass", "abcdefgh")
	if err != nil {
		t.Fatalf("hashAPR1WithSalt failed: %v", err)
	}
	if hash2 == hash4 {
		t.Errorf("different passwords with same salt should differ:\n  %q\n  %q", hash2, hash4)
	}
}

// ─── HtpasswdEntry ──────────────────────────────────────────────────────────

func TestHtpasswdEntry(t *testing.T) {
	entry, err := HtpasswdEntry("admin", "secret123")
	if err != nil {
		t.Fatalf("HtpasswdEntry failed: %v", err)
	}
	if !strings.HasPrefix(entry, "admin:") {
		t.Errorf("entry should start with 'admin:', got %q", entry)
	}
	if !strings.Contains(entry, "$apr1$") {
		t.Errorf("entry should contain $apr1$, got %q", entry)
	}
}

// ─── lineSplit ──────────────────────────────────────────────────────────────

func TestLineSplit(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello\nworld", 2},
		{"a\nb\nc\n", 3},
		{"\n\n\n", 3},
	}
	for _, tt := range tests {
		got := lineSplit(tt.input)
		if len(got) != tt.want {
			t.Errorf("lineSplit(%q) returned %d lines, want %d", tt.input, len(got), tt.want)
		}
	}
}

// ─── simpleDiff ─────────────────────────────────────────────────────────────

func TestSimpleDiff(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{"identical", "a\nb\nc", "a\nb\nc"},
		{"added line", "a\nb", "a\nb\nc"},
		{"removed line", "a\nb\nc", "a\nb"},
		{"changed line", "a\nb\nc", "a\nx\nc"},
		{"both empty", "", ""},
		{"empty old", "", "a\nb"},
		{"empty new", "a\nb", ""},
		{"trailing newline", "a\nb\n", "a\nb\nc\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := simpleDiff(tt.old, tt.new)
			// diff should contain lines prefixed with +, -, or space
			if tt.old == tt.new && tt.old != "" {
				if strings.Contains(diff, "+") || strings.Contains(diff, "-") {
					t.Errorf("identical inputs should have no +/- markers, got:\n%s", diff)
				}
			}
			if tt.old == "" && tt.new != "" {
				if !strings.Contains(diff, "+") {
					t.Errorf("empty old should have + lines, got:\n%s", diff)
				}
			}
			if tt.new == "" && tt.old != "" {
				if !strings.Contains(diff, "-") {
					t.Errorf("empty new should have - lines, got:\n%s", diff)
				}
			}
		})
	}
}

// ─── buildDynamicYAML ───────────────────────────────────────────────────────

func TestBuildDynamicYAML(t *testing.T) {
	// Save and restore traefikDynamic path
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()

	// Create a temp file for the "existing" dynamic.yml so readExistingBackendURLs works
	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")

	// Write a pre-existing dynamic.yml with a remote backend to test URL preservation
	existingYAML := `http:
  services:
    remote-app:
      loadBalancer:
        servers:
          - url: "http://100.123.0.125:7000"
    local-app:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8080"
`
	if err := os.WriteFile(traefikDynamic, []byte(existingYAML), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	config := &Config{
		Apps: []App{
			{
				ID:         "remote-app",
				Name:       "Remote App",
				Domain:     "remote.example.com",
				Port:       7000,
				Command:    "true",
				BasicAuth:  []string{"admin:$apr1$abc$def"},
				BackendURL: "http://100.123.0.125:7000",
			},
			{
				ID:      "local-app",
				Name:    "Local App",
				Domain:  "local.example.com",
				Port:    8080,
				Command: "myapp start",
			},
			{
				ID:        "rate-limited-app",
				Name:      "Rate Limited App",
				Domain:    "ratelimited.example.com",
				Port:      9090,
				Command:   "true",
				RateLimit: "10,60m",
			},
		},
	}

	yaml, err := buildDynamicYAML(config)
	if err != nil {
		t.Fatalf("buildDynamicYAML failed: %v", err)
	}

	// Check structure
	if !strings.Contains(yaml, "http:") {
		t.Error("YAML missing 'http:'")
	}
	if !strings.Contains(yaml, "routers:") {
		t.Error("YAML missing 'routers:'")
	}
	if !strings.Contains(yaml, "services:") {
		t.Error("YAML missing 'services:'")
	}

	// Check each app appears in routers
	for _, app := range config.Apps {
		if !strings.Contains(yaml, app.ID+":") {
			t.Errorf("YAML missing app ID %q", app.ID)
		}
		if !strings.Contains(yaml, "Host(`"+app.Domain+"`)") {
			t.Errorf("YAML missing Host rule for %q", app.Domain)
		}
	}

	// Check backend URLs
	if !strings.Contains(yaml, `url: "http://100.123.0.125:7000"`) {
		t.Error("YAML missing remote backend URL")
	}
	if !strings.Contains(yaml, `url: "http://127.0.0.1:8080"`) {
		t.Error("YAML missing localhost backend URL for local-app")
	}
	if !strings.Contains(yaml, `url: "http://127.0.0.1:9090"`) {
		t.Error("YAML missing localhost backend URL for rate-limited-app")
	}

	// Check basic auth middleware
	if !strings.Contains(yaml, "remote-app-basic-auth") {
		t.Error("YAML missing basic-auth middleware for remote-app")
	}
	if !strings.Contains(yaml, "admin:$apr1$abc$def") {
		t.Error("YAML missing basic auth user entry")
	}

	// Check rate limit middleware
	if !strings.Contains(yaml, "rate-limited-app-rate-limit") {
		t.Error("YAML missing rate-limit middleware for rate-limited-app")
	}
	if !strings.Contains(yaml, "average: 10") {
		t.Error("YAML missing rate limit average")
	}
	if !strings.Contains(yaml, "period: 60m") {
		t.Error("YAML missing rate limit period")
	}
	if !strings.Contains(yaml, "burst: 10") {
		t.Error("YAML missing rate limit burst")
	}

	// Check TLS domains
	if !strings.Contains(yaml, "domains:") {
		t.Error("YAML missing domains section")
	}
}

func TestBuildDynamicYAML_PreservesExistingURLs(t *testing.T) {
	// Test that readExistingBackendURLs fallback works when backend_url is not set
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")

	// Existing dynamic.yml has correct remote backend
	existingYAML := `http:
  services:
    remote-app:
      loadBalancer:
        servers:
          - url: "http://100.123.0.125:7000"
`
	if err := os.WriteFile(traefikDynamic, []byte(existingYAML), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	// App without backend_url in config — should be preserved from existing file
	config := &Config{
		Apps: []App{
			{
				ID:      "remote-app",
				Name:    "Remote App",
				Domain:  "remote.example.com",
				Port:    7000,
				Command: "true",
				// No BackendURL set — should fall back to existing dynamic.yml
			},
		},
	}

	yaml, err := buildDynamicYAML(config)
	if err != nil {
		t.Fatalf("buildDynamicYAML failed: %v", err)
	}

	if !strings.Contains(yaml, `url: "http://100.123.0.125:7000"`) {
		t.Errorf("YAML should preserve existing backend URL, got:\n%s", yaml)
	}
}

// ─── readExistingBackendURLs (via buildDynamicYAML) ────────────────────────

func TestBuildDynamicYAML_PathPrefixAndBackendURL(t *testing.T) {
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")

	config := &Config{
		Apps: []App{
			{
				ID:         "slv2",
				Name:       "SuperLandings v2",
				Domain:     "slv2.example.com",
				Port:       3100,
				Command:    "true",
				BackendURL: "http://127.0.0.1:3100",
				PathPrefix: "/slv2",
			},
		},
	}

	yaml, err := buildDynamicYAML(config)
	if err != nil {
		t.Fatalf("buildDynamicYAML failed: %v", err)
	}

	if !strings.Contains(yaml, "rule: \"Host(`slv2.example.com`)\"") {
		t.Error("YAML missing Host rule for slv2")
	}
	if !strings.Contains(yaml, `url: "http://127.0.0.1:3100"`) {
		t.Error("YAML missing backend URL for slv2")
	}
	if !strings.Contains(yaml, "slv2-addprefix:") {
		t.Error("YAML missing addPrefix middleware declaration")
	}
	if !strings.Contains(yaml, `prefix: "/slv2"`) {
		t.Error("YAML missing /slv2 prefix value")
	}
	if !strings.Contains(yaml, "- slv2-addprefix") {
		t.Error("YAML missing addPrefix middleware reference in router")
	}
}

func TestReadExistingBackendURLs_YAMLParsing(t *testing.T) {
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")

	yaml := "http:\n  routers:\n    app1:\n      service: app1\n" +
		"    app2:\n      service: app2\n" +
		"  services:\n    app1:\n      loadBalancer:\n        servers:\n" +
		"          - url: \"http://100.123.0.1:8080\"\n" +
		"    app2:\n      loadBalancer:\n        servers:\n" +
		"          - url: \"http://100.123.0.2:9090\"\n" +
		"    app3:\n      loadBalancer:\n        servers:\n" +
		"          - url: \"http://192.168.1.1:3000\"\n"

	if err := os.WriteFile(traefikDynamic, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	urls := readExistingBackendURLs(&Config{})
	if len(urls) != 3 {
		t.Errorf("expected 3 app URLs, got %d: %v", len(urls), urls)
	}

	checks := map[string]string{
		"app1": "http://100.123.0.1:8080",
		"app2": "http://100.123.0.2:9090",
		"app3": "http://192.168.1.1:3000",
	}
	for appID, expectedURL := range checks {
		if got, ok := urls[appID]; !ok {
			t.Errorf("missing URL for app %q", appID)
		} else if got != expectedURL {
			t.Errorf("app %q: got URL %q, want %q", appID, got, expectedURL)
		}
	}
}

func TestReadExistingBackendURLs_NoFile(t *testing.T) {
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "nonexistent.yml")

	urls := readExistingBackendURLs(&Config{})
	if len(urls) != 0 {
		t.Errorf("expected empty map for missing file, got %v", urls)
	}
}

func TestReadExistingBackendURLs_PreservesAllAppURLs(t *testing.T) {
	// This specifically tests the fix: that sub-keys (loadBalancer, servers)
	// don't overwrite app IDs, and all apps' URLs are preserved.
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")

	// Complex real-world YAML with multiple apps and mixed URLs
	yaml := "http:\n  routers:\n    odysseus:\n      service: odysseus\n" +
		"    hermes-webui:\n      service: hermes-webui\n" +
		"    cmdcenter:\n      service: cmdcenter\n" +
		"  services:\n    odysseus:\n      loadBalancer:\n        servers:\n" +
		"          - url: \"http://100.123.0.125:7000\"\n" +
		"    hermes-webui:\n      loadBalancer:\n        servers:\n" +
		"          - url: \"http://100.123.0.125:8787\"\n" +
		"    cmdcenter:\n      loadBalancer:\n        servers:\n" +
		"          - url: \"http://127.0.0.1:3031\"\n"

	if err := os.WriteFile(traefikDynamic, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	urls := readExistingBackendURLs(&Config{})
	if len(urls) != 3 {
		t.Errorf("expected 3 URLs, got %d. Map: %v", len(urls), urls)
	}

	// All three must be present with correct URLs
	if urls["odysseus"] != "http://100.123.0.125:7000" {
		t.Errorf("odysseus: got %q, want %q", urls["odysseus"], "http://100.123.0.125:7000")
	}
	if urls["hermes-webui"] != "http://100.123.0.125:8787" {
		t.Errorf("hermes-webui: got %q, want %q", urls["hermes-webui"], "http://100.123.0.125:8787")
	}
	if urls["cmdcenter"] != "http://127.0.0.1:3031" {
		t.Errorf("cmdcenter: got %q, want %q", urls["cmdcenter"], "http://127.0.0.1:3031")
	}
}

// ─── checkCertificateForDomain ──────────────────────────────────────────────

func TestCheckCertificateForDomain(t *testing.T) {
	// This function reads from /etc/traefik/acme.json which we can't easily write to.
	// Test the error path (file doesn't exist).
	origDir := traefikConfigDir
	defer func() { traefikConfigDir = origDir }()

	// Use a temp dir with no acme.json
	tmpDir := t.TempDir()
	traefikConfigDir = tmpDir

	found, err := checkCertificateForDomain("example.com")
	if err == nil {
		t.Error("expected error for missing acme.json, got nil")
	}
	if found {
		t.Error("expected found=false for missing file")
	}
}

// ─── DefaultSecurityConfig ──────────────────────────────────────────────────

func TestDefaultSecurityConfig(t *testing.T) {
	cfg := DefaultSecurityConfig()
	if cfg.TokenExpirationDays != 30 {
		t.Errorf("TokenExpirationDays: got %d, want 30", cfg.TokenExpirationDays)
	}
	if cfg.MaxFailedAttempts != 5 {
		t.Errorf("MaxFailedAttempts: got %d, want 5", cfg.MaxFailedAttempts)
	}
	if cfg.RateLimitPerMinute != 60 {
		t.Errorf("RateLimitPerMinute: got %d, want 60", cfg.RateLimitPerMinute)
	}
	if !cfg.RequireHTTPS {
		t.Error("RequireHTTPS should be true")
	}
	if cfg.AllowedIPs == nil {
		t.Error("AllowedIPs should be initialized (not nil)")
	}
	if cfg.AuditLogRetentionDays != 90 {
		t.Errorf("AuditLogRetentionDays: got %d, want 90", cfg.AuditLogRetentionDays)
	}
}

// ─── DryRunDiff ─────────────────────────────────────────────────────────────

func TestDryRunDiff(t *testing.T) {
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")

	// Write an existing config
	existing := "http:\n  routers:\n    app1:\n      service: app1\n" +
		"      entryPoints:\n        - websecure\n" +
		"      tls:\n        certResolver: letsencrypt\n" +
		"        domains:\n          - main: app1.example.com\n" +
		"  services:\n    app1:\n      loadBalancer:\n        servers:\n" +
		"          - url: \"http://127.0.0.1:8080\"\n"
	if err := os.WriteFile(traefikDynamic, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	config := &Config{
		Apps: []App{
			{
				ID:      "app1",
				Name:    "App 1",
				Domain:  "app1.example.com",
				Port:    8080,
				Command: "true",
			},
		},
	}

	diff, err := DryRunDiff(config)
	if err != nil {
		t.Fatalf("DryRunDiff failed: %v", err)
	}

	if diff == "" {
		t.Log("DryRunDiff returned empty (no changes) — acceptable for identical config")
	}
}
