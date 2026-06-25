package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ─── Source-of-truth command catalog ───────────────────────────────────────
//
// 'hotify-cli guide' emits this catalog as JSON (default, agent-friendly)
// or as prose (--text).  It lives in the binary so it is always version-exact.
// Agents: run 'hotify-cli guide' once to learn every command, its flags, and
// the canonical ordering for common workflows.

type guideFlag struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Req         string `json:"required,omitempty"` // "yes" | "no"
	Description string `json:"description"`
}

type guideCommand struct {
	Name        string      `json:"name"`
	Summary     string      `json:"summary"`
	Category    string      `json:"category"`
	Flags       []guideFlag `json:"flags"`
	Before      []string    `json:"before,omitempty"` // commands commonly run before this one
	After       []string    `json:"after,omitempty"`  // commands commonly run after this one
	Note        string      `json:"note,omitempty"`
}

type guideNote struct {
	Topic   string `json:"topic"`
	Detail  string `json:"detail"`
}

type guideWorkflow struct {
	Name   string   `json:"name"`
	Steps  []string `json:"steps"`
	Detail string   `json:"detail,omitempty"`
}

type guideCatalog struct {
	Version   string          `json:"version"`
	Schema    string          `json:"schema"`
	Tagline   string          `json:"tagline"`
	Commands  []guideCommand  `json:"commands"`
	Workflows []guideWorkflow `json:"workflows"`
	Tips      []guideNote     `json:"tips"`
	Gotchas   []guideNote     `json:"gotchas"`
}

