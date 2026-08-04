package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	hotifyDir := filepath.Join(tmpDir, ".hotify")
	if err := os.MkdirAll(hotifyDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := []byte(`{"domain":"example.com","cloudflare_token":"","admin_email":"admin@example.com","apps":[]}`)
	if err := os.WriteFile(filepath.Join(hotifyDir, "config.json"), cfg, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var err error
	auditLogger, err = NewAuditLogger()
	if err != nil {
		t.Fatalf("audit logger: %v", err)
	}
	return tmpDir
}

func TestRemoteAppSetupProxyNoCmd(t *testing.T) {
	setupTestHome(t)

	body := map[string]interface{}{
		"name":        "SL v2",
		"domain":      "slv2",
		"port":        3100,
		"backend_url": "http://127.0.0.1:3100",
		"path_prefix": "/slv2",
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/remote/apps/slv2/config-setup", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handleRemoteAppSetupAPI(rec, req, "slv2")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result["success"].(bool) != true {
		t.Fatalf("expected success, got %v", result)
	}
	if result["backend_url"] != "http://127.0.0.1:3100" {
		t.Errorf("backend_url: got %v, want proxy URL", result["backend_url"])
	}
	if result["path_prefix"] != "/slv2" {
		t.Errorf("path_prefix: got %v, want /slv2", result["path_prefix"])
	}

	// cmd should be empty for externally managed proxy apps
	if result["cmd"] != nil && result["cmd"] != "" {
		t.Errorf("cmd should be empty for proxy apps, got %v", result["cmd"])
	}

	// Verify the persisted config contains the new app
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	var found bool
	for _, app := range cfg.Apps {
		if app.ID == "slv2" {
			found = true
			if app.Command != "" {
				t.Errorf("persisted cmd should be empty, got %q", app.Command)
			}
			if app.BackendURL != "http://127.0.0.1:3100" {
				t.Errorf("persisted backend_url mismatch: %q", app.BackendURL)
			}
			if app.PathPrefix != "/slv2" {
				t.Errorf("persisted path_prefix mismatch: %q", app.PathPrefix)
			}
		}
	}
	if !found {
		t.Errorf("app slv2 not found in persisted config")
	}
}

func TestRemoteAppSetupRequiresCmdWithoutBackend(t *testing.T) {
	setupTestHome(t)

	body := map[string]interface{}{
		"name":   "My App",
		"domain": "myapp",
		"port":   3000,
	}
	payload, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/remote/apps/myapp/config-setup", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handleRemoteAppSetupAPI(rec, req, "myapp")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	bodyStr, _ := io.ReadAll(rec.Body)
	if !bytes.Contains(bodyStr, []byte("cmd")) {
		t.Errorf("expected error message to mention missing cmd, got %s", bodyStr)
	}
}
