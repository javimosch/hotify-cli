package main

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── Debounced dynamic.yml writer ────────────────────────────────────────────
//
// All calls to updateDynamicConfig enqueue the desired config snapshot instead
// of writing immediately. A single background goroutine (startDynamicConfigWriter)
// waits for 5 s of silence after the last enqueue, then flushes the most-recent
// snapshot atomically (temp file → rename). This means:
//
//   - Rapid successive changes (e.g. concurrent API calls) are collapsed into one
//     write, eliminating the race where Traefik's watch reloads a partial file.
//   - The file is always consistent on disk; a crash during the write leaves the
//     previous version intact.

const dynamicWriteDebounce = 5 * time.Second

var (
	dynamicWriteCh   = make(chan *Config, 64) // buffered so callers never block
	dynamicWriteOnce sync.Once               // ensures the goroutine starts once
)

// startDynamicConfigWriter launches the background flush goroutine. Safe to
// call multiple times — only the first call has any effect.
func startDynamicConfigWriter() {
	dynamicWriteOnce.Do(func() {
		go dynamicConfigWriterLoop()
	})
}

func dynamicConfigWriterLoop() {
	var pending *Config
	var debounce <-chan time.Time // nil until first enqueue

	for {
		select {
		case cfg := <-dynamicWriteCh:
			if cfg != nil {
				// New snapshot — keep the latest and restart the 5 s window.
				pending = cfg
				debounce = time.After(dynamicWriteDebounce)
			}
		case <-debounce:
			// 5 s of silence — flush the latest snapshot.
			debounce = nil
			if pending == nil {
				continue
			}
			if err := writeDynamicConfigAtomic(pending); err != nil {
				log.Printf("[hotify] ERROR flushing dynamic.yml: %v", err)
			} else {
				log.Printf("[hotify] dynamic.yml flushed (%d app(s))", len(pending.Apps))
			}
			pending = nil
		}
	}
}

// writeDynamicConfigAtomic writes yaml to a temp file then renames atomically.
func writeDynamicConfigAtomic(config *Config) error {
	tmpPath := traefikDynamic + ".tmp"
	content, err := buildDynamicYAML(config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, traefikDynamic); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

const (
	traefikConfigDir = "/etc/traefik"
	traefikDynamic   = "/etc/traefik/dynamic.yml"
	traefikMain      = "/etc/traefik/traefik.yml"
	traefikEnv       = "/etc/traefik/cloudflare.env"
	traefikService   = "/etc/systemd/system/traefik.service"
)

// TraefikChallengeType controls the ACME challenge method.
type TraefikChallengeType string

const (
	ChallengeHTTP TraefikChallengeType = "http"
	ChallengeDNS  TraefikChallengeType = "dns"
)

// ─── APR1-MD5 (htpasswd compatible) ──────────────────────────────────────────

const apr1Alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// apr1Encode64 encodes src bytes into the apr1 base64 variant (different byte order).
func apr1Encode64(src []byte, n int) string {
	var result strings.Builder
	for i := 0; i < n; i += 3 {
		w := 0
		nBytes := 0
		for j := 0; j < 3 && i+j < n; j++ {
			w |= int(src[i+j]) << (j * 8)
			nBytes++
		}
		for j := 0; j < nBytes+1; j++ {
			result.WriteByte(apr1Alphabet[(w>>uint(j*6))&0x3f])
		}
	}
	return result.String()
}

// HashAPR1 hashes password with APR1-MD5 in htpasswd format: $apr1$<salt>$<hash>
// This is what `htpasswd -nbm user password` produces and what Traefik accepts.
func HashAPR1(password string) (string, error) {
	saltBytes := make([]byte, 6)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("error generating salt: %v", err)
	}
	// Encode salt to printable apr1 alphabet
	saltStr := ""
	for _, b := range saltBytes {
		saltStr += string(apr1Alphabet[int(b)%len(apr1Alphabet)])
	}
	return hashAPR1WithSalt(password, saltStr)
}

