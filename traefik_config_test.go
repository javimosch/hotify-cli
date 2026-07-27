package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetupTraefikConfig_HTTPChallenge tests the basic config generation with HTTP challenge
func TestSetupTraefikConfig_HTTPChallenge(t *testing.T) {
	tmpDir := t.TempDir()

	// Override all traefik path vars to point to temp dir
	origConfigDir := traefikConfigDir
	origMain := traefikMain
	origEnv := traefikEnv
	traefikConfigDir = tmpDir
	traefikMain = filepath.Join(tmpDir, "traefik.yml")
	traefikEnv = filepath.Join(tmpDir, "cloudflare.env")
	defer func() {
		traefikConfigDir = origConfigDir
		traefikMain = origMain
		traefikEnv = origEnv
	}()

	config := &Config{
		CloudflareToken: "test-cf-token",
		Domain:          "example.com",
		AdminEmail:      "admin@example.com",
	}

	err := setupTraefikConfig(config, ChallengeHTTP, false, true)
	if err != nil {
		t.Fatalf("setupTraefikConfig failed: %v", err)
	}

	// Check traefik.yml was created
	mainData, err := os.ReadFile(traefikMain)
	if err != nil {
		t.Fatalf("failed to read traefik.yml: %v", err)
	}
	mainContent := string(mainData)

	if !strings.Contains(mainContent, "admin@example.com") {
		t.Error("traefik.yml missing admin email")
	}
	if !strings.Contains(mainContent, "httpChallenge") {
		t.Error("traefik.yml missing http challenge")
	}
	if !strings.Contains(mainContent, "entryPoint: web") {
		t.Error("traefik.yml missing HTTP challenge entryPoint")
	}
	if !strings.Contains(mainContent, "redirections") {
		t.Error("traefik.yml missing redirect config")
	}

	// Check cloudflare.env was created
	envData, err := os.ReadFile(traefikEnv)
	if err != nil {
		t.Fatalf("failed to read cloudflare.env: %v", err)
	}
	envContent := string(envData)
	if !strings.Contains(envContent, "admin@example.com") {
		t.Error("cloudflare.env missing email")
	}
	if !strings.Contains(envContent, "test-cf-token") {
		t.Error("cloudflare.env missing token")
	}

	// Check acme.json was created
	acmePath := filepath.Join(tmpDir, "acme.json")
	if _, err := os.Stat(acmePath); os.IsNotExist(err) {
		t.Error("acme.json was not created")
	}
}

// TestSetupTraefikConfig_DNSChallenge tests DNS challenge config
func TestSetupTraefikConfig_DNSChallenge(t *testing.T) {
	tmpDir := t.TempDir()

	origConfigDir := traefikConfigDir
	origMain := traefikMain
	origEnv := traefikEnv
	traefikConfigDir = tmpDir
	traefikMain = filepath.Join(tmpDir, "traefik.yml")
	traefikEnv = filepath.Join(tmpDir, "cloudflare.env")
	defer func() {
		traefikConfigDir = origConfigDir
		traefikMain = origMain
		traefikEnv = origEnv
	}()

	config := &Config{
		CloudflareToken: "dns-token",
		Domain:          "example.com",
		AdminEmail:      "dns@example.com",
	}

	err := setupTraefikConfig(config, ChallengeDNS, false, true)
	if err != nil {
		t.Fatalf("setupTraefikConfig failed: %v", err)
	}

	mainData, err := os.ReadFile(traefikMain)
	if err != nil {
		t.Fatalf("failed to read traefik.yml: %v", err)
	}
	mainContent := string(mainData)

	if !strings.Contains(mainContent, "dnsChallenge") {
		t.Error("traefik.yml missing dns challenge")
	}
	if !strings.Contains(mainContent, "cloudflare") {
		t.Error("traefik.yml missing cloudflare provider")
	}
	if !strings.Contains(mainContent, "delayBeforeCheck") {
		t.Error("traefik.yml missing delayBeforeCheck")
	}
}

