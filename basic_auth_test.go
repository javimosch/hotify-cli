package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestConfig(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", tmpHome)

	// Create the .hotify directory explicitly (saveConfig doesn't create it)
	configPath := filepath.Join(tmpHome, ".hotify", "config.json")
	os.MkdirAll(filepath.Dir(configPath), 0755)

	config := &Config{
		CloudflareToken: "test-token",
		Domain:          "example.com",
		AdminEmail:      "admin@example.com",
		Apps: []App{
			{
				ID:      "testapp",
				Name:    "Test App",
				Domain:  "testapp.example.com",
				Port:    8080,
				Command: "true",
			},
			{
				ID:         "remoteapp",
				Name:       "Remote App",
				Domain:     "remote.example.com",
				Port:       9090,
				Command:    "true",
				BackendURL: "http://10.0.0.1:9090",
			},
		},
		Security: DefaultSecurityConfig(),
	}
	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}
	return tmpHome
}

func TestBasicAuthAdd(t *testing.T) {
	setupTestConfig(t)

	// Add a basic auth user
	err := withConfig(func(c *Config) error {
		for i := range c.Apps {
			if c.Apps[i].ID == "testapp" {
				c.Apps[i].BasicAuth = []string{"admin:$apr1$test$hashvalue"}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withConfig failed: %v", err)
	}

	// Verify
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	found := false
	for _, app := range loaded.Apps {
		if app.ID == "testapp" {
			found = true
			if len(app.BasicAuth) != 1 {
				t.Fatalf("expected 1 basic auth entry, got %d", len(app.BasicAuth))
			}
			if app.BasicAuth[0] != "admin:$apr1$test$hashvalue" {
				t.Errorf("expected 'admin:$apr1$test$hashvalue', got %q", app.BasicAuth[0])
			}
		}
	}
	if !found {
		t.Fatal("testapp not found in config")
	}
}

func TestBasicAuthRemove(t *testing.T) {
	setupTestConfig(t)

	// First add a user
	err := withConfig(func(c *Config) error {
		for i := range c.Apps {
			if c.Apps[i].ID == "testapp" {
				c.Apps[i].BasicAuth = []string{"admin:$apr1$test$hashvalue"}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withConfig failed: %v", err)
	}

	// Now remove
	err = withConfig(func(c *Config) error {
		for i := range c.Apps {
			if c.Apps[i].ID == "testapp" {
				c.Apps[i].BasicAuth = nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withConfig failed: %v", err)
	}

	// Verify
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	for _, app := range loaded.Apps {
		if app.ID == "testapp" {
			if len(app.BasicAuth) != 0 {
				t.Errorf("expected 0 basic auth entries after removal, got %d", len(app.BasicAuth))
			}
		}
	}
}

func TestBasicAuthMultipleUsers(t *testing.T) {
	setupTestConfig(t)

	err := withConfig(func(c *Config) error {
		for i := range c.Apps {
			if c.Apps[i].ID == "testapp" {
				c.Apps[i].BasicAuth = []string{
					"admin:$apr1$a$b",
					"user2:$apr1$c$d",
					"user3:$apr1$e$f",
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withConfig failed: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	for _, app := range loaded.Apps {
		if app.ID == "testapp" {
			if len(app.BasicAuth) != 3 {
				t.Errorf("expected 3 basic auth entries, got %d", len(app.BasicAuth))
			}
		}
	}
}

func TestAppPersistence_BackendURL(t *testing.T) {
	setupTestConfig(t)

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	for _, app := range loaded.Apps {
		if app.ID == "remoteapp" {
			if app.BackendURL != "http://10.0.0.1:9090" {
				t.Errorf("remoteapp backend_url: got %q, want 'http://10.0.0.1:9090'", app.BackendURL)
			}
		}
	}
}

func TestAppPersistence_RateLimit(t *testing.T) {
	setupTestConfig(t)

	// Add rate limit to an app
	err := withConfig(func(c *Config) error {
		for i := range c.Apps {
			if c.Apps[i].ID == "testapp" {
				c.Apps[i].RateLimit = "20,120s"
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withConfig failed: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	for _, app := range loaded.Apps {
		if app.ID == "testapp" {
			if app.RateLimit != "20,120s" {
				t.Errorf("rate limit: got %q, want '20,120s'", app.RateLimit)
			}
		}
	}
}

func TestConfigSecurityDefault(t *testing.T) {
	setupTestConfig(t)

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if loaded.Security.TokenExpirationDays != 30 {
		t.Errorf("TokenExpirationDays: got %d, want 30", loaded.Security.TokenExpirationDays)
	}
	if loaded.Security.MaxFailedAttempts != 5 {
		t.Errorf("MaxFailedAttempts: got %d, want 5", loaded.Security.MaxFailedAttempts)
	}
	if loaded.Security.RateLimitPerMinute != 60 {
		t.Errorf("RateLimitPerMinute: got %d, want 60", loaded.Security.RateLimitPerMinute)
	}
}

func TestEmptyConfigWasCreated(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", tmpHome)

	// Don't save anything — loadConfig should return empty config without error
	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig on fresh HOME should succeed: %v", err)
	}
	if config.CloudflareToken != "" {
		t.Errorf("expected empty token on fresh config")
	}
	if config.Security.TokenExpirationDays != 30 {
		t.Errorf("expected default security config")
	}

	// save empty config
	if err := saveConfig(config); err != nil {
		t.Fatalf("saveConfig on empty config failed: %v", err)
	}

	// re-read
	config2, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig after save failed: %v", err)
	}
	if config2.CloudflareToken != "" {
		t.Errorf("expected empty token after round-trip")
	}
	if !strings.Contains(config2.Domain, "") && config2.Domain != "" {
		t.Errorf("expected empty domain")
	}
}