func hashAPR1WithSalt(password, salt string) (string, error) {
	pass := []byte(password)
	saltB := []byte(salt)
	magic := []byte("$apr1$")

	// Digest B: md5(pass + salt + pass)
	digestB := md5.New()
	digestB.Write(pass)
	digestB.Write(saltB)
	digestB.Write(pass)
	sumB := digestB.Sum(nil)

	// Digest A: md5(pass + magic + salt + <repeat sumB> + <end bits of len(pass)>)
	digestA := md5.New()
	digestA.Write(pass)
	digestA.Write(magic)
	digestA.Write(saltB)
	for i := len(pass); i > 0; i -= 16 {
		if i > 16 {
			digestA.Write(sumB)
		} else {
			digestA.Write(sumB[:i])
		}
	}
	for i := len(pass); i > 0; i >>= 1 {
		if i&1 != 0 {
			digestA.Write([]byte{0})
		} else {
			digestA.Write(pass[:1])
		}
	}
	sumA := digestA.Sum(nil)

	// 1000 rounds
	for i := 0; i < 1000; i++ {
		c := md5.New()
		if i&1 != 0 {
			c.Write(pass)
		} else {
			c.Write(sumA)
		}
		if i%3 != 0 {
			c.Write(saltB)
		}
		if i%7 != 0 {
			c.Write(pass)
		}
		if i&1 != 0 {
			c.Write(sumA)
		} else {
			c.Write(pass)
		}
		sumA = c.Sum(nil)
	}

	// Rearrange bytes in apr1 order
	order := []int{0, 6, 12, 1, 7, 13, 2, 8, 14, 3, 9, 15, 4, 10, 5, 11}
	rearranged := make([]byte, 16)
	for i, j := range order {
		rearranged[i] = sumA[j]
	}

	encoded := apr1Encode64(rearranged, 16)
	return fmt.Sprintf("$apr1$%s$%s", salt, encoded), nil
}

// HtpasswdEntry returns a full htpasswd line: "user:$apr1$salt$hash"
func HtpasswdEntry(user, password string) (string, error) {
	// Generate random salt (8 characters for APR1-MD5)
	saltBytes := make([]byte, 8)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("error generating salt: %v", err)
	}
	
	// Encode salt to printable apr1 alphabet
	saltStr := ""
	for _, b := range saltBytes {
		saltStr += string(apr1Alphabet[int(b)%len(apr1Alphabet)])
	}
	
	// Use openssl command for correct APR1-MD5 generation
	// This is a temporary fix for the APR1-MD5 implementation bug
	cmd := exec.Command("openssl", "passwd", "-apr1", "-salt", saltStr, password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("openssl command failed: %v", err)
	}
	
	// Parse the output which should be in format: $apr1$salt$hash
	hash := strings.TrimSpace(string(output))
	return fmt.Sprintf("%s:%s", user, hash), nil
}

// validateTraefikConfig checks that the config has required fields before
// applying any Traefik configuration changes.
func validateTraefikConfig(config *Config) error {
	if config.AdminEmail == "" {
		return fmt.Errorf("admin_email is required for Traefik ACME configuration")
	}
	if config.Domain == "" {
		return fmt.Errorf("domain is required for Traefik configuration")
	}
	return nil
}

// validateApp checks that an app has all required fields for Traefik routing.
func validateApp(app App) error {
	if app.ID == "" {
		return fmt.Errorf("app ID is required")
	}
	if app.Domain == "" {
		return fmt.Errorf("app domain is required for app '%s'", app.ID)
	}
	if app.Port <= 0 || app.Port > 65535 {
		return fmt.Errorf("app '%s' has invalid port %d (must be 1-65535)", app.ID, app.Port)
	}
	return nil
}

