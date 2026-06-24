# Hotify CLI - Agent Documentation (v2.10.1)

## Agent Quick Start

> **Run `hotify-cli guide` first.** It emits the complete command catalog —
> every command, its flags, the canonical ordering for common workflows, and
> cross-command suggestions — as JSON (agent-friendly). The catalog lives in
> the binary so it is always version-exact.
>
> ```sh
> hotify-cli guide                       # full catalog as JSON
> hotify-cli guide --text                 # human-readable prose
> hotify-cli guide | jq '.commands[] | select(.category == "dns-traefik")'
> hotify-cli guide | jq '.workflows[] | select(.name == "new-app")'
> ```

## Overview
Hotify is a CLI+UI tool for managing Traefik/Cloudflare app deployment. It automates DNS setup, SSL certificates, and reverse proxy configuration for web apps.

**Default output is JSON** (machine-readable). Add `--human` for human-readable text.

**Transport**: All remote operations use the hotify HTTP API (no SSH required). Set `ssh_host` on targets only for the legacy `traefik-system` command.

## v2.6.0 CLI Structure

```
init          Initialize config (non-interactive in JSON mode, requires --token --domain --email)
setup         Create or update an app (upsert) — replaces old add/edit
add           Strict create (fails if ID exists) — legacy compat
remove        Remove from config (does NOT clean DNS/Traefik — use prune)
list          List all apps

start [--id]  Start remote app (with --id) or hotify daemon (without --id --daemon). Add --local for local execution.
stop  [--id]  Stop remote app (SIGTERM) or hotify daemon. Add --local for local execution.
restart --id  Restart remote app. Add --local for local execution.
status [--id] Remote app status or daemon status. Add --local for local execution.
pause  --id    Pause remote app (SIGSTOP). Add --local for local execution.
resume --id    Resume paused remote app (SIGCONT). Add --local for local execution.

deploy        File transfer only: --id --source required

setup-dns     Create/update Cloudflare DNS A record: --id --ip (auto-detected if omitted)
setup-traefik Configure Traefik routing for an app: --id [--challenge-type http|dns]
basic-auth    Manage Traefik HTTP basic auth credentials: --id --action add|remove|list

## External Reverse Proxy Support

Hotify-cli supports external reverse proxy targets, allowing apps to run on different machines while hotify handles DNS, TLS, and Traefik routing. This is useful for:
- Apps running on different servers via Tailscale/VPN
- Containerized apps on separate hosts
- Microservices architectures across multiple machines

### Usage

Add the `--backend-url` parameter when setting up an app:

```bash
hotify-cli setup \
  --id myapp \
  --name "My App" \
  --domain myapp \
  --port 3000 \
  --cmd "/usr/local/bin/myapp start" \
  --backend-url "http://100.114.4.57:8080"
```

When `--backend-url` is set:
- Traefik will route traffic to the specified URL instead of `http://127.0.0.1:<port>`
- The app can run on any reachable machine (local network, Tailscale, VPN)
- DNS and TLS certificate management still handled by hotify
- Basic auth and other Traefik middleware still apply

### Example: Tailscale Network

```bash
# App running on remote container via Tailscale
hotify-cli setup \
  --id remote-app \
  --name "Remote App" \
  --domain app.example.com \
  --port 3000 \
  --cmd "/usr/local/bin/app start" \
  --backend-url "http://<tailscale-ip>:3000"

# Setup DNS and Traefik
hotify-cli setup-dns --id remote-app
hotify-cli setup-traefik --id remote-app
```

### Notes

- The `--port` parameter is still required for validation but not used when `--backend-url` is set
- Backend URL must be reachable from the Traefik server
- Supports HTTP/HTTPS URLs
- Can be updated using `hotify-cli setup --id <app> --backend-url <new-url>`

### Remote Service Proxying Best Practices

**Service Binding**
- Bind backend services to `0.0.0.0` instead of `127.0.0.1` for remote access
- Verify with `ss -tlnp | grep <port>` (should show 0.0.0.0, not 127.0.0.1)
- Use authentication (passwords, basic auth) when binding to 0.0.0.0

**ACME Certificate Timing**
- Wait 10-30 seconds after Traefik setup before testing domain access
- Let's Encrypt certificate generation takes time to complete
- Verify certificate in `/etc/traefik/acme.json` before testing

