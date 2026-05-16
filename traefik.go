package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	traefikConfigDir  = "/etc/traefik"
	traefikDynamic    = "/etc/traefik/dynamic.yml"
	traefikMain       = "/etc/traefik/traefik.yml"
	traefikEnv        = "/etc/traefik/cloudflare.env"
	traefikService    = "/etc/systemd/system/traefik.service"
)

func setupTraefikConfig(config *Config) error {
	// Create traefik directory if it doesn't exist
	if err := os.MkdirAll(traefikConfigDir, 0755); err != nil {
		return fmt.Errorf("error creating traefik directory: %v", err)
	}

	// Write main traefik.yml
	mainConfig := fmt.Sprintf(`global:
  checkNewVersion: true
  sendAnonymousUsage: false

api:
  dashboard: true
  insecure: false

entryPoints:
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
        certResolver: letsencrypt

certificatesResolvers:
  letsencrypt:
    acme:
      email: %s
      storage: /etc/traefik/acme.json
      dnsChallenge:
        provider: cloudflare
        delayBeforeCheck: 30

providers:
  file:
    filename: /etc/traefik/dynamic.yml
    watch: true
`, config.AdminEmail)

	if err := os.WriteFile(traefikMain, []byte(mainConfig), 0644); err != nil {
		return fmt.Errorf("error writing traefik.yml: %v", err)
	}

	// Write cloudflare.env
	envConfig := fmt.Sprintf("CF_API_EMAIL=%s\nCF_API_KEY=%s\n", config.AdminEmail, config.CloudflareToken)
	if err := os.WriteFile(traefikEnv, []byte(envConfig), 0600); err != nil {
		return fmt.Errorf("error writing cloudflare.env: %v", err)
	}

	// Create acme.json
	acmePath := filepath.Join(traefikConfigDir, "acme.json")
	if _, err := os.Stat(acmePath); os.IsNotExist(err) {
		if err := os.WriteFile(acmePath, []byte("{}"), 0600); err != nil {
			return fmt.Errorf("error creating acme.json: %v", err)
		}
	}

	return nil
}

func updateDynamicConfig(config *Config) error {
	dynamicConfig := "http:\n  routers:\n"

	for _, app := range config.Apps {
		dynamicConfig += fmt.Sprintf("    %s:\n", app.ID)
		dynamicConfig += fmt.Sprintf("      rule: \"Host(`%s`)\"\n", app.Domain)
		dynamicConfig += fmt.Sprintf("      service: %s\n", app.ID)
		dynamicConfig += "      entryPoints:\n        - websecure\n"
		dynamicConfig += "      tls:\n        certResolver: letsencrypt\n\n"
	}

	dynamicConfig += "  services:\n"

	for _, app := range config.Apps {
		dynamicConfig += fmt.Sprintf("    %s:\n", app.ID)
		dynamicConfig += "      loadBalancer:\n"
		dynamicConfig += "        servers:\n"
		dynamicConfig += fmt.Sprintf("          - url: \"http://localhost:%d\"\n\n", app.Port)
	}

	if err := os.WriteFile(traefikDynamic, []byte(dynamicConfig), 0644); err != nil {
		return fmt.Errorf("error writing dynamic.yml: %v", err)
	}

	return nil
}

func setupTraefikService() error {
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

	// Reload systemd and enable service
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

func restartTraefik() error {
	cmd := exec.Command("sudo", "systemctl", "restart", "traefik")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error restarting traefik: %v", err)
	}
	return nil
}

func setupTraefikForApp(appID string) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	// Setup main traefik config
	if err := setupTraefikConfig(config); err != nil {
		return fmt.Errorf("error setting up traefik config: %v", err)
	}

	// Setup systemd service
	if err := setupTraefikService(); err != nil {
		return fmt.Errorf("error setting up traefik service: %v", err)
	}

	// Update dynamic config
	if err := updateDynamicConfig(config); err != nil {
		return fmt.Errorf("error updating dynamic config: %v", err)
	}

	// Restart traefik
	if err := restartTraefik(); err != nil {
		return fmt.Errorf("error restarting traefik: %v", err)
	}

	fmt.Printf("✅ Traefik configured for app: %s\n", appID)
	return nil
}
