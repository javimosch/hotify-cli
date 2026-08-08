package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetupTraefikAppRemotePropagation(t *testing.T) {
	var captured map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/remote/apps/slv2/setup-traefik" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":          true,
			"app_id":           "slv2",
			"challenge_type":   "http",
			"redirect_enabled": true,
			"backend_url":      "http://127.0.0.1:3100",
			"path_prefix":      "/slv2",
			"action":           "traefik_configured",
		})
	}))
	defer server.Close()

	client := &DeploymentClient{
		HTTPClient: &HTTPClient{
			BaseURL:   server.URL,
			AuthToken: "test-token",
		},
	}

	result, err := client.SetupTraefikApp("slv2", "http", false)
	if err != nil {
		t.Fatalf("SetupTraefikApp error: %v", err)
	}

	if captured["challenge_type"] != "http" {
		t.Errorf("expected challenge_type http, got %v", captured["challenge_type"])
	}
	if captured["no_redirect"] != false {
		t.Errorf("expected no_redirect false, got %v", captured["no_redirect"])
	}

	if result == nil {
		t.Fatal("expected non-nil result map")
	}
	if result["backend_url"] != "http://127.0.0.1:3100" {
		t.Errorf("backend_url: got %v", result["backend_url"])
	}
	if result["path_prefix"] != "/slv2" {
		t.Errorf("path_prefix: got %v", result["path_prefix"])
	}
	if result["app_id"] != "slv2" {
		t.Errorf("app_id: got %v", result["app_id"])
	}

	// Error path
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "boom")
	}))
	defer errServer.Close()

	client.HTTPClient.BaseURL = errServer.URL
	if _, err := client.SetupTraefikApp("slv2", "http", false); err == nil {
		t.Error("expected error for 500 response")
	}
}