**Local Flag Usage**
- Use `--local` flag for DNS/Traefik setup when remote daemon has auth issues
- Local mode executes directly on the machine, avoiding remote API authentication
- Example: `hotify-cli setup-dns --id <app> --ip <ip> --local`

**Domain Naming Patterns**
- Use descriptive subdomains for multi-machine deployments
- Examples: `app-server1.example.com`, `app-server2.example.com`
- Benefits: Clear service location identification, prevents domain conflicts

**Network Connectivity**
- Verify proxy server can reach backend service before hotify setup
- Test with: `ssh <proxy-server> "curl -s http://<backend-ip>:<port>/health"`
- Common pattern: Use public server as proxy for private containers/machines via Tailscale

## Cleanup

prune         Cleanup DNS/Traefik: --id <app> or --all

# Docker Compose passthrough
compose [--id <app>] <subcommand> [args...]   # app-aware docker compose passthrough

# Docker Compose Deployment Automation (v2.6.0)
deploy-compose   Copy full project tree to remote compose_path via HTTP API
compose-sync     Sync compose file (+ .env) only — faster than full deploy
compose-copy-dir Copy a specific local directory into remote compose_path
volume-init      Populate a Docker named volume with local directory content
setup-compose    Register app config + deploy project files in one command

traefik-system  Install/manage Traefik on target
auth          Authenticate with remote daemon
targets       Manage deployment targets
api-keys      Manage API keys
```

## v2.6.0 Docker Compose Deployment Commands

### deploy-compose (most commonly used)

Replaces the manual `scp` workflow. Copies the full project tree (compose file, .env, webui/, etc.) to the remote `compose_path` in one command.

```bash
# Full deploy of a compose project
hotify-cli deploy-compose \
  --id cir-doc-gen \
  --source /local/path/to/project

# With explicit compose file and remote path override
hotify-cli deploy-compose \
  --id cir-doc-gen \
  --source /local/path/to/project \
  --compose-file docker-compose.yml \
  --remote-path /home/dk1/cir-doc-gen

# Deploy and start immediately
hotify-cli deploy-compose --id cir-doc-gen --source ./project --start
```

Replaces this manual workflow:
```bash
# OLD (manual):
scp docker-compose.yml dk1@host:/home/dk1/cir-doc-gen/
scp .env dk1@host:/home/dk1/cir-doc-gen/
scp -r webui dk1@host:/home/dk1/cir-doc-gen/
```

### compose-sync

Sync only the compose file (and optionally `.env`) without re-uploading the full project tree. Useful after editing docker-compose.yml.

```bash
# Sync from current directory
hotify-cli compose-sync --id cir-doc-gen

# Sync from specific directory, then restart
hotify-cli compose-sync --id cir-doc-gen --source ./project --restart

# Sync compose file only (skip .env)
hotify-cli compose-sync --id cir-doc-gen --env=false
```

### compose-copy-dir

Copy a specific local directory into the app's remote `compose_path`.

```bash
# Copy webui/ directory
hotify-cli compose-copy-dir \
  --id cir-doc-gen \
  --dir webui \
  --source /local/path/webui

# Copy templates/
hotify-cli compose-copy-dir --id cir-doc-gen --dir templates --source ./templates
```

Replaces: `scp -r webui dk1@host:/home/dk1/cir-doc-gen/`

### volume-init

Populate a Docker named volume with local directory contents.

```bash
hotify-cli volume-init \
  --id cir-doc-gen \
  --volume cir-webui \
  --source /home/dk1/cir-doc-gen/webui