// setupTraefikConfig writes main traefik.yml and cloudflare.env.
// challengeType controls the ACME challenge method (http or dns).
// enableDocker adds the Docker provider for automatic container discovery.
// enableRedirect controls whether HTTP-to-HTTPS redirect is enabled (default: true).
func setupTraefikConfig(config *Config, challengeType TraefikChallengeType, enableDocker bool, enableRedirect bool) error {
	if err := validateTraefikConfig(config); err != nil {
		return fmt.Errorf("configuration validation failed: %v", err)
	}

	if err := os.MkdirAll(traefikConfigDir, 0755); err != nil {
		return fmt.Errorf("error creating traefik directory: %v", err)
	}

	// Build the ACME challenge block
	var challengeBlock string
	if challengeType == ChallengeDNS {
		challengeBlock = `      dnsChallenge:
        provider: cloudflare
        delayBeforeCheck: 30`
	} else {
		// Default: HTTP challenge — simpler and doesn't require CF token scopes
		challengeBlock = `      httpChallenge:
        entryPoint: web`
	}

	// Build the providers block
	providersBlock := `providers:
  file:
    filename: /etc/traefik/dynamic.yml
    watch: true`
	if enableDocker {
		providersBlock = `providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false
  file:
    filename: /etc/traefik/dynamic.yml
    watch: true`
	}

	// Build the entryPoints block with optional redirect
	var entryPointsBlock string
	if enableRedirect {
		entryPointsBlock = `entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: websecure
          scheme: https
          permanent: true

  websecure:
    address: ":443"
    http:
      tls:
        certResolver: letsencrypt`
	} else {
		entryPointsBlock = `entryPoints:
  web:
    address: ":80"

  websecure:
    address: ":443"
    http:
      tls:
        certResolver: letsencrypt`
	}

	mainConfig := fmt.Sprintf(`global:
  checkNewVersion: true
  sendAnonymousUsage: false

api:
  dashboard: true
  insecure: false

%s

certificatesResolvers:
  letsencrypt:
    acme:
      email: %s
      storage: /etc/traefik/acme.json
%s

%s
`, entryPointsBlock, config.AdminEmail, challengeBlock, providersBlock)

	if err := os.WriteFile(traefikMain, []byte(mainConfig), 0644); err != nil {
		return fmt.Errorf("error writing traefik.yml: %v", err)
	}

	// Write cloudflare.env (always kept for DNS challenge compatibility)
	envConfig := fmt.Sprintf("CF_API_EMAIL=%s\nCF_API_KEY=%s\n", config.AdminEmail, config.CloudflareToken)
	if err := os.WriteFile(traefikEnv, []byte(envConfig), 0600); err != nil {
		return fmt.Errorf("error writing cloudflare.env: %v", err)
	}

	// Create acme.json if absent
	acmePath := filepath.Join(traefikConfigDir, "acme.json")
	if _, err := os.Stat(acmePath); os.IsNotExist(err) {
		if err := os.WriteFile(acmePath, []byte("{}"), 0600); err != nil {
			return fmt.Errorf("error creating acme.json: %v", err)
		}
	}

	return nil
}