func buildGuide() guideCatalog {
	return guideCatalog{
		Version: Version,
		Schema:  "hotify.guide/v1",
		Tagline: "Traefik/Cloudflare app management CLI — DNS, SSL, reverse proxy for web apps. Default output is JSON (add --human for text).",
		Commands: []guideCommand{
			// ── Configuration ──────────────────────────────────────────
			{
				Name:     "init",
				Summary:  "Initialize config (non-interactive in JSON mode, interactive with --human)",
				Category: "configuration",
				Flags: []guideFlag{
					{Name: "--token", Type: "string", Default: "", Req: "yes-in-json", Description: "Cloudflare API token"},
					{Name: "--domain", Type: "string", Default: "", Req: "yes-in-json", Description: "Base domain (e.g. intrane.fr)"},
					{Name: "--email", Type: "string", Default: "", Req: "yes-in-json", Description: "Admin email for Let's Encrypt"},
				},
				After: []string{"setup"},
				Note:  "Run first on a fresh install. Creates ~/.hotify/config.json.",
			},
			{
				Name:     "setup",
				Summary:  "Create or update an app (upsert). The primary command for registering apps.",
				Category: "configuration",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "Unique app identifier"},
					{Name: "--name", Type: "string", Default: "", Req: "no", Description: "Human-readable name"},
					{Name: "--domain", Type: "string", Default: "", Req: "no", Description: "Subdomain (auto-prefixed with base domain)"},
					{Name: "--port", Type: "int", Default: "0", Req: "no", Description: "App port (1-65535)"},
					{Name: "--command", Type: "string", Default: "", Req: "no", Description: "Start command (e.g. '/usr/bin/app start')"},
					{Name: "--source", Type: "string", Default: "", Req: "no", Description: "GitHub repo or local path"},
					{Name: "--backend-url", Type: "string", Default: "", Req: "no", Description: "Custom backend URL for remote apps (e.g. http://100.123.0.125:7000)"},
					{Name: "--path-prefix", Type: "string", Default: "", Req: "no", Description: "Traefik addPrefix middleware path (e.g. /slv2)"},
					{Name: "--compose-file", Type: "string", Default: "", Req: "no", Description: "Docker Compose file name"},
					{Name: "--compose-path", Type: "string", Default: "", Req: "no", Description: "Remote path for compose project"},
					{Name: "--setup-dns", Type: "bool", Default: "false", Req: "no", Description: "Auto-setup Cloudflare DNS A record"},
				},
				Before: []string{"init"},
				After:  []string{"setup-dns", "setup-traefik"},
				Note:   "Upsert semantics: re-run with the same --id to update fields. Always pair with setup-dns + setup-traefik.",
			},
			{
				Name:     "add",
				Summary:  "Add a new app (strict create — fails if ID already exists)",
				Category: "configuration",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "Unique app identifier"},
					{Name: "--name", Type: "string", Default: "", Req: "yes", Description: "Human-readable name"},
					{Name: "--domain", Type: "string", Default: "", Req: "yes", Description: "Subdomain"},
					{Name: "--port", Type: "int", Default: "0", Req: "yes", Description: "App port"},
					{Name: "--command", Type: "string", Default: "", Req: "yes", Description: "Start command"},
				},
				After: []string{"setup-dns", "setup-traefik"},
				Note:  "Legacy. Prefer 'setup' (upsert) for agent workflows.",
			},
			{
				Name:     "remove",
				Summary:  "Remove app from config. Warns about DNS/Traefik cleanup.",
				Category: "configuration",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID to remove"},
				},
				Before: []string{"prune"},
				Note:   "Does NOT clean DNS or Traefik. Run 'prune --id <app>' after removal.",
			},
			{
				Name:     "list",
				Summary:  "List all configured apps",
				Category: "configuration",
			},

			// ── Process Management ─────────────────────────────────────
			{
				Name:     "start",
				Summary:  "Start an app or the hotify daemon. Use --daemon for daemon mode, --id for app, --local for same-machine.",
				Category: "process",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "no", Description: "App ID to start"},
					{Name: "--daemon", Type: "bool", Default: "false", Req: "no", Description: "Start hotify daemon (port 8080)"},
					{Name: "--local", Type: "bool", Default: "false", Req: "no", Description: "Execute directly on local server"},
				},
				After: []string{"deploy"},
			},
			{
				Name:     "stop",
				Summary:  "Stop an app or the hotify daemon",
				Category: "process",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "no", Description: "App ID to stop"},
					{Name: "--local", Type: "bool", Default: "false", Req: "no", Description: "Execute directly on local server"},
				},
			},
			{
				Name:     "restart",
				Summary:  "Restart an app",
				Category: "process",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID to restart"},
					{Name: "--local", Type: "bool", Default: "false", Req: "no", Description: "Execute directly on local server"},
				},
			},
			{
				Name:     "status",
				Summary:  "App or daemon status",
				Category: "process",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "no", Description: "App ID (omit for daemon status)"},
					{Name: "--local", Type: "bool", Default: "false", Req: "no", Description: "Execute directly on local server"},
				},
			},
			{
				Name:     "pause",
				Summary:  "Pause an app (SIGSTOP — freeze in memory)",
				Category: "process",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID to pause"},
				},
				After: []string{"resume"},
			},
			{
				Name:     "resume",
				Summary:  "Resume a paused app (SIGCONT)",
				Category: "process",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID to resume"},
				},
				Before: []string{"pause"},
			},

			// ── Deployment ─────────────────────────────────────────────
			{
				Name:     "deploy",
				Summary:  "Upload binary/folder to remote target via HTTP API",
				Category: "deployment",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID (must exist in config)"},
					{Name: "--source", Type: "string", Default: "", Req: "yes", Description: "Local path to binary or folder"},
					{Name: "--setup-dns", Type: "bool", Default: "false", Req: "no", Description: "Also setup DNS A record"},
				},
				Before: []string{"setup"},
				After:  []string{"start", "setup-dns", "setup-traefik"},
			},

			// ── DNS & Traefik ──────────────────────────────────────────
			{
				Name:     "setup-dns",
				Summary:  "Create/update Cloudflare DNS A record for an app. IP auto-detected if omitted.",
				Category: "dns-traefik",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--ip", Type: "string", Default: "", Req: "no", Description: "Server public IP (auto-detected via ifconfig.me)"},
					{Name: "--local", Type: "bool", Default: "false", Req: "no", Description: "Execute directly on local server"},
				},
				Before: []string{"setup-traefik"},
				After:  []string{"setup"},
			},
			{
				Name:     "setup-traefik",
				Summary:  "Configure Traefik routing for an app. Regenerates ALL apps' dynamic config (see note).",
				Category: "dns-traefik",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--challenge-type", Type: "string", Default: "http", Req: "no", Description: "ACME challenge: http or dns"},
					{Name: "--no-redirect", Type: "bool", Default: "false", Req: "no", Description: "Disable HTTP-to-HTTPS redirect"},
					{Name: "--dry-run", Type: "bool", Default: "false", Req: "no", Description: "Preview changes without applying"},
					{Name: "--rate-limit", Type: "string", Default: "", Req: "no", Description: "Rate limit: 'count,period' e.g. '10,60m'"},
					{Name: "--local", Type: "bool", Default: "false", Req: "no", Description: "Execute directly on local server"},
				},
				Before: []string{"setup-dns"},
				After:  []string{"setup"},
				Note:   "⚠ Regenerates ALL apps. Apps without backend_url in config.json may lose remote backends. To prevent: set backend_url via setup --backend-url.",
			},
			{
				Name:     "basic-auth",
				Summary:  "Manage Traefik HTTP basic auth credentials for an app. Run setup-traefik after changes.",
				Category: "dns-traefik",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--action", Type: "string", Default: "", Req: "yes", Description: "add, remove, or list"},
					{Name: "--user", Type: "string", Default: "", Req: "for-add-remove", Description: "Username"},
					{Name: "--password", Type: "string", Default: "", Req: "for-add", Description: "Plaintext password (hashed client-side)"},
					{Name: "--hash", Type: "string", Default: "", Req: "no", Description: "Pre-hashed htpasswd entry (user:$apr1$...)"},
					{Name: "--local", Type: "bool", Default: "false", Req: "no", Description: "Execute directly on local server"},
				},
				After: []string{"setup-traefik"},
				Note:  "After add/remove, run setup-traefik --id <app> to apply.",
			},
			{
				Name:     "import-traefik",
				Summary:  "Import existing Traefik configuration into hotify",
				Category: "dns-traefik",
				Note:     "One-shot migration tool. Reads current /etc/traefik/dynamic.yml and registers apps in config.json. After import, apps may need backend_url set manually for remote backends.",
			},

			// ── Docker ─────────────────────────────────────────────────
			{
				Name:     "docker",
				Summary:  "Docker container management (list, start, stop, restart, status, logs)",
				Category: "docker",
				Flags: []guideFlag{
					{Name: "--target", Type: "string", Default: "", Req: "no", Description: "Target name for remote Docker"},
				},
			},
			{
				Name:     "compose",
				Summary:  "Docker Compose passthrough. Resolves compose_path and compose_file from app config when --id is set.",
				Category: "docker",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "no", Description: "App ID (resolves paths)"},
				},
				Before: []string{"setup"},
			},
			{
				Name:     "deploy-compose",
				Summary:  "Copy full Docker Compose project tree to remote compose_path",
				Category: "docker",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--source", Type: "string", Default: "", Req: "yes", Description: "Local project directory"},
					{Name: "--compose-file", Type: "string", Default: "", Req: "no", Description: "Compose file name (default: compose.yml)"},
					{Name: "--remote-path", Type: "string", Default: "", Req: "no", Description: "Override remote compose_path"},
					{Name: "--start", Type: "bool", Default: "false", Req: "no", Description: "Start after deploy"},
				},
				Before: []string{"setup"},
			},
			{
				Name:     "compose-sync",
				Summary:  "Sync compose file (+ .env) to remote compose_path",
				Category: "docker",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--source", Type: "string", Default: "", Req: "no", Description: "Source directory"},
					{Name: "--restart", Type: "bool", Default: "false", Req: "no", Description: "Restart after sync"},
					{Name: "--env", Type: "bool", Default: "true", Req: "no", Description: "Sync .env file"},
				},
			},
			{
				Name:     "compose-copy-dir",
				Summary:  "Copy a local directory into remote compose_path",
				Category: "docker",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--dir", Type: "string", Default: "", Req: "yes", Description: "Subdirectory name"},
					{Name: "--source", Type: "string", Default: "", Req: "yes", Description: "Local directory path"},
				},
			},
			{
				Name:     "volume-init",
				Summary:  "Populate a Docker named volume with local directory contents",
				Category: "docker",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--volume", Type: "string", Default: "", Req: "yes", Description: "Volume name (resolved as <app_id>_<volume>)"},
					{Name: "--source", Type: "string", Default: "", Req: "yes", Description: "Local directory to copy into volume"},
				},
			},
			{
				Name:     "setup-compose",
				Summary:  "Register app + deploy project files in one command",
				Category: "docker",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "yes", Description: "App ID"},
					{Name: "--name", Type: "string", Default: "", Req: "yes", Description: "App name"},
					{Name: "--domain", Type: "string", Default: "", Req: "yes", Description: "Subdomain"},
					{Name: "--port", Type: "int", Default: "0", Req: "yes", Description: "App port"},
					{Name: "--command", Type: "string", Default: "", Req: "yes", Description: "Start command"},
					{Name: "--source", Type: "string", Default: "", Req: "yes", Description: "Local project directory"},
					{Name: "--compose-file", Type: "string", Default: "", Req: "no", Description: "Compose file name"},
					{Name: "--remote-path", Type: "string", Default: "", Req: "no", Description: "Remote path override"},
					{Name: "--setup-dns", Type: "bool", Default: "false", Req: "no", Description: "Also setup DNS"},
					{Name: "--start", Type: "bool", Default: "false", Req: "no", Description: "Start after deploy"},
				},
				After: []string{"setup-dns", "setup-traefik"},
			},

			// ── Infrastructure ─────────────────────────────────────────
			{
				Name:     "prune",
				Summary:  "Remove DNS/Traefik config for an app (--id) or rebuild all (--all). Run after remove.",
				Category: "infrastructure",
				Flags: []guideFlag{
					{Name: "--id", Type: "string", Default: "", Req: "no", Description: "App ID to prune"},
					{Name: "--all", Type: "bool", Default: "false", Req: "no", Description: "Rebuild Traefik config for all apps"},
				},
				Before: []string{"remove"},
			},
			{
				Name:     "traefik-system",
				Summary:  "Manage Traefik installation on targets (install, status, remove)",
				Category: "infrastructure",
				Flags: []guideFlag{
					{Name: "--status", Type: "bool", Default: "false", Req: "no", Description: "Check installation status"},
					{Name: "--force", Type: "bool", Default: "false", Req: "no", Description: "Force reinstall"},
					{Name: "--remove", Type: "bool", Default: "false", Req: "no", Description: "Remove Traefik installation"},
				},
				Before: []string{"setup", "init"},
			},

			// ── Remote Management ──────────────────────────────────────
			{
				Name:     "auth",
				Summary:  "Authenticate with remote hotify daemon (add, list, remove, test targets)",
				Category: "remote",
				Flags: []guideFlag{
					{Name: "--action", Type: "string", Default: "", Req: "yes", Description: "add, list, remove, or test"},
					{Name: "--url", Type: "string", Default: "", Req: "for-add", Description: "Remote daemon URL"},
					{Name: "--token", Type: "string", Default: "", Req: "for-add", Description: "Auth token"},
					{Name: "--name", Type: "string", Default: "", Req: "for-add-remove", Description: "Target name"},
				},
				Before: []string{"targets"},
			},
			{
				Name:     "targets",
				Summary:  "Manage deployment targets (list, use, validate, remove)",
				Category: "remote",
				Flags: []guideFlag{
					{Name: "--action", Type: "string", Default: "", Req: "yes", Description: "list, use, validate, or remove"},
					{Name: "--name", Type: "string", Default: "", Req: "no", Description: "Target name"},
				},
				After: []string{"auth"},
			},
			{
				Name:     "api-keys",
				Summary:  "Manage API keys on remote daemon (add, list, remove, regenerate, permissions)",
				Category: "remote",
			},

			// ── Self-description ───────────────────────────────────────
			{
				Name:     "guide",
				Summary:  "Emit the full command catalog as JSON (default) or prose (--text). Agent entry point.",
				Category: "meta",
				Flags: []guideFlag{
					{Name: "--text", Type: "bool", Default: "false", Req: "no", Description: "Human-readable prose output"},
				},
				Note: "Run this first as an AI agent. The JSON output is the source of truth for all commands.",
			},
			{
				Name:     "help",
				Summary:  "Show this help message",
				Category: "meta",
			},
			{
				Name:     "version",
				Summary:  "Print version number",
				Category: "meta",
			},
		},
		Tips: []guideNote{
			{
				Topic: "safe-apply",
				Detail: "Always preview before applying: setup-traefik --id <app> --dry-run --local. " +
					"Review the diff, then run without --dry-run. Never skip this — setup-traefik " +
					"regenerates ALL apps' dynamic config from config.json.",
			},
			{
				Topic: "cloudflare-token-sync",
				Detail: "Remote servers need the Cloudflare token for ACME DNS challenges. " +
					"Copy config: scp ~/.hotify/config.json <host>:/tmp/ && " +
					"ssh <host> 'mkdir -p ~/.hotify && mv /tmp/hotify-config.json ~/.hotify/config.json'. " +
					"Or use --challenge-type http instead (no CF token needed).",
			},
			{
				Topic: "prefer-dns-challenge",
				Detail: "DNS challenge (--challenge-type dns) is more reliable than HTTP challenge. " +
					"HTTP requires port 80 accessible from the internet and can fail with 404 errors " +
					"during initial ACME setup. DNS challenge works even if the app isn't running yet.",
			},
			{
				Topic: "acme-timing",
				Detail: "After setup-traefik, wait 10-30s for Let's Encrypt certificate generation before " +
					"testing the domain. Verify in /etc/traefik/acme.json.",
			},
			{
				Topic: "basic-auth-apply",
				Detail: "basic-auth only modifies config.json. The Traefik middleware isn't created until " +
					"you run setup-traefik --id <app> --local. Always call setup-traefik after adding/removing users.",
			},
			{
				Topic: "cross-suggestions",
				Detail: "Commands suggest next steps: after setup-dns → suggests setup-traefik if missing; " +
					"after setup-traefik → suggests setup-dns if missing. Don't skip these — " +
					"if DNS is missing the domain won't resolve; if Traefik is missing the cert won't issue.",
			},
			{
				Topic: "iliffe-cluster-dk-access",
				Detail: "On the JAR network, always use --local for DNS/Traefik operations on dk1. " +
					"This bypasses the remote daemon auth and avoids SSH key issues.",
			},
		},
		Gotchas: []guideNote{
			{
				Topic: "backend-url-remote",
				Detail: "Never use 127.0.0.1 as backend URL for remote apps. Traefik runs on the proxy server (dk1), " +
					"so 127.0.0.1 points to the proxy, not the remote service. Always use the remote machine's " +
					"Tailscale IP (e.g., http://100.123.0.125:7000). Verify after setup: grep url /etc/traefik/dynamic.yml.",
			},
			{
				Topic: "domain-duplication",
				Detail: "Domains can get duplicated (app.example.com.example.com) if re-running setup with " +
					"existing config. Fix: sed -i 's/app.example.com.example.com/app.example.com/g' " +
					"~/.hotify/config.json && sudo systemctl restart traefik.",
			},
			{
				Topic: "tailscale-funnel-port-443",
				Detail: "Tailscale Funnel uses port 443 and conflicts with Traefik's HTTPS endpoint. " +
					"If you get connection resets on port 443, check: tailscale funnel status. " +
					"Disable with: tailscale funnel reset. Also check iptables for redirect rules.",
			},
			{
				Topic: "process-manager-conflict",
				Detail: "Never run multiple process managers for the same app. If migrating an app from systemd " +
					"to hotify: Add to hotify → Setup DNS/Traefik → Test → Stop old manager → Start via hotify. " +
					"Check for conflicts: systemctl list-units --all | grep <app>.",
			},
			{
				Topic: "setup-traefik-regenerates-all",
				Detail: "setup-traefik regenerates the ENTIRE /etc/traefik/dynamic.yml from config.json, not just " +
					"the specified app. Apps without backend_url in config.json will have their backend reset to " +
					"http://127.0.0.1:<port>. To prevent: always set backend_url via setup --backend-url for remote apps.",
			},
			{
				Topic: "no-traefik-reload",
				Detail: "Traefik does not support 'reload' (it returns an error). Always use restart: " +
					"sudo systemctl restart traefik. With watch:true in providers.file, Traefik auto-reloads " +
					"when dynamic.yml changes — no restart needed.",
			},
			{
				Topic: "http-challenge-redirect",
				Detail: "HTTP-to-HTTPS redirect breaks ACME HTTP challenges. hotify-cli handles this automatically " +
					"via smart redirect: temporarily disables redirect, gets the cert, then re-enables. " +
					"If ACME fails with HTTP challenge, use --challenge-type dns instead.",
			},
			{
				Topic: "path-prefix-manual",
				Detail: "--path-prefix is stored in config.json but setup-traefik does NOT generate the Traefik " +
					"addPrefix middleware. You must manually edit /etc/traefik/dynamic.yml to add the " +
					"middleware config. This is a known limitation (GitHub issue #1).",
			},
		},
		Workflows: []guideWorkflow{
			{
				Name:  "new-app",
				Steps: []string{"init", "setup", "setup-dns", "setup-traefik", "start"},
				Detail: "1. init (once) → 2. setup --id <id> --name <n> --domain <d> --port <p> --cmd <c> → " +
					"3. setup-dns --id <id> → 4. setup-traefik --id <id> → 5. start --id <id>",
			},
			{
				Name:  "new-app-remote-backend",
				Steps: []string{"init", "setup", "setup-dns", "setup-traefik"},
				Detail: "For apps on a different machine (Tailscale). Use --backend-url during setup: " +
					"setup --id <id> --backend-url http://<tailscale-ip>:<port>. Then setup-dns + setup-traefik as normal.",
			},
			{
				Name:  "update-app",
				Steps: []string{"setup", "setup-dns", "setup-traefik", "restart"},
				Detail: "setup --id <id> --port <new-port> (or other fields) → setup-traefik --id <id> → restart --id <id>",
			},
			{
				Name:  "remove-app",
				Steps: []string{"stop", "remove", "prune"},
				Detail: "stop --id <id> → remove --id <id> → prune --id <id>",
			},
			{
				Name:  "add-basic-auth",
				Steps: []string{"basic-auth", "setup-traefik"},
				Detail: "basic-auth --id <app> --action add --user <u> --password <p> → setup-traefik --id <app> --local",
			},
			{
				Name:  "initial-server-setup",
				Steps: []string{"init", "traefik-system", "setup", "setup-dns", "setup-traefik"},
				Detail: "1. init → 2. traefik-system (install Traefik) → 3. setup (register first app) → " +
					"4. setup-dns → 5. setup-traefik. Then install fail2ban (see AGENTS.md).",
			},
			{
				Name:  "deploy-compose-app",
				Steps: []string{"setup", "deploy-compose", "compose", "setup-dns", "setup-traefik"},
				Detail: "setup --id <id> --compose-file <f> --compose-path <p> → deploy-compose --id <id> --source <dir> → " +
					"compose --id <id> up -d → setup-dns → setup-traefik",
			},
		},
	}
}

