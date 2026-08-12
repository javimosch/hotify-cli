package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// TraefikDynamicConfig represents the structure of /etc/traefik/dynamic.yml
type TraefikDynamicConfig struct {
	HTTP struct {
		Routers     map[string]TraefikRouter     `yaml:"routers"`
		Services    map[string]TraefikService    `yaml:"services"`
		Middlewares map[string]TraefikMiddleware `yaml:"middlewares"`
	} `yaml:"http"`
}

type TraefikRouter struct {
	Rule        string   `yaml:"rule"`
	Service     string   `yaml:"service"`
	EntryPoints []string `yaml:"entryPoints"`
	Middlewares []string `yaml:"middlewares,omitempty"`
	TLS         struct {
		CertResolver string `yaml:"certResolver"`
		Domains      []struct {
			Main string `yaml:"main"`
		} `yaml:"domains"`
	} `yaml:"tls"`
}

type TraefikService struct {
	LoadBalancer struct {
		Servers []struct {
			URL string `yaml:"url"`
		} `yaml:"servers"`
	} `yaml:"loadBalancer"`
}

type TraefikMiddleware struct {
	BasicAuth struct {
		Users []string `yaml:"users"`
	} `yaml:"basicAuth"`
	AddPrefix struct {
		Prefix string `yaml:"prefix"`
	} `yaml:"addPrefix"`
}

// importTraefikConfig imports existing Traefik configuration into hotify
func importTraefikConfig() error {
	dynamicPath := traefikDynamic
	
	// Check if Traefik config exists
	if _, err := os.Stat(dynamicPath); os.IsNotExist(err) {
		return fmt.Errorf("Traefik dynamic config not found at %s", dynamicPath)
	}

	// Read Traefik config
	data, err := os.ReadFile(dynamicPath)
	if err != nil {
		return fmt.Errorf("error reading Traefik config: %v", err)
	}

	// Parse YAML
	var traefikConfig TraefikDynamicConfig
	if err := yaml.Unmarshal(data, &traefikConfig); err != nil {
		return fmt.Errorf("error parsing Traefik YAML: %v", err)
	}

	// Load existing hotify config
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("error loading hotify config: %v", err)
	}

	// Convert Traefik config to hotify apps
	importedApps := []App{}
	for routerID, router := range traefikConfig.HTTP.Routers {
		// Extract domain from Host rule: "Host(`domain.com`)"
		domain := extractDomainFromRule(router.Rule)
		if domain == "" {
			fmt.Printf("⚠️  Skipping router %s: could not extract domain\n", routerID)
			continue
		}

		// Get service to determine port/backend URL
		service, ok := traefikConfig.HTTP.Services[router.Service]
		if !ok {
			fmt.Printf("⚠️  Skipping router %s: service %s not found\n", routerID, router.Service)
			continue
		}

		// Extract port or backend URL from service
		port, backendURL := extractPortOrBackendURL(service)
		
		// Extract basic auth and path-prefix middlewares if configured
		var basicAuth []string
		var pathPrefix string
		for _, middlewareID := range router.Middlewares {
			if middleware, ok := traefikConfig.HTTP.Middlewares[middlewareID]; ok {
				if len(middleware.BasicAuth.Users) > 0 {
					basicAuth = append(basicAuth, middleware.BasicAuth.Users...)
				}
				if middleware.AddPrefix.Prefix != "" {
					pathPrefix = middleware.AddPrefix.Prefix
				}
			}
		}

		// Create hotify app
		app := App{
			ID:         routerID,
			Name:       routerID, // Use router ID as name (can be updated later)
			Domain:     domain,
			Port:       port,
			Command:    fmt.Sprintf("# TODO: add command for %s", routerID),
			Source:     "imported-from-traefik",
			Status:     "unknown",
			BasicAuth:  basicAuth,
			BackendURL: backendURL,
			PathPrefix: pathPrefix,
		}

		importedApps = append(importedApps, app)
	}

	if len(importedApps) == 0 {
		return fmt.Errorf("no apps found in Traefik configuration")
	}

	// Merge with existing apps (avoid duplicates by ID)
	existingAppIDs := make(map[string]bool)
	for _, app := range config.Apps {
		existingAppIDs[app.ID] = true
	}

	newAppsCount := 0
	for _, importedApp := range importedApps {
		if !existingAppIDs[importedApp.ID] {
			config.Apps = append(config.Apps, importedApp)
			newAppsCount++
		}
	}

	// Save updated config
	if err := saveConfig(config); err != nil {
		return fmt.Errorf("error saving hotify config: %v", err)
	}

	fmt.Printf("✅ Imported %d apps from Traefik configuration\n", len(importedApps))
	fmt.Printf("   - %d new apps added\n", newAppsCount)
	fmt.Printf("   - %d apps already existed (skipped)\n", len(importedApps)-newAppsCount)
	
	return nil
}

// extractDomainFromRule extracts domain from Host rule
// Example: "Host(`powersentry.pve.intrane.fr`)" -> "powersentry.pve.intrane.fr"
func extractDomainFromRule(rule string) string {
	// Remove Host( and backticks
	rule = strings.TrimSpace(rule)
	rule = strings.TrimPrefix(rule, "Host(")
	rule = strings.TrimSuffix(rule, ")")
	rule = strings.Trim(rule, "`")
	return rule
}

// extractPortOrBackendURL extracts port or backend URL from service config.
// Localhost URLs (127.0.0.1 or localhost, http or https) are stored as a port
// so the app can be managed like a local service; anything else is kept as a
// backend_url for proxy/Tailscale use cases.
func extractPortOrBackendURL(service TraefikService) (int, string) {
	if len(service.LoadBalancer.Servers) == 0 {
		return 0, ""
	}

	url := service.LoadBalancer.Servers[0].URL

	localPrefixes := []string{
		"http://127.0.0.1:",
		"http://localhost:",
		"https://127.0.0.1:",
		"https://localhost:",
	}
	for _, prefix := range localPrefixes {
		if strings.HasPrefix(url, prefix) {
			portStr := strings.TrimPrefix(url, prefix)
			var port int
			fmt.Sscanf(portStr, "%d", &port)
			return port, ""
		}
	}

	// External backend URL
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return 0, url
	}

	return 0, ""
}

// initImportTraefik is the command handler for import-traefik
func initImportTraefik() {
	format := getOutputFormat()

	if err := importTraefikConfig(); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "import_error",
				Message:     fmt.Sprintf("Error importing Traefik config: %v", err),
				Recoverable: true,
				Suggestions: []string{
					"Ensure Traefik is installed and configured",
					"Check that /etc/traefik/dynamic.yml exists and is valid YAML",
					"Run with sudo if needed to read Traefik config files",
				},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"message": "Traefik configuration imported successfully",
			"note":    "Review imported apps and update their 'command' field as needed",
		},
	}
	printOutput(result, format)
}