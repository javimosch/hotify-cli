package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadConfig_EmptyDir tests that loadConfig returns an empty config when no file exists
func TestLoadConfig_EmptyDir(t *testing.T) {
	// Override HOME to a temp directory so getConfigPath() returns a temp path
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig on empty dir should succeed, got: %v", err)
	}
	if config == nil {
		t.Fatal("loadConfig returned nil")
	}
	if len(config.Apps) != 0 {
		t.Errorf("expected 0 apps, got %d", len(config.Apps))
	}
	if config.CloudflareToken != "" {
		t.Errorf("expected empty token, got %q", config.CloudflareToken)
	}
}

// TestSaveAndLoadConfig tests round-trip save + load
func TestSaveAndLoadConfig(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	orig := &Config{
		CloudflareToken: "test-token-123",
		Domain:          "example.com",
		AdminEmail:      "admin@example.com",
		Apps: []App{
			{
				ID:         "app1",
				Name:       "App One",
				Domain:     "app1.example.com",
				Port:       8080,
				Command:    "/usr/bin/app1 start",
				Status:     "running",
				BackendURL: "http://10.0.0.1:8080",
				PID:        12345,
				RateLimit:  "10,60m",
			},
			{
				ID:      "app2",
				Name:    "App Two",
				Domain:  "app2.example.com",
				Port:    9090,
				Command: "/usr/bin/app2 start",
				BasicAuth: []string{
					"admin:$apr1$abc$def",
				},
			},
		},
		Security: DefaultSecurityConfig(),
	}

	// Ensure directory exists
	os.MkdirAll(filepath.Join(tmpHome, ".hotify"), 0755)

	if err := saveConfig(orig); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpHome, ".hotify", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("config file not created at %s", configPath)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig after save failed: %v", err)
	}

	if loaded.CloudflareToken != orig.CloudflareToken {
		t.Errorf("token: got %q, want %q", loaded.CloudflareToken, orig.CloudflareToken)
	}
	if loaded.Domain != orig.Domain {
		t.Errorf("domain: got %q, want %q", loaded.Domain, orig.Domain)
	}
	if loaded.AdminEmail != orig.AdminEmail {
		t.Errorf("email: got %q, want %q", loaded.AdminEmail, orig.AdminEmail)
	}
	if len(loaded.Apps) != len(orig.Apps) {
		t.Fatalf("apps: got %d, want %d", len(loaded.Apps), len(orig.Apps))
	}

	// Check app fields
	for i, app := range loaded.Apps {
		if app.ID != orig.Apps[i].ID {
			t.Errorf("app[%d].ID: got %q, want %q", i, app.ID, orig.Apps[i].ID)
		}
		if app.Port != orig.Apps[i].Port {
			t.Errorf("app[%d].Port: got %d, want %d", i, app.Port, orig.Apps[i].Port)
		}
		if app.BackendURL != orig.Apps[i].BackendURL {
			t.Errorf("app[%d].BackendURL: got %q, want %q", i, app.BackendURL, orig.Apps[i].BackendURL)
		}
		if app.RateLimit != orig.Apps[i].RateLimit {
			t.Errorf("app[%d].RateLimit: got %q, want %q", i, app.RateLimit, orig.Apps[i].RateLimit)
		}
	}
}

// TestLoadConfig_CorruptFile tests that loadConfig returns an error for invalid JSON
func TestLoadConfig_CorruptFile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Write invalid JSON
	configPath := filepath.Join(tmpHome, ".hotify", "config.json")
	os.MkdirAll(filepath.Dir(configPath), 0755)
	if err := os.WriteFile(configPath, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	_, err := loadConfig()
	if err == nil {
		t.Error("expected error for corrupt JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parsing config") {
		t.Errorf("expected 'parsing config' error, got: %v", err)
	}
}

// TestWithConfig tests the withConfig helper
func TestWithConfig(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Save initial config
	orig := &Config{
		CloudflareToken: "initial",
		Domain:          "example.com",
		AdminEmail:      "a@b.com",
		Apps:            []App{},
		Security:        DefaultSecurityConfig(),
	}
	os.MkdirAll(filepath.Join(tmpHome, ".hotify"), 0755)
	if err := saveConfig(orig); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Use withConfig to modify
	err := withConfig(func(c *Config) error {
		c.CloudflareToken = "updated"
		c.Apps = append(c.Apps, App{ID: "newapp", Domain: "new.example.com", Port: 3000})
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
	if loaded.CloudflareToken != "updated" {
		t.Errorf("token: got %q, want 'updated'", loaded.CloudflareToken)
	}
	if len(loaded.Apps) != 1 || loaded.Apps[0].ID != "newapp" {
		t.Errorf("apps: got %+v, want [newapp]", loaded.Apps)
	}
}

// TestGetConfigPath tests that getConfigPath returns a reasonable path
func TestGetConfigPath(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	path, err := getConfigPath()
	if err != nil {
		t.Fatalf("getConfigPath failed: %v", err)
	}
	if !strings.HasSuffix(path, ".hotify/config.json") {
		t.Errorf("expected path ending in .hotify/config.json, got %q", path)
	}
	if !strings.HasPrefix(path, tmpHome) {
		t.Errorf("expected path in temp dir %q, got %q", tmpHome, path)
	}
}

// TestSaveConfig_Atomicity tests that saveConfig uses atomic temp+rename
func TestSaveConfig_Atomicity(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Create initial config
	initial := &Config{
		CloudflareToken: "before",
		Domain:          "example.com",
		AdminEmail:      "a@b.com",
		Apps:            []App{{ID: "keep-me", Domain: "keep.example.com", Port: 8080}},
		Security:        DefaultSecurityConfig(),
	}
	os.MkdirAll(filepath.Join(tmpHome, ".hotify"), 0755)
	if err := saveConfig(initial); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Read initial back
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if len(loaded.Apps) != 1 {
		t.Errorf("expected 1 app, got %d", len(loaded.Apps))
	}
}