// TestSetupTraefikConfig_NoRedirect tests config without HTTP redirect
func TestSetupTraefikConfig_NoRedirect(t *testing.T) {
	tmpDir := t.TempDir()

	origConfigDir := traefikConfigDir
	origMain := traefikMain
	origEnv := traefikEnv
	traefikConfigDir = tmpDir
	traefikMain = filepath.Join(tmpDir, "traefik.yml")
	traefikEnv = filepath.Join(tmpDir, "cloudflare.env")
	defer func() {
		traefikConfigDir = origConfigDir
		traefikMain = origMain
		traefikEnv = origEnv
	}()

	config := &Config{
		CloudflareToken: "no-redirect-token",
		Domain:          "example.com",
		AdminEmail:      "no-redirect@example.com",
	}

	err := setupTraefikConfig(config, ChallengeHTTP, false, false)
	if err != nil {
		t.Fatalf("setupTraefikConfig failed: %v", err)
	}

	mainData, err := os.ReadFile(traefikMain)
	if err != nil {
		t.Fatalf("failed to read traefik.yml: %v", err)
	}
	mainContent := string(mainData)

	if strings.Contains(mainContent, "redirections") {
		t.Error("traefik.yml should NOT contain redirect when disabled")
	}
}

// TestSetupTraefikConfig_WithDocker tests config with Docker provider
func TestSetupTraefikConfig_WithDocker(t *testing.T) {
	tmpDir := t.TempDir()

	origConfigDir := traefikConfigDir
	origMain := traefikMain
	origEnv := traefikEnv
	traefikConfigDir = tmpDir
	traefikMain = filepath.Join(tmpDir, "traefik.yml")
	traefikEnv = filepath.Join(tmpDir, "cloudflare.env")
	defer func() {
		traefikConfigDir = origConfigDir
		traefikMain = origMain
		traefikEnv = origEnv
	}()

	config := &Config{
		CloudflareToken: "docker-token",
		Domain:          "example.com",
		AdminEmail:      "docker@example.com",
	}

	err := setupTraefikConfig(config, ChallengeHTTP, true, true)
	if err != nil {
		t.Fatalf("setupTraefikConfig failed: %v", err)
	}

	mainData, err := os.ReadFile(traefikMain)
	if err != nil {
		t.Fatalf("failed to read traefik.yml: %v", err)
	}
	mainContent := string(mainData)

	if !strings.Contains(mainContent, "docker") {
		t.Error("traefik.yml missing Docker provider")
	}
	if !strings.Contains(mainContent, "/var/run/docker.sock") {
		t.Error("traefik.yml missing Docker socket path")
	}
	if !strings.Contains(mainContent, "exposedByDefault: false") {
		t.Error("traefik.yml missing exposedByDefault: false")
	}
}

// TestSetupTraefikConfig_ValidationError tests that validation catches missing fields
func TestSetupTraefikConfig_ValidationError(t *testing.T) {
	// Test with nil config (will panic) — skip; test missing fields
	config := &Config{
		CloudflareToken: "",
		Domain:          "",
		AdminEmail:      "",
	}

	err := setupTraefikConfig(config, ChallengeHTTP, false, true)
	if err == nil {
		t.Error("expected validation error for empty config")
	} else if !strings.Contains(err.Error(), "admin_email") {
		t.Errorf("expected error about admin_email, got: %v", err)
	}
}

// TestSetupTraefikConfig_AcmeJsonExists tests that acme.json is not overwritten
func TestSetupTraefikConfig_AcmeJsonExists(t *testing.T) {
	tmpDir := t.TempDir()

	origConfigDir := traefikConfigDir
	origMain := traefikMain
	origEnv := traefikEnv
	traefikConfigDir = tmpDir
	traefikMain = filepath.Join(tmpDir, "traefik.yml")
	traefikEnv = filepath.Join(tmpDir, "cloudflare.env")
	defer func() {
		traefikConfigDir = origConfigDir
		traefikMain = origMain
		traefikEnv = origEnv
	}()

	// Pre-create acme.json with custom content
	acmePath := filepath.Join(tmpDir, "acme.json")
	acmeContent := `{"custom": "data"}`
	if err := os.WriteFile(acmePath, []byte(acmeContent), 0600); err != nil {
		t.Fatalf("failed to pre-create acme.json: %v", err)
	}

	config := &Config{
		CloudflareToken: "token",
		Domain:          "example.com",
		AdminEmail:      "admin@example.com",
	}

	if err := setupTraefikConfig(config, ChallengeHTTP, false, true); err != nil {
		t.Fatalf("setupTraefikConfig failed: %v", err)
	}

	// Verify acme.json was NOT overwritten
	data, err := os.ReadFile(acmePath)
	if err != nil {
		t.Fatalf("failed to read acme.json: %v", err)
	}
	if string(data) != acmeContent {
		t.Errorf("acme.json was overwritten: got %q, want %q", string(data), acmeContent)
	}
}
