package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// CloudflareDNSListResponse is used for listing existing DNS records.
type CloudflareDNSListResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Content string `json:"content"`
	} `json:"result"`
}

func getZoneID(domain, token, email string) (string, error) {
	// Extract registrable domain (second-to-last label, e.g. "intrane.fr" from "sub.intrane.fr")
	// Split by dots and take the last two parts.
	parts := strings.Split(domain, ".")
	baseDomain := domain
	if len(parts) >= 2 {
		baseDomain = strings.Join(parts[len(parts)-2:], ".")
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", baseDomain)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

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

// existingDNSRecord returns (recordID, currentIP, found) for the first A record
// matching `domain` in the given zone. Returns ("", "", false) if not found.
func existingDNSRecord(domain, zoneID, token, email string) (string, string, bool) {
	url := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=A&name=%s",
		zoneID, domain,
	)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("X-Auth-Email", email)
	req.Header.Set("X-Auth-Key", token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var listResp CloudflareDNSListResponse
	if err := json.Unmarshal(body, &listResp); err != nil || !listResp.Success {
		return "", "", false
	}
	if len(listResp.Result) == 0 {
		return "", "", false
	}
	r := listResp.Result[0]
	return r.ID, r.Content, true
}

// updateDNSRecord updates an existing A record to a new IP.
func updateDNSRecord(domain, ip, recordID, zoneID, token, email string) error {
	url := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s",
		zoneID, recordID,
	)
	payload := map[string]interface{}{
		"type":    "A",
		"name":    domain,
		"content": ip,
		"ttl":     1,
		"proxied": false,
	}
	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("error creating update request: %v", err)
	}
	req.Header.Set("X-Auth-Email", email)
	req.Header.Set("X-Auth-Key", token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error updating DNS record: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var dnsResp CloudflareDNSResponse
	if err := json.Unmarshal(body, &dnsResp); err != nil || !dnsResp.Success {
		return fmt.Errorf("failed to update DNS record: %s", string(body))
	}
	return nil
}

// setupDNSRecord creates an A record. TC4: checks for existing record first;
// skips if IP matches, updates if IP differs, creates if absent.
func setupDNSRecord(domain, ip, zoneID, token, email string) error {
	// TC4: existence check
	if existingID, existingIP, found := existingDNSRecord(domain, zoneID, token, email); found {
		if existingIP == ip {
			// Record exists with correct IP — nothing to do
			fmt.Printf("ℹ️  DNS record already exists for %s → %s (skipping creation)\n", domain, ip)
			return nil
		}
		// Record exists but IP differs — update it
		fmt.Printf("⚠️  DNS record for %s exists with IP %s, updating to %s\n", domain, existingIP, ip)
		return updateDNSRecord(domain, ip, existingID, zoneID, token, email)
	}

	// No existing record — create it
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	payload := map[string]interface{}{
		"type":    "A",
		"name":    domain,
		"content": ip,
		"ttl":     1,
		"proxied": false,
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

	fmt.Printf("✅ DNS record set for %s → %s\n", app.Domain, serverIP)
	return nil
}
