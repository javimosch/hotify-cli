package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupRoutingForApp(t *testing.T) {
	// Isolate the test from the real home directory and Traefik paths.
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	origDynamic := traefikDynamic
	origMain := traefikMain
	defer func() {
		traefikDynamic = origDynamic
		traefikMain = origMain
	}()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")
	traefikMain = filepath.Join(tmpDir, "traefik.yml")

	mainContent := `global:
  checkNewVersion: true
entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"
`
	if err := os.WriteFile(traefikMain, []byte(mainContent), 0644); err != nil {
		t.Fatalf("write traefik.yml: %v", err)
	}

	foreignContent := `http:
  routers:
    foreign-router:
      rule: "Host(` + "`" + `foreign.example.com` + "`" + `)"
      service: foreign-service
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
  services:
    foreign-service:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:9999"
`
	if err := os.WriteFile(traefikDynamic, []byte(foreignContent), 0644); err != nil {
		t.Fatalf("write existing dynamic.yml: %v", err)
	}

	cfg := &Config{
		Domain:     "example.com",
		AdminEmail: "admin@example.com",
		TraefikMode: "file",
		Apps: []App{
			{
				ID:         "slv2",
				Name:       "SL v2",
				Domain:     "slv2.example.com",
				Port:       3100,
				Command:    "sl-cli server",
				BackendURL: "http://100.114.4.57:3100",
				PathPrefix: "/slv2",
			},
		},
	}
	if err := os.MkdirAll(filepath.Join(tmpHome, ".hotify"), 0755); err != nil {
		t.Fatalf("mkdir .hotify: %v", err)
	}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if err := setupRoutingForApp("slv2", false); err != nil {
		t.Fatalf("setupRoutingForApp failed: %v", err)
	}

	// dynamic.yml was regenerated and contains the proxy service.
	data, err := os.ReadFile(traefikDynamic)
	if err != nil {
		t.Fatalf("read dynamic.yml: %v", err)
	}
	out := string(data)

	if !strings.Contains(out, "slv2:") {
		t.Error("dynamic.yml missing slv2 router/service")
	}
	if !strings.Contains(out, "Host(`slv2.example.com`)") {
		t.Error("dynamic.yml missing Host rule for slv2")
	}
	if !strings.Contains(out, `url: "http://100.114.4.57:3100"`) {
		t.Error("dynamic.yml missing backend_url for slv2")
	}
	if !strings.Contains(out, "slv2-addprefix:") {
		t.Error("dynamic.yml missing addPrefix middleware for slv2")
	}
	if !strings.Contains(out, `prefix: "/slv2"`) {
		t.Error("dynamic.yml missing path prefix /slv2")
	}

	// Foreign config was preserved.
	if !strings.Contains(out, "foreign-router:") {
		t.Error("foreign router was not preserved")
	}
	if !strings.Contains(out, "foreign-service:") {
		t.Error("foreign service was not preserved")
	}

	// traefik.yml was not modified.
	mainData, err := os.ReadFile(traefikMain)
	if err != nil {
		t.Fatalf("read traefik.yml: %v", err)
	}
	if string(mainData) != mainContent {
		t.Error("traefik.yml was unexpectedly modified by setup-routing")
	}
}

func TestSetupRoutingForApp_AppNotFound(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	cfg := &Config{
		Domain:     "example.com",
		AdminEmail: "admin@example.com",
		Apps:       []App{},
	}
	if err := os.MkdirAll(filepath.Join(tmpHome, ".hotify"), 0755); err != nil {
		t.Fatalf("mkdir .hotify: %v", err)
	}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	err := setupRoutingForApp("missing", false)
	if err == nil {
		t.Fatal("expected error for missing app")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %q", err.Error())
	}
}
