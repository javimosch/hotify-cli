package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildDynamicYAML_ProxyService verifies that buildDynamicYAML generates a
// Traefik router, service, and addPrefix middleware for sl-cli-style proxy
// services that use backend_url and path_prefix.
func TestBuildDynamicYAML_ProxyService(t *testing.T) {
	origPath := traefikDynamic
	defer func() { traefikDynamic = origPath }()
	tmpDir := t.TempDir()
	traefikDynamic = filepath.Join(tmpDir, "dynamic.yml")

	// Ensure the file exists so readExistingBackendURLs has a stable target.
	if err := os.WriteFile(traefikDynamic, []byte("http:\n  services:\n"), 0644); err != nil {
		t.Fatalf("failed to write temp dynamic.yml: %v", err)
	}

	config := &Config{
		Apps: []App{
			{
				ID:         "slv2",
				Name:       "SL v2",
				Domain:     "slv2.example.com",
				Port:       3100,
				Command:    "sl-cli serve",
				BackendURL: "http://127.0.0.1:3100",
				PathPrefix: "/slv2",
			},
		},
	}

	yaml, err := buildDynamicYAML(config)
	if err != nil {
		t.Fatalf("buildDynamicYAML failed: %v", err)
	}

	if !strings.Contains(yaml, `url: "http://127.0.0.1:3100"`) {
		t.Errorf("missing backend URL in service:\n%s", yaml)
	}
	if !strings.Contains(yaml, "slv2-addprefix:") {
		t.Errorf("missing addPrefix middleware:\n%s", yaml)
	}
	if !strings.Contains(yaml, `prefix: "/slv2"`) {
		t.Errorf("missing path prefix:\n%s", yaml)
	}
	if !strings.Contains(yaml, "middlewares:") {
		t.Errorf("missing middlewares on router:\n%s", yaml)
	}
	if !strings.Contains(yaml, "- slv2-addprefix") {
		t.Errorf("router not wired to addPrefix middleware:\n%s", yaml)
	}
}