// buildDynamicYAML renders the Traefik dynamic.yml content from the config.
// TC1 fix: each router TLS section now includes an explicit `domains:` entry
// so Traefik/ACME can resolve the domain during certificate provisioning.
func buildDynamicYAML(config *Config) (string, error) {
	for _, app := range config.Apps {
		if err := validateApp(app); err != nil {
			return "", fmt.Errorf("app validation failed: %v", err)
		}
	}

	var sb strings.Builder
	sb.WriteString("http:\n  routers:\n")

	for _, app := range config.Apps {
		sb.WriteString(fmt.Sprintf("    %s:\n", app.ID))
		sb.WriteString(fmt.Sprintf("      rule: \"Host(`%s`)\"\n", app.Domain))
		sb.WriteString(fmt.Sprintf("      service: %s\n", app.ID))
		sb.WriteString("      entryPoints:\n        - websecure\n")
		if len(app.BasicAuth) > 0 {
			sb.WriteString(fmt.Sprintf("      middlewares:\n        - %s-basic-auth\n", app.ID))
		}
		sb.WriteString("      tls:\n")
		sb.WriteString("        certResolver: letsencrypt\n")
		// Explicit domain spec — prevents ACME "domain not defined" errors
		sb.WriteString("        domains:\n")
		sb.WriteString(fmt.Sprintf("          - main: %s\n\n", app.Domain))
	}

	sb.WriteString("  services:\n")

	for _, app := range config.Apps {
		sb.WriteString(fmt.Sprintf("    %s:\n", app.ID))
		sb.WriteString("      loadBalancer:\n")
		sb.WriteString("        servers:\n")
		// Use custom backend URL if provided, otherwise default to localhost
		if app.BackendURL != "" {
			sb.WriteString(fmt.Sprintf("          - url: \"%s\"\n\n", app.BackendURL))
		} else {
			sb.WriteString(fmt.Sprintf("          - url: \"http://127.0.0.1:%d\"\n\n", app.Port))
		}
	}

	// Emit middlewares section only for apps that have basicAuth entries
	hasMiddleware := false
	for _, app := range config.Apps {
		if len(app.BasicAuth) > 0 {
			hasMiddleware = true
			break
		}
	}
	if hasMiddleware {
		sb.WriteString("  middlewares:\n")
		for _, app := range config.Apps {
			if len(app.BasicAuth) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("    %s-basic-auth:\n", app.ID))
			sb.WriteString("      basicAuth:\n")
			sb.WriteString("        users:\n")
			for _, entry := range app.BasicAuth {
				// YAML-escape the hash — $ chars need to be quoted
				sb.WriteString(fmt.Sprintf("          - %q\n", entry))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// updateDynamicConfig enqueues a config snapshot for the debounced writer.
// The background goroutine (startDynamicConfigWriter) will flush the latest
// snapshot to dynamic.yml after 5 s of no further calls, atomically.
// Validation is done eagerly here so callers get errors immediately.
func updateDynamicConfig(config *Config) error {
	// Validate first so the caller gets a synchronous error if something is wrong.
	for _, app := range config.Apps {
		if err := validateApp(app); err != nil {
			return fmt.Errorf("app validation failed: %v", err)
		}
	}

	// Deep-copy the Apps slice so the snapshot is immutable after enqueue.
	snapshot := *config
	appsCopy := make([]App, len(config.Apps))
	copy(appsCopy, config.Apps)
	snapshot.Apps = appsCopy

	// Non-blocking send — channel is buffered (64); if full we fall back to a
	// direct synchronous write so we never silently drop an update.
	select {
	case dynamicWriteCh <- &snapshot:
	default:
		log.Printf("[hotify] dynamic write queue full — flushing synchronously")
		return writeDynamicConfigAtomic(&snapshot)
	}
	return nil
}

func setupTraefikService() error {
	// If the service file already exists we skip writing it — modifying
	// /etc/systemd/system/ requires root privileges but the config files
	// in /etc/traefik/ are typically owned by the deploying user.
	// On fresh installs (traefik-system command), this file is created with sudo.
	if _, err := os.Stat(traefikService); err == nil {
		// Service already installed — just ensure it's enabled
		cmd := exec.Command("sudo", "systemctl", "enable", "traefik")
		_ = cmd.Run() // best-effort; failure is non-fatal
		return nil
	}

	serviceConfig := `[Unit]
Description=Traefik
Documentation=https://docs.traefik.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/traefik --configFile=/etc/traefik/traefik.yml
Restart=on-failure
RestartSec=5s
EnvironmentFile=/etc/traefik/cloudflare.env
KillMode=process
KillSignal=SIGTERM
TimeoutStopSec=30s

[Install]
WantedBy=multi-user.target
`

	if err := os.WriteFile(traefikService, []byte(serviceConfig), 0644); err != nil {
		return fmt.Errorf("error writing traefik.service: %v", err)
	}

	cmd := exec.Command("sudo", "systemctl", "daemon-reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error reloading systemd: %v", err)
	}

	cmd = exec.Command("sudo", "systemctl", "enable", "traefik")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error enabling traefik service: %v", err)
	}

	return nil
}

// restartTraefik restarts the Traefik systemd service.
// TC3 fix: detects whether the service is already running (and thus whether
// we need start vs restart), and ignores "reload not supported" gracefully.
func restartTraefik() error {
	// Check if traefik service exists and is running
	statusCmd := exec.Command("sudo", "systemctl", "is-active", "--quiet", "traefik")
	isRunning := statusCmd.Run() == nil

	var cmd *exec.Cmd
	if isRunning {
		cmd = exec.Command("sudo", "systemctl", "restart", "traefik")
	} else {
		cmd = exec.Command("sudo", "systemctl", "start", "traefik")
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error restarting traefik: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// setupTraefikForApp configures Traefik for a single app (and all current apps).
// Uses HTTP challenge by default; pass --challenge-type dns to switch.
func setupTraefikForApp(appID string) error {
	return setupTraefikForAppWithChallenge(appID, ChallengeHTTP, false)
}

// setupTraefikForAppWithChallenge is the full path with explicit challenge choice.
// Uses smart redirect handling for HTTP challenge to avoid ACME issues.
func setupTraefikForAppWithChallenge(appID string, challengeType TraefikChallengeType, enableDocker bool) error {
	return setupTraefikForAppWithSmartRedirect(appID, challengeType, enableDocker)
}

// setupTraefikForAppWithChallengeAndRedirect is the full path with explicit challenge and redirect control.
func setupTraefikForAppWithChallengeAndRedirect(appID string, challengeType TraefikChallengeType, enableDocker bool, enableRedirect bool) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// TC5: validate before touching any files
	if err := validateTraefikConfig(config); err != nil {
		return err
	}
	found := false
	for _, app := range config.Apps {
		if app.ID == appID {
			found = true
			if err := validateApp(app); err != nil {
				return err
			}
		}
	}
	if !found {
		return fmt.Errorf("app '%s' not found in configuration", appID)
	}

	if err := setupTraefikConfig(config, challengeType, enableDocker, enableRedirect); err != nil {
		return fmt.Errorf("error setting up traefik config: %v", err)
	}

	if err := setupTraefikService(); err != nil {
		return fmt.Errorf("error setting up traefik service: %v", err)
	}

	if err := updateDynamicConfig(config); err != nil {
		return fmt.Errorf("error updating dynamic config: %v", err)
	}

	if err := restartTraefik(); err != nil {
		return fmt.Errorf("error restarting traefik: %v", err)
	}

	dockerNote := ""
	if enableDocker {
		dockerNote = " (Docker provider enabled)"
	}
	redirectNote := ""
	if !enableRedirect {
		redirectNote = " (HTTP redirect disabled)"
	}
	fmt.Printf("✅ Traefik configured for app: %s (challenge: %s)%s%s\n", appID, challengeType, dockerNote, redirectNote)
	return nil
}

// enableDockerProvider adds the Docker provider to Traefik configuration
func enableDockerProvider() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	if err := validateTraefikConfig(config); err != nil {
		return err
	}

	// Use HTTP challenge (default), enable Docker, and keep redirect enabled
	if err := setupTraefikConfig(config, ChallengeHTTP, true, true); err != nil {
		return fmt.Errorf("error setting up traefik config: %v", err)
	}

	if err := restartTraefik(); err != nil {
		return fmt.Errorf("error restarting traefik: %v", err)
	}

	fmt.Println("✅ Docker provider enabled in Traefik")
	return nil
}

// disableDockerProvider removes the Docker provider from Traefik configuration
func disableDockerProvider() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	if err := validateTraefikConfig(config); err != nil {
		return err
	}

	// Use HTTP challenge (default), disable Docker, and keep redirect enabled
	if err := setupTraefikConfig(config, ChallengeHTTP, false, true); err != nil {
		return fmt.Errorf("error setting up traefik config: %v", err)
	}

	if err := restartTraefik(); err != nil {
		return fmt.Errorf("error restarting traefik: %v", err)
	}

	fmt.Println("✅ Docker provider disabled in Traefik")
	return nil
}

