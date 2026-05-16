package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type CloudflareZoneResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
}

type CloudflareDNSResponse struct {
	Success bool `json:"success"`
	Result  struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
}

func getZoneID(domain, token, email string) (string, error) {
	// Extract base domain (remove subdomain)
	baseDomain := domain
	for i := len(domain) - 1; i >= 0; i-- {
		if domain[i] == '.' {
			baseDomain = domain[i+1:]
			break
		}
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", baseDomain)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	// Try legacy API format first
	req.Header.Set("X-Auth-Email", email)
	req.Header.Set("X-Auth-Key", token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	var zoneResp CloudflareZoneResponse
	if err := json.Unmarshal(body, &zoneResp); err != nil {
		return "", fmt.Errorf("error parsing response: %v", err)
	}

	if !zoneResp.Success || len(zoneResp.Result) == 0 {
		return "", fmt.Errorf("no zone found for domain %s", baseDomain)
	}

	return zoneResp.Result[0].ID, nil
}

func setupDNSRecord(domain, ip, zoneID, token, email string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)

	payload := map[string]interface{}{
		"type":     "A",
		"name":     domain,
		"content":  ip,
		"ttl":      1,
		"proxied":  false,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("X-Auth-Email", email)
	req.Header.Set("X-Auth-Key", token)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(bytes.NewReader(payloadBytes))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %v", err)
	}

	var dnsResp CloudflareDNSResponse
	if err := json.Unmarshal(body, &dnsResp); err != nil {
		return fmt.Errorf("error parsing response: %v", err)
	}

	if !dnsResp.Success {
		return fmt.Errorf("failed to create DNS record: %s", string(body))
	}

	return nil
}

func setupDNSForApp(appID, serverIP string) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	var app *App
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			app = &config.Apps[i]
			break
		}
	}

	if app == nil {
		return fmt.Errorf("app with ID '%s' not found", appID)
	}

	zoneID, err := getZoneID(app.Domain, config.CloudflareToken, config.AdminEmail)
	if err != nil {
		return fmt.Errorf("error getting zone ID: %v", err)
	}

	if err := setupDNSRecord(app.Domain, serverIP, zoneID, config.CloudflareToken, config.AdminEmail); err != nil {
		return fmt.Errorf("error setting up DNS record: %v", err)
	}

	fmt.Printf("✅ DNS record created for %s -> %s\n", app.Domain, serverIP)
	return nil
}
