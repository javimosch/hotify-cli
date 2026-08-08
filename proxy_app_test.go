package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runGoRun invokes the current hotify-cli source with the given subcommand/args
// and a temporary HOME, returning the stdout output. It is used for end-to-end
// CLI tests without building a release binary.
func runGoRun(t *testing.T, home string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	cmd.Dir = ""
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go run failed: %v\nstdout: %s\nstderr: %s", err, string(out), stderr)
	}
	return out
}

func TestProxyAppSetup_WithoutCmd(t *testing.T) {
	tmpHome := setupTestConfig(t)

	out := runGoRun(t, tmpHome,
		"setup",
		"--id", "proxyapp",
		"--name", "Proxy App",
		"--domain", "proxy",
		"--port", "8080",
		"--backend-url", "http://10.0.0.1:8080",
		"--path-prefix", "/slv2",
	)

	var result CommandResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal setup output: %v\noutput: %s", err, string(out))
	}
	if !result.Success {
		t.Fatalf("setup failed: %+v", result.Error)
	}
	if result.Data["backend_url"] != "http://10.0.0.1:8080" {
		t.Errorf("backend_url: got %v, want %q", result.Data["backend_url"], "http://10.0.0.1:8080")
	}
	if result.Data["path_prefix"] != "/slv2" {
		t.Errorf("path_prefix: got %v, want %q", result.Data["path_prefix"], "/slv2")
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	var found bool
	for _, app := range cfg.Apps {
		if app.ID == "proxyapp" {
			found = true
			if app.BackendURL != "http://10.0.0.1:8080" {
				t.Errorf("config backend_url: got %q, want %q", app.BackendURL, "http://10.0.0.1:8080")
			}
			if app.PathPrefix != "/slv2" {
				t.Errorf("config path_prefix: got %q, want %q", app.PathPrefix, "/slv2")
			}
			if app.Command != "" {
				t.Errorf("config command: got %q, want empty for proxy app", app.Command)
			}
		}
	}
	if !found {
		t.Errorf("proxyapp not found in config")
	}
}

func TestProxyAppList_IncludesProxyFields(t *testing.T) {
	tmpHome := setupTestConfig(t)

	// Create a proxy app
	_ = runGoRun(t, tmpHome,
		"setup",
		"--id", "proxylist",
		"--name", "Proxy List",
		"--domain", "proxylist",
		"--port", "8080",
		"--backend-url", "http://10.0.0.1:8080",
		"--path-prefix", "/api",
	)

	out := runGoRun(t, tmpHome, "list")

	var result CommandResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal list output: %v\noutput: %s", err, string(out))
	}
	if !result.Success {
		t.Fatalf("list failed: %+v", result.Error)
	}
	apps, ok := result.Data["apps"].([]interface{})
	if !ok {
		t.Fatalf("expected apps list, got %T", result.Data["apps"])
	}
	for _, raw := range apps {
		app, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected app map, got %T", raw)
		}
		if app["id"] != "proxylist" {
			continue
		}
		if app["backend_url"] != "http://10.0.0.1:8080" {
			t.Errorf("list backend_url: got %v, want %q", app["backend_url"], "http://10.0.0.1:8080")
		}
		if app["path_prefix"] != "/api" {
			t.Errorf("list path_prefix: got %v, want %q", app["path_prefix"], "/api")
		}
		return
	}
	t.Errorf("proxylist not found in list output")
}

func TestRemoteAppSetup_WithoutCmdAndProxyFields(t *testing.T) {
	tmpHome := setupTestConfig(t)
	// Ensure HOME is set for the handler's loadConfig/saveConfig and audit logger
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	os.Setenv("HOME", tmpHome)

	var err error
	auditLogger, err = NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "Remote Proxy",
		"domain":      "remoteproxy",
		"port":        8080,
		"backend_url": "http://10.0.0.2:8080",
		"path_prefix": "/v2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/remote/apps/remoteproxy/config-setup", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handleRemoteAppSetupAPI(rec, req, "remoteproxy")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v\n%s", err, rec.Body.String())
	}
	if result["backend_url"] != "http://10.0.0.2:8080" {
		t.Errorf("response backend_url: got %v, want %q", result["backend_url"], "http://10.0.0.2:8080")
	}
	if result["path_prefix"] != "/v2" {
		t.Errorf("response path_prefix: got %v, want %q", result["path_prefix"], "/v2")
	}

	// Verify the app was actually saved
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	var found bool
	for _, app := range cfg.Apps {
		if app.ID == "remoteproxy" {
			found = true
			if app.Command != "" {
				t.Errorf("remote proxy command: got %q, want empty", app.Command)
			}
		}
	}
	if !found {
		t.Errorf("remoteproxy not saved to config")
	}
}

// TestProxyAppSetup_RequiresCmdWithoutBackendURL ensures we did not accidentally
// make --cmd optional for non-proxy apps.
func TestProxyAppSetup_RequiresCmdWithoutBackendURL(t *testing.T) {
	setupTestConfig(t)

	out, err := exec.Command("go", "run", ".", "setup",
		"--id", "nope",
		"--name", "No Backend",
		"--domain", "nope",
		"--port", "8080",
	).Output()

	if err == nil {
		t.Fatalf("expected setup to fail without cmd/backend-url, got output: %s", string(out))
	}
	stderr := ""
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = string(exitErr.Stderr)
	}
	combined := string(out) + stderr
	if !strings.Contains(combined, "--cmd is optional when --backend-url is set") &&
		!strings.Contains(combined, "backend_url") {
		t.Errorf("expected validation message mentioning backend-url, got:\n%s", combined)
	}

	// The config should not have been written
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	for _, app := range cfg.Apps {
		if app.ID == "nope" {
			t.Errorf("app 'nope' should not have been created")
		}
	}
}