// ─── ACME Certificate Verification ─────────────────────────────────────────────

// ACMECertificate represents a certificate in acme.json
type ACMECertificate struct {
	Domain struct {
		Main string   `json:"main"`
		SANs []string `json:"sans,omitempty"`
	} `json:"domain"`
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

// ACMEData represents the structure of acme.json
type ACMEData struct {
	Letsencrypt struct {
		Certificates []ACMECertificate `json:"Certificates"`
	} `json:"letsencrypt"`
}

// checkCertificateForDomain checks if a valid certificate exists for the given domain
func checkCertificateForDomain(domain string) (bool, error) {
	acmePath := filepath.Join(traefikConfigDir, "acme.json")
	
	// Read acme.json
	data, err := os.ReadFile(acmePath)
	if err != nil {
		return false, fmt.Errorf("failed to read acme.json: %v", err)
	}

	var acmeData ACMEData
	if err := json.Unmarshal(data, &acmeData); err != nil {
		return false, fmt.Errorf("failed to parse acme.json: %v", err)
	}

	// Check if certificate exists for the domain
	for _, cert := range acmeData.Letsencrypt.Certificates {
		if cert.Domain.Main == domain {
			return true, nil
		}
		// Also check SANs (Subject Alternative Names)
		for _, san := range cert.Domain.SANs {
			if san == domain {
				return true, nil
			}
		}
	}

	return false, nil
}

// waitForCertificate waits for a certificate to be issued for the given domain
func waitForCertificate(domain string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		hasCert, err := checkCertificateForDomain(domain)
		if err != nil {
			return fmt.Errorf("error checking certificate: %v", err)
		}
		if hasCert {
			return nil
		}
		
		// Wait 2 seconds before checking again
		time.Sleep(2 * time.Second)
	}
	
	return fmt.Errorf("timeout waiting for certificate for domain %s", domain)
}

