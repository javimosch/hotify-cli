# Hotify CLI - Agent Documentation

## Overview
Hotify is a CLI+UI tool for managing Traefik/Cloudflare app deployment. It automates DNS setup, SSL certificates, and reverse proxy configuration for web apps.

## Agent Usage

### Quick Start for Agents

1. **Initialize Configuration** (first time only):
```bash
hotify-cli init
```
Prompts for:
- Cloudflare API token (use legacy format)
- Base domain (e.g., intrane.fr)
- Admin email (for Let's Encrypt)

2. **Add an App**:
```bash
hotify-cli add \
  --id app-id \
  --name "App Name" \
  --domain subdomain \
  --port 3000 \
  --command "/path/to/binary start" \
  --source "github.com/user/repo"
```

3. **Setup DNS**:
```bash
hotify-cli setup-dns --id app-id --ip 92.113.145.178
```

4. **Setup Traefik**:
```bash
hotify-cli setup-traefik --id app-id
```

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

### Prerequisites for Agents

- Cloudflare API token with DNS edit permissions
- Domain managed in Cloudflare
- Server with systemd support
- SSH access to target server (for Traefik system management)
- App binary or folder ready for deployment
- Target configured with SSH host for system management

**Agent-Friendly Features:**
- All commands support `--json` flag for machine-readable output
- Semantic exit codes for programmatic decision making
- Idempotent operations (safe to run multiple times)
- Non-interactive execution suitable for automation

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
