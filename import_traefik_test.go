package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const proxyPathPrefixYAML = `http:
  routers:
    slv2:
      rule: "Host(%s)"
      service: slv2
      entryPoints:
        - websecure
      middlewares:
        - slv2-addprefix
      tls:
        certResolver: letsencrypt
  services:
    slv2:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:3100"
  middlewares:
    slv2-addprefix:
      addPrefix:
        prefix: "/slv2"
`

const proxyBackendURLYAML = `http:
  routers:
    remote:
      rule: "Host(%s)"
      service: remote
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
  services:
    remote:
      loadBalancer:
        servers:
          - url: "http://100.114.4.57:8080"
`

func TestImportTraefikConfig_ProxyServicePathPrefix(t *testing.T) {
	origDynamic := traefikDynamic
	defer func() { traefikDynamic = origDynamic }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")
	t.Setenv("HOME", tmpDir)

	domainRule := "`slv2.example.com`"
	if err := os.WriteFile(traefikDynamic, []byte(fmt.Sprintf(proxyPathPrefixYAML, domainRule)), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	if err := importTraefikConfig(); err != nil {
		t.Fatalf("importTraefikConfig failed: %v", err)
	}

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if len(config.Apps) != 1 {
		t.Fatalf("expected 1 imported app, got %d", len(config.Apps))
	}

	app := config.Apps[0]
	if app.ID != "slv2" {
		t.Errorf("app.ID: got %q, want %q", app.ID, "slv2")
	}
	if app.Domain != "slv2.example.com" {
		t.Errorf("app.Domain: got %q, want %q", app.Domain, "slv2.example.com")
	}
	if app.Port != 3100 {
		t.Errorf("app.Port: got %d, want %d", app.Port, 3100)
	}
	if app.PathPrefix != "/slv2" {
		t.Errorf("app.PathPrefix: got %q, want %q", app.PathPrefix, "/slv2")
	}
	if app.BackendURL != "" {
		t.Errorf("app.BackendURL: got %q, want empty for localhost service", app.BackendURL)
	}
}

func TestImportTraefikConfig_ProxyServiceBackendURL(t *testing.T) {
	origDynamic := traefikDynamic
	defer func() { traefikDynamic = origDynamic }()

	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")
	t.Setenv("HOME", tmpDir)

	domainRule := "`remote.example.com`"
	if err := os.WriteFile(traefikDynamic, []byte(fmt.Sprintf(proxyBackendURLYAML, domainRule)), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	if err := importTraefikConfig(); err != nil {
		t.Fatalf("importTraefikConfig failed: %v", err)
	}

	config, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if len(config.Apps) != 1 {
		t.Fatalf("expected 1 imported app, got %d", len(config.Apps))
	}

	app := config.Apps[0]
	if app.ID != "remote" {
		t.Errorf("app.ID: got %q, want %q", app.ID, "remote")
	}
	if app.Domain != "remote.example.com" {
		t.Errorf("app.Domain: got %q, want %q", app.Domain, "remote.example.com")
	}
	if app.BackendURL != "http://100.114.4.57:8080" {
		t.Errorf("app.BackendURL: got %q, want %q", app.BackendURL, "http://100.114.4.57:8080")
	}
	if app.Port != 0 {
		t.Errorf("app.Port: got %d, want 0 for external backend URL", app.Port)
	}
	if app.PathPrefix != "" {
		t.Errorf("app.PathPrefix: got %q, want empty", app.PathPrefix)
	}
}