// cmdGuide handles 'hotify-cli guide [--text]'
func cmdGuide(args []string) error {
	textMode := false
	for _, a := range args {
		if a == "--text" || a == "-t" {
			textMode = true
			break
		}
	}

	g := buildGuide()
	if textMode {
		fmt.Print(renderGuideText(g))
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(g)
}

func renderGuideText(g guideCatalog) string {
	var b strings.Builder
	fmt.Fprintf(&b, "hotify-cli %s — %s\n\n", g.Version, g.Tagline)

	// Group by category
	catOrder := []string{"meta", "configuration", "deployment", "dns-traefik", "process", "docker", "infrastructure", "remote"}
	catNames := map[string]string{
		"meta":           "Self-description",
		"configuration":  "Configuration",
		"deployment":     "Deployment",
		"dns-traefik":    "DNS & Traefik",
		"process":        "Process Management",
		"docker":         "Docker",
		"infrastructure": "Infrastructure",
		"remote":         "Remote Management",
	}

	byCat := make(map[string][]guideCommand)
	for _, cmd := range g.Commands {
		byCat[cmd.Category] = append(byCat[cmd.Category], cmd)
	}

	for _, cat := range catOrder {
		cmds, ok := byCat[cat]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n%s:\n", catNames[cat])
		for _, cmd := range cmds {
			fmt.Fprintf(&b, "  %s\n", cmd.Name)
			if cmd.Summary != "" {
				fmt.Fprintf(&b, "    %s\n", cmd.Summary)
			}
			if len(cmd.Flags) > 0 {
				for _, f := range cmd.Flags {
					req := ""
					if f.Req == "yes" || f.Req == "yes-in-json" {
						req = " [required]"
					}
					fmt.Fprintf(&b, "    %-25s %s%s\n", f.Name+" <"+f.Type+">", f.Description, req)
				}
			}
			if len(cmd.Before) > 0 {
				fmt.Fprintf(&b, "    ↤ before: %s\n", strings.Join(cmd.Before, ", "))
			}
			if len(cmd.After) > 0 {
				fmt.Fprintf(&b, "    ↦ after:  %s\n", strings.Join(cmd.After, ", "))
			}
			if cmd.Note != "" {
				fmt.Fprintf(&b, "    ⓘ %s\n", cmd.Note)
			}
		}
	}

	if len(g.Tips) > 0 {
		fmt.Fprintf(&b, "\nTips:\n")
		for _, t := range g.Tips {
			fmt.Fprintf(&b, "  %s:\n    %s\n", t.Topic, t.Detail)
		}
	}

	if len(g.Gotchas) > 0 {
		fmt.Fprintf(&b, "\nGotchas:\n")
		for _, gk := range g.Gotchas {
			fmt.Fprintf(&b, "  %s:\n    %s\n", gk.Topic, gk.Detail)
		}
	}

	fmt.Fprintf(&b, "\nWorkflows:\n")
	for _, w := range g.Workflows {
		fmt.Fprintf(&b, "  %s:\n", w.Name)
		fmt.Fprintf(&b, "    %s\n", w.Detail)
	}

	return b.String()
}