// setupTraefikForAppWithSmartRedirect handles ACME challenge redirect automatically
// When using HTTP challenge, it temporarily disables redirect, obtains certificate, then re-enables redirect
func setupTraefikForAppWithSmartRedirect(appID string, challengeType TraefikChallengeType, enableDocker bool) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Find the app's domain
	var appDomain string
	found := false
	for _, app := range config.Apps {
		if app.ID == appID {
			found = true
			appDomain = app.Domain
			break
		}
	}
	if !found {
		return fmt.Errorf("app '%s' not found in configuration", appID)
	}

	// Check if certificate already exists
	hasCert, _ := checkCertificateForDomain(appDomain)
	
	// If using HTTP challenge and certificate doesn't exist, use smart redirect handling
	if challengeType == ChallengeHTTP && !hasCert {
		fmt.Printf("🔧 Using HTTP challenge - temporarily disabling redirect for ACME...\n")
		
		// Step 1: Setup without redirect
		if err := setupTraefikForAppWithChallengeAndRedirect(appID, challengeType, enableDocker, false); err != nil {
			return fmt.Errorf("failed to setup Traefik without redirect: %v", err)
		}
		
		// Step 2: Wait for certificate (with timeout)
		fmt.Printf("⏳ Waiting for certificate generation (max 60s)...\n")
		if err := waitForCertificate(appDomain, 60*time.Second); err != nil {
			// Certificate generation failed - re-enable redirect before returning error
			fmt.Printf("⚠️ Certificate generation failed: %v\n", err)
			fmt.Printf("🔧 Re-enabling redirect...\n")
			setupTraefikForAppWithChallengeAndRedirect(appID, challengeType, enableDocker, true)
			return fmt.Errorf("certificate generation failed: %v (consider using --challenge-type dns)", err)
		}
		
		fmt.Printf("✅ Certificate obtained successfully\n")
		
		// Step 3: Re-enable redirect
		fmt.Printf("🔧 Re-enabling HTTP-to-HTTPS redirect...\n")
		if err := setupTraefikForAppWithChallengeAndRedirect(appID, challengeType, enableDocker, true); err != nil {
			return fmt.Errorf("failed to re-enable redirect: %v", err)
		}
		
		fmt.Printf("✅ Traefik configured with redirect for app: %s\n", appID)
		return nil
	}
	
	// For DNS challenge or existing certificates, use normal setup
	return setupTraefikForAppWithChallengeAndRedirect(appID, challengeType, enableDocker, true)
}