```

Volume name resolution: `<app_id>_<volume>` → `/var/lib/docker/volumes/cir-doc-gen_cir-webui/_data/`

**NOTE**: The remote hotify daemon must have write access to `/var/lib/docker/volumes/` (root or Docker group membership on the remote host).

Replaces: `sudo cp -r /home/dk1/cir-doc-gen/webui/* /var/lib/docker/volumes/cir-doc-gen_cir-webui/_data/`

### setup-compose

Combines app registration (`setup`) and project file deployment in a single command.

```bash
hotify-cli setup-compose \
  --id cir-doc-gen \
  --name "CIR Doc Gen" \
  --domain cir-doc-gen \
  --port 8080 \
  --cmd "docker compose up -d" \
  --source /local/project \
  --compose-file docker-compose.yml \
  --remote-path /home/dk1/cir-doc-gen \
  --setup-dns \
  --start
```

## Agent Usage

### Quick Start for Agents

1. **Initialize Configuration** (non-interactive, agent-friendly):
```bash
hotify-cli init --token <cf-api-token> --domain example.com --email admin@example.com
```
- Returns JSON with `warnings` array (includes Traefik warning if not installed)
- Use `--human` flag for interactive prompts

2. **Setup App** (create or update):
```bash
hotify-cli setup \
  --id app-id \
  --name "App Name" \
  --domain subdomain \
  --port 3000 \
  --cmd "/path/to/binary start"
```
- With DNS auto-setup: add `--setup-dns` (IP auto-detected via ifconfig.me)
- With explicit IP: add `--setup-dns --ip 1.2.3.4`
- Update only port: `hotify-cli setup --id app-id --port 4000`

3. **Deploy Binary**:
```bash
hotify-cli deploy --id app-id --source ./mybinary
hotify-cli deploy --id app-id --source ./mybinary --setup-dns  # DNS too
```

4. **Start/Stop/Restart**:
```bash
# Remote mode (default - requires target)
hotify-cli start   --id app-id
hotify-cli stop    --id app-id   # sends SIGTERM
hotify-cli restart --id app-id
hotify-cli status  --id app-id
hotify-cli pause   --id app-id   # SIGSTOP (freeze)
hotify-cli resume  --id app-id   # SIGCONT (unfreeze)

# Local mode (execute directly on local server, no target needed)
hotify-cli start   --id app-id --local
hotify-cli stop    --id app-id --local
hotify-cli restart --id app-id --local
hotify-cli status  --id app-id --local
hotify-cli pause   --id app-id --local
hotify-cli resume  --id app-id --local
```

5. **Remove + Cleanup**:
```bash
hotify-cli remove --id app-id        # removes from config, warns about DNS/Traefik
hotify-cli prune  --id app-id        # removes Traefik config, warns about DNS
hotify-cli prune  --all              # rebuilds Traefik for current app list
```

6. **Setup Traefik**:
```bash
hotify-cli traefik-system --target <name>   # install Traefik on target
hotify-cli traefik-system --status          # check status
```

### Pause/Resume Behavior

- **Pause** (`SIGSTOP`): Freezes the process without killing it. The app remains in memory but stops consuming CPU.
- **Resume** (`SIGCONT`): Unfreezes a paused process, resuming execution.
- **Remote Mode**: Requires a configured target. The remote hotify daemon must track the app's PID in the config (`App.PID` field).
- **Local Mode** (`--local` flag): Execute directly on local server without requiring a target. Reads PID from local config and sends signals directly.
- **Limitations**:
  - Paused processes still hold memory and file descriptors
  - If the daemon restarts, PID tracking is lost
  - SIGTERM may not work on paused processes (resume first or force-kill)
  - Orphaned PIDs (process died externally) need manual cleanup
  - Local mode requires the hotify-cli config to be on the same machine as the app

### CLI Commands for Agents

```bash
# Initialize configuration
hotify-cli init

# Add new app
hotify-cli add --id <id> --name "<name>" --domain <subdomain> --port <port> --command "<command>"

# Edit existing app
hotify-cli edit --id <id> [--name <name>] [--domain <domain>] [--port <port>] [--command <command>]

# Remove app
hotify-cli remove --id <id>

# List all apps
hotify-cli list

# Start web UI daemon
hotify-cli start --daemon

# Check daemon status
hotify-cli status

# Stop daemon
hotify-cli stop
```

### App Requirements

Apps must follow this pattern:
- Single binary executable
- Support `start`/`stop` commands
- Expose HTTP server on a port
- No built-in SSL (Traefik handles SSL)

Example:
```bash
myapp start --port 3000  # Starts HTTP server
myapp stop               # Stops daemon
```

### Configuration File

Located at `~/.hotify/config.json`:
```json
{
  "cloudflare_token": "token",
  "domain": "example.com",
  "admin_email": "admin@example.com",
  "apps": [
    {
      "id": "app-id",
      "name": "App Name",
      "domain": "app.example.com",
      "port": 3000,
      "command": "/path/to/app start",
      "source": "github.com/user/repo",
      "status": "stopped"
    }
  ]
}
```

### Common Agent Workflows

#### Deploy New App
```bash
# 1. Add app configuration
hotify-cli add \
  --id myapp \
  --name "My App" \
  --domain myapp \
  --port 3000 \
  --command "/home/user/myapp start" \
  --source "github.com/user/myapp"

# 2. Deploy binary to server (manual step)
# scp myapp user@server:/home/user/myapp

# 3. Setup DNS
hotify-cli setup-dns --id myapp --ip 92.113.145.178

# 4. Setup Traefik
hotify-cli setup-traefik --id myapp

# 5. Access at https://myapp.example.com
```

#### Update Existing App
```bash
# Update configuration
hotify-cli edit --id myapp --port 4000

# Update Traefik config
hotify-cli setup-traefik --id myapp
```

#### Remove App
```bash
# Remove from configuration
hotify-cli remove --id myapp

# Update Traefik to remove routing
hotify-cli setup-traefik --id myapp
```

#### Basic Auth Protection

Add HTTP basic authentication to protect apps behind Traefik:

```bash
# Add a user with password (hashed client-side with APR1-MD5)
hotify-cli basic-auth --id myapp --action add --user admin --password secret

# Add a pre-hashed entry (htpasswd format)
hotify-cli basic-auth --id myapp --action add --hash 'admin:$apr1$...'

# List users (passwords masked)
hotify-cli basic-auth --id myapp --action list

# Remove a user
hotify-cli basic-auth --id myapp --action remove --user admin

# Apply changes to Traefik
hotify-cli setup-traefik --id myapp
```

**Notes:**
- Passwords are hashed using APR1-MD5 (htpasswd compatible)
- Pre-hashed entries can be imported from existing htpasswd files
- After adding/removing users, run `setup-traefik` to apply changes
- Basic auth middleware is only added to Traefik for apps with users configured

#### Remote Execution

The `basic-auth`, `setup-traefik`, and `setup-dns` commands support remote execution via HTTP API, allowing developers to run commands from their local machine without SSH access to the server:

```bash
# Remote mode (default - uses configured target)
hotify-cli basic-auth --id myapp --action list
hotify-cli setup-traefik --id myapp
hotify-cli setup-dns --id myapp

# Explicit target specification
hotify-cli basic-auth --id myapp --action list --target dk1
hotify-cli setup-traefik --id myapp --target dk1
hotify-cli setup-dns --id myapp --target dk1

# Local mode (execute directly on local server)
hotify-cli basic-auth --id myapp --action list --local
hotify-cli setup-traefik --id myapp --local
hotify-cli setup-dns --id myapp --local
```

**Remote Endpoints:**
- `POST /api/remote/apps/{id}/basic-auth` - Manage basic auth credentials
- `POST /api/remote/apps/{id}/setup-traefik` - Configure Traefik routing
- `POST /api/remote/apps/{id}/setup-dns` - Configure Cloudflare DNS

**Use Case:** DevOps installs hotify-cli on the remote server (dk1), developers use hotify-cli locally with configured targets to manage apps without SSH access.

### Web UI for Admins

Start daemon for human admins:
```bash
hotify-cli start --daemon
# Access at http://localhost:8080
```

Web UI provides:
- Configuration overview
- App management (add/edit/remove)
- One-click DNS setup
- One-click Traefik configuration
- Real-time status updates

### Traefik Integration

Hotify manages these files:
- `/etc/traefik/traefik.yml` - Main configuration
- `/etc/traefik/dynamic.yml` - Routing rules
- `/etc/traefik/cloudflare.env` - Cloudflare credentials
- `/etc/traefik/acme.json` - SSL certificates
- `/etc/systemd/system/traefik.service` - Systemd service

### DNS Integration

Hotify uses Cloudflare API to:
- Get zone ID for domain
- Create A records for subdomains
- Configure DNS-only mode (proxy disabled)

## Post-Install Security: Traefik Access Logs + Fail2ban

After setting up Traefik (via `hotify-cli traefik-system`), **you MUST enable access logs and fail2ban** to protect against brute-force attacks on basic-auth-protected apps. Traefik's `basicAuth` middleware is stateless — it checks credentials on every request without tracking failures, making it trivially bypassable without this setup.

### Step 1: Enable Traefik access logs

Add `accessLog` to `/etc/traefik/traefik.yml`:

```yaml
accessLog:
  filePath: "/var/log/traefik/access.log"
  bufferingSize: 100
```

Then restart Traefik:
```bash
sudo mkdir -p /var/log/traefik
sudo systemctl restart traefik
```

Verify logs are being written:
```bash
sudo tail -f /var/log/traefik/access.log
# → 1.2.3.4 - admin [17/Jun/2026:06:47:47 +0000] "GET / HTTP/2.0" 401 17 ...
```

### Step 2: Install fail2ban Traefik jail

Create `/etc/fail2ban/filter.d/traefik-auth-local.conf`:
```ini
[Definition]
failregex = <HOST> - \S+ \[.*\] "\S+ \S+ \S+" 401\s
ignoreregex =
```

Create `/etc/fail2ban/jail.d/traefik-auth.local`:
```ini
[traefik-auth]
enabled   = true
filter    = traefik-auth-local
backend   = polling
port      = http,https
logpath   = /var/log/traefik/access.log
maxretry  = 5
findtime  = 600
bantime   = 3600
ignoreip  = 127.0.0.0/8 <server-public-ip>
```

Reload fail2ban and verify:
```bash
sudo fail2ban-client reload
sudo fail2ban-client status traefik-auth
```

**Make sure to add the server's own public IP to `ignoreip`** so internal services aren't locked out.

### How it works

| Threshold | Action |
|-----------|--------|
| 5 failed auth attempts in 10 minutes | iptables ban on ports 80/443 for 1 hour |
| All Traefik-proxied services blocked | Brute force on any app → locked out of all apps |

This means an attacker failing basic auth on powersentry gets blocked from ALL hotify-managed apps on that server (cmdcenter, discovery, etc.).

### Verification

```bash
# Simulate bad auth attempts (from outside the server)
for i in $(seq 1 6); do
  curl -s -o /dev/null -u "admin:wrong$i" \
    https://your-app.domain.com/ 2>/dev/null
done

# Check fail2ban
sudo fail2ban-client status traefik-auth
# → Banned IP list: your.ip.here (after 5th attempt, 6th gets blocked)

# Unban if needed
sudo fail2ban-client set traefik-auth unbanip <banned-ip>
```

### Prerequisites for Agents

- Cloudflare API token with DNS edit permissions
- Domain managed in Cloudflare
- Server with systemd support
- SSH access to target server (for Traefik system management)
- App binary or folder ready for deployment
- Target configured with SSH host for system management

**Agent-Friendly Features:**
- **JSON output by default** - all commands output structured JSON for machine readability
- Add `--human` flag for human-readable text output when needed
- Semantic exit codes for programmatic decision making
- Idempotent operations (safe to run multiple times)
- Non-interactive execution suitable for automation
- Structured error messages with suggestions for recovery

### JSON Output Support

**JSON is the default output format** for all agent-friendly commands. Add `--human` flag for human-readable text output.

The following commands support JSON output:

#### Fully Supported Commands (JSON by default)
- **Authentication**: `hotify-cli auth --action <action>` (JSON by default, add `--human` for text)
- **API Keys**: `hotify-cli api-keys --action <action>` (JSON by default, add `--human` for text)
- **Targets**: `hotify-cli targets --action <action>` (JSON by default, add `--human` for text)
- **Deployment**: `hotify-cli deploy --id <id> --action <action>` (JSON by default, add `--human` for text)
- **Traefik System**: `hotify-cli traefik-system [--status|--remove|--force]` (JSON by default, add `--human` for text)
- **Daemon Management**: `hotify-cli start --daemon`, `hotify-cli stop`, `hotify-cli status` (JSON by default, add `--human` for text)

#### Interactive Commands (JSON Not Supported)
- **init**: Interactive setup requiring user input
- **add/edit/remove/list**: Text-only output (flag parsing limitations)

#### JSON Output Structure
All JSON responses follow this consistent structure:
```json
{
  "version": "1.0.0",
  "success": true|false,
  "data": {
    // Command-specific data
  },
  "error": {
    "code": <exit_code>,
    "type": "<error_type>",
    "message": "<error_message>",
    "recoverable": true|false,
    "suggestions": ["<suggestion1>", "<suggestion2>"]
  },
  "metadata": {
    "timestamp": <unix_timestamp>
  }
}
```

#### Semantic Exit Codes
- `0` - Success
- `1` - Generic failure
- `90` - Traefik not installed
- `91` - Traefik already installed
- `92` - Traefik installation failed
- `93` - Traefik service failed
- `94` - Traefik configuration invalid
- `95` - Permissions error
- `96` - Target not found
- `97` - Invalid argument
- `98` - Configuration error
- `99` - Daemon error
- `105` - Connection timeout

#### Example JSON Usage
```bash
# Check daemon status (JSON by default)
hotify-cli status
# Response: {"version":"1.0.0","success":true,"data":{"status":"not_running"},"metadata":{"timestamp":1778999882}}

# Get human-readable output
hotify-cli status --human
# Response: ✅ Success
#          status: not_running

# Check Traefik installation status (JSON by default)
hotify-cli traefik-system --status
# Response: {"version":"1.0.0","success":true,"data":{"status":{"installed":true,...}}}

# Deploy application (JSON by default)
hotify-cli deploy --id myapp --source ./myapp
# Response: {"version":"1.0.0","success":true,"data":{"app_id":"myapp","deployment_type":"binary",...}}
```

### Troubleshooting

#### Configuration Issues
```bash
# Check config file
cat ~/.hotify/config.json

# Reinitialize if needed
rm ~/.hotify/config.json
hotify-cli init
```

#### DNS Issues
- Verify Cloudflare token permissions
- Check domain is in Cloudflare
- Ensure server IP is correct
- Check Cloudflare dashboard

#### Traefik Issues
```bash
# Check Traefik status
sudo systemctl status traefik

# View logs
sudo journalctl -u traefik -f

# Restart Traefik
sudo systemctl restart traefik
```

#### Daemon Issues
```bash
# Check status
hotify-cli status

# View logs
cat /tmp/hotify-cli.log

# Restart daemon
hotify-cli stop
hotify-cli start --daemon
```

### Authentication & Security

Hotify includes a comprehensive authentication system for remote daemon access:

#### Bootstrap Token
When the daemon starts with no API keys, it automatically generates a secure bootstrap token:
```bash
hotify-cli start --daemon
# Check logs for the generated token:
cat /tmp/hotify-cli.log
```

The bootstrap token is displayed with clear instructions:
```
==============================================
Initial admin key created successfully!
Token: <generated-token>
==============================================
IMPORTANT: Store this token securely!
```

#### Authentication Commands
```bash
# Authenticate with remote daemon
hotify-cli auth --action add --url <url> --token <token> --name <name>

# List authenticated remotes
hotify-cli auth --action list

# Remove remote authentication
hotify-cli auth --action remove --name <name>

# Test connection to remote
hotify-cli auth --action test [--name <name>]
```

#### Target Management
```bash
# List all configured targets
hotify-cli targets --action list

# Set default target
hotify-cli targets --action use --name <name>

# Validate target connectivity
hotify-cli targets --action validate [--name <name>]

# Remove target
hotify-cli targets --action remove --name <name>
```

#### Traefik System Management
Hotify can automatically set up Traefik on bare VMs with idempotent installation:

```bash
# Check Traefik installation status (JSON output for agent consumption)
hotify-cli traefik-system --status --json

# Install Traefik (idempotent - skips if already installed)
hotify-cli traefik-system --json

# Force reinstall Traefik
hotify-cli traefik-system --force --json

# Remove Traefik installation
hotify-cli traefik-system --remove --json
```

**Agent Workflow for Traefik Setup:**

When deploying to a target that may not have Traefik installed:

```bash
# 1. Check if Traefik is installed
hotify-cli traefik-system --status --json

# 2. If not installed (exit code 90), install it
hotify-cli traefik-system --json

# 3. Proceed with app deployment
hotify-cli deploy --id myapp --source ./myapp
```

**Exit Codes for Agent Decision Making:**
- `0` - Success
- `90` - Traefik not installed (agent should run install command)
- `91` - Traefik already installed (use --force to reinstall)
- `92` - Traefik installation failed
- `93` - Traefik service failed
- `94` - Traefik configuration invalid
- `95` - Permissions error
- `96` - Target not found

**JSON Output Structure:**
```json
{
  "version": "1.0",
  "success": true,
  "data": {
    "status": {
      "installed": true,
      "version": "Version:      2.9.0",
      "status": "active",
      "binary_path": "/usr/local/bin/traefik",
      "config_dir": "/etc/traefik",
      "service_name": "traefik.service",
      "systemd_enabled": true
    },
    "actions_taken": ["installed_binary", "created_config_dir", "setup_systemd_service", "started_service"]
  },
  "error": {
    "code": 91,
    "type": "traefik_already_installed",
    "message": "Traefik is already installed",
    "recoverable": true,
    "suggestions": ["Use --force to reinstall", "Use --status to check current installation"]
  }
}
```

**Target Configuration for Traefik Setup:**

Targets must include SSH access for system management:

```json
{
  "name": "production",
  "url": "http://192.168.1.100:3060",
  "ssh_host": "user@192.168.1.100",
  "auth_token": "encrypted_token",
  "permissions": ["deploy", "start", "stop"],
  "default": true
}
```

#### Deployment System
Hotify includes a full deployment system supporting both binary and folder-based applications:

```bash
# Deploy single binary
hotify-cli deploy --id myapp --source ./myapp-binary --json

# Deploy folder (Node.js/Bun/Python apps)
hotify-cli deploy --id myapp --source ./myapp-folder --json

# Start deployed application
hotify-cli deploy --id myapp --action start --json

# Stop deployed application
hotify-cli deploy --id myapp --action stop --json

# Restart deployed application
hotify-cli deploy --id myapp --action restart --json

# Check application status
hotify-cli deploy --id myapp --action status --json
```

**Deployment Features:**
- Supports single binary files (Go, Rust, etc.)
- Supports folder deployment with automatic tar/gzip compression (Node.js, Bun, Python)
- Automatic deployment type detection (file vs folder)
- Target-aware deployment (uses configured targets)
- API-based deployment with authentication
- Source validation and cleanup

**Agent Deployment Workflow:**
```bash
# 1. Ensure Traefik is installed
hotify-cli traefik-system --status --json
# If exit code 90, install it:
hotify-cli traefik-system --json

# 2. Deploy application (binary or folder)
hotify-cli deploy --id myapp --source ./myapp --json

# 3. Start the application
hotify-cli deploy --id myapp --action start --json

# 4. Verify status
hotify-cli deploy --id myapp --action status --json
```

#### API Key Management
```bash
# Add new API key (local management)
hotify-cli api-keys --action add --name <name> --permissions <perms>

# List API keys
hotify-cli api-keys --action list

# Remove API key
hotify-cli api-keys --action remove --name <name>

# Regenerate token
hotify-cli api-keys --action regenerate --name <name>

# Update permissions
hotify-cli api-keys --action permissions --name <name> --add <perms> --remove <perms>
```

#### API Endpoints
- **Auth**: `/api/auth/login`, `/api/auth/validate`, `/api/auth/refresh`, `/api/auth/logout`
- **API Keys**: `/api/api-keys` (GET/POST), `/api/api-keys/{name}` (GET/DELETE/POST)
- All endpoints require Bearer token authentication

#### Permissions
- `deploy` - Deploy applications
- `start` - Start applications
- `stop` - Stop applications
- `restart` - Restart applications
- `logs` - View application logs
- `config` - Modify configuration
- `admin` - Full administrative access

#### Security Features
- AES-256 encryption for tokens at rest
- Token expiration (configurable, default 30 days)
- Rate limiting (configurable, default 60 req/min)
- Failed attempt tracking
- Audit logging for all security events
- IP whitelisting support

### Security Notes

- Cloudflare token stored in plaintext
- Set restrictive permissions on config file
- Use tokens with minimal permissions
- Consider environment variables for sensitive data
- Regularly rotate tokens
- Bootstrap token should be removed after setting up proper API keys
- Enable HTTPS in production (require_https config option)

### Limitations

- Single server deployment
- Requires specific app architecture
- No built-in app deployment (manual binary deployment required)
- No process management (Phase 3 - future)
- Cloudflare DNS only
- Requires SSH access for Traefik system management

### Best Practices

1. **Test in staging** before production
2. **Verify DNS propagation** before Traefik setup
3. **Monitor Traefik logs** for certificate issues
4. **Backup configuration** before changes
5. **Use descriptive app IDs** and names
6. **Document custom commands** for each app
7. **Regular security updates** for Traefik
8. **Monitor SSL certificate expiry**

### Migration & Local Management

#### Migrating Existing Services to Hotify

When an agent needs to migrate an existing service to hotify management:

```bash
# 1. Investigate current service setup
ps aux | grep app-name                      # Find process
systemctl --user status app-name             # Check systemd user service
sudo systemctl status app-name              # Check systemd system service
netstat -tlnp | grep :port                  # Check port usage

# 2. Stop existing process manager
systemctl --user stop app-name               # Stop systemd user service
# OR
sudo systemctl stop app-name                # Stop systemd system service

# 3. Add to hotify configuration
hotify-cli setup \
  --id app-name \
  --name "App Name" \
  --domain app-name \
  --port 8080 \
  --command "/path/to/app start"

# 4. Setup DNS and Traefik (can be done while service runs)
hotify-cli setup-dns --id app-name --ip <server-ip>
hotify-cli setup-traefik --id app-name

# 5. Start via hotify (use --local for same-machine management)
hotify-cli start --id app-name --local

# 6. Verify migration success
hotify-cli status --id app-name --local
curl -I https://app-name.example.com

# 7. Disable old service to prevent conflicts
systemctl --user disable app-name
# OR
sudo systemctl disable app-name
```

#### Local vs Remote Management for Agents

Agents should understand when to use `--local` flag:

**Use `--local` when:**
- Hotify-cli and app run on the same machine
- Managing services on the local server
- No remote daemon configuration needed
- Faster execution for local operations

**Skip `--local` when:**
- Managing apps on remote servers
- Using configured targets with auth tokens
- Distributed deployment scenarios
- Need HTTP API for remote operations

```bash
# Local management example
hotify-cli start --id myapp --local    # Direct PID management
hotify-cli status --id myapp --local   # Local config access

# Remote management example  
hotify-cli start --id myapp            # Via HTTP API to remote daemon
hotify-cli status --id myapp           # Remote status check
```

#### Multi-Port Application Considerations

For applications with multiple ports (HTTP + WebSocket, etc.):

```bash
# Primary HTTP port gets Traefik configuration
hotify-cli setup --id multiport-app --domain app --port 8080 --command "/app start"
hotify-cli setup-dns --id multiport-app --ip <server-ip>
hotify-cli setup-traefik --id multiport-app

# Secondary ports remain accessible via direct IP
# WebSocket example: ws://<server-ip>:8765
# Configure workers/clients to use direct IP for secondary ports
```

#### Process Conflict Detection

Agents should detect and prevent process manager conflicts:

```bash
# Check for conflicting systemd services
systemctl list-units --all | grep app-name
systemctl --user list-units --all | grep app-name

# Check for supervisor processes
sudo supervisorctl status all | grep app-name

# Check for running processes
ps aux | grep app-name | grep -v grep

# Resolution: disable conflicting managers
sudo systemctl disable conflicting-service
systemctl --user disable conflicting-service
sudo supervisorctl remove conflicting-service
```

#### Fallback Access Strategy

Maintain fallback access during migration:

```bash
# Phase 1: Both access methods available
curl http://<server-ip>:8080/health       # Direct access
curl https://app.example.com/health       # Domain access

# Phase 2: Update dependent services
# Update worker WebSocket URLs
# Update API client endpoints
# Update documentation

# Phase 3: Deprecate direct access
# Configure firewall rules
# Update monitoring systems
# Remove direct access from docs
```

### Example Agent Session

```bash
# Initialize hotify
hotify-cli init
# Enter: cca4b29bdc3123341e05ec99e36cd8a76f39c
# Enter: intrane.fr
# Enter: arancibiajav@gmail.com

# Add cmdcenter app
hotify-cli add \
  --id cmdcenter \
  --name "Command Center" \
  --domain cmdcenter \
  --port 3031 \
  --command "/usr/local/bin/cmdcenter start -daemon" \
  --source "github.com/jarancibia/cmdcenter"

# Setup DNS
hotify-cli setup-dns --id cmdcenter --ip 92.113.145.178

# Setup Traefik
hotify-cli setup-traefik --id cmdcenter

# Verify
curl https://cmdcenter.intrane.fr
```

### Notes for Agents

- Always run `hotify-cli init` first on new setups
- App IDs must be unique
- Domains are automatically prefixed with base domain
- Traefik setup requires sudo permissions
- DNS setup requires valid Cloudflare token
- Configuration changes are persistent
- Daemon runs in background with logging
