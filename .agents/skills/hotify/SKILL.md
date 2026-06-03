---
name: hotify
description: Use this skill when working with hotify-cli for Traefik/Cloudflare app management, deployment, and infrastructure automation. Always prefer hotify HTTP API over SSH/scp when possible.
---

# Hotify CLI - Core Skill

Hotify is a CLI+UI tool for managing Traefik/Cloudflare app deployment. It automates DNS setup, SSL certificates, and reverse proxy configuration for web apps.

## No-Humans Mindset: Prefer Hotify Over SSH/SCP

**Core Principle**: If an operation can be done via hotify's HTTP API, always prefer it over SSH/scp. This enables true remote management without requiring SSH access to servers.

### Decision Tree for Operations
1. **Is there a hotify command for this operation?** → Use hotify CLI
2. **Does it support remote execution?** → Use `--target` flag (default)
3. **Only use SSH if**: hotify cannot perform the operation at all

### When to Use Hotify HTTP API (Always Preferred)
- **App configuration**: `setup`, `add`, `edit`, `remove`, `list`
- **Process management**: `start`, `stop`, `restart`, `status`, `pause`, `resume`
- **DNS & Traefik**: `setup-dns`, `setup-traefik`, `basic-auth`
- **Deployment**: `deploy`, `deploy-compose`, `compose-sync`, `compose-copy-dir`
- **Docker operations**: `compose`, `docker` commands
- **Infrastructure**: `traefik-system`

### When SSH Might Still Be Needed
- Initial hotify-cli installation on remote server
- System-level troubleshooting outside hotify's scope
- Manual file operations not supported by hotify API

## Remote Execution Pattern (v2.7.3+)

All app configuration commands support remote execution via HTTP API:

```bash
# Remote mode (default - uses configured target)
hotify-cli basic-auth --id myapp --action list
hotify-cli setup-traefik --id myapp
hotify-cli setup-dns --id myapp

# Explicit target specification
hotify-cli basic-auth --id myapp --action list --target dk1
hotify-cli setup-traefik --id myapp --target dk1

# Local mode (execute directly on local server)
hotify-cli basic-auth --id myapp --action list --local
hotify-cli setup-traefik --id myapp --local
```

## Key Commands

### App Management
- `hotify setup --id <id> --name <n> --domain <d> --port <p> --cmd <c> [--backend-url <url>] [--target <t> | --local]` — Create or update app
- `hotify add --id <id> --name <n> --domain <d> --port <p> --cmd <c> [--backend-url <url>] [--target <t> | --local]` — Strict create (fails if exists)
- `hotify remove --id <id> [--target <t> | --local]` — Delete app
- `hotify list [--target <t> | --local]` — List all apps

### Remote Process Management
- `hotify start --id <id>` — Start app (uses configured target by default)
- `hotify stop --id <id>` — Stop app
- `hotify restart --id <id>` — Restart app
- `hotify status --id <id>` — App status
- `hotify pause --id <id>` — Pause app (SIGSTOP)
- `hotify resume --id <id>` — Resume paused app (SIGCONT)

### App Configuration (Remote Execution Support)
- `hotify setup-dns --id <id> [--ip <ip>]` — Create/update Cloudflare DNS A record
- `hotify setup-traefik --id <id> [--challenge-type http|dns]` — Configure Traefik routing
- `hotify basic-auth --id <id> --action <add|remove|list>` — Manage Traefik HTTP basic auth

### Docker Compose Deployment
- `hotify deploy-compose --id <id> --source <dir> [--target <t> | --local]` — Copy full project tree to remote
- `hotify compose-sync --id <id> [--source <dir>] [--target <t> | --local]` — Sync compose file (+ .env) only
- `hotify compose-copy-dir --id <id> --dir <subdir> --source <local-dir> [--target <t> | --local]` — Copy directory to remote
- `hotify compose [--id <app>] [--target <t> | --local] <subcommand> [args...]` — Passthrough to docker compose (v2.10.0 adds remote support)

### Deployment
- `hotify deploy --id <id> --source <path> [--target <t> | --local]` — Deploy binary/folder (v2.10.0 adds --local)
- `hotify prune --id <id> [--target <t> | --local]` — Remove DNS/Traefik for app (v2.10.0 adds remote support)

### Authentication & Targets
- `hotify auth --url <u> --token <t> --name <n>` — Authenticate with remote daemon
- `hotify targets --action list` — List targets
- `hotify targets --action use --name <n>` — Set active target

### API Key Management (Local Only)
- `hotify api-keys --action add --name <n> [--permissions <p>]` — Create API key with permissions
- `hotify api-keys --action list` — List all API keys
- `hotify api-keys --action remove --name <n>` — Remove API key
- `hotify api-keys --action permissions --name <n> --add <p> --remove <p>` — Update permissions
- `hotify api-keys --action usage --name <n>` — Show API key usage statistics

**Available Permissions**: `deploy`, `start`, `stop`, `restart`, `logs`, `config`, `admin`, `all`, `*`
**Note**: Permissions are fully enforced at server level as of v2.7.4. Use `all` or `*` for full access.

## External Reverse Proxy Support (v2.8.1+)

Hotify-cli supports external reverse proxy targets, allowing apps to run on different machines while hotify handles DNS, TLS, and Traefik routing.

### Use Cases
- Apps running on different servers via Tailscale/VPN
- Containerized apps on separate hosts
- Microservices architectures across multiple machines

### Usage
```bash
# Setup app with external backend URL
hotify-cli setup \
  --id myapp \
  --name "My App" \
  --domain myapp \
  --port 3000 \
  --cmd "/usr/local/bin/myapp start" \
  --backend-url "http://100.114.4.57:8080"
```

When `--backend-url` is set:
- Traefik routes to the specified URL instead of `http://127.0.0.1:<port>`
- DNS and TLS certificate management still handled by hotify
- Basic auth and other Traefik middleware still apply
- The app can run on any reachable machine (local network, Tailscale, VPN)

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

### Remote Service Proxying Caveats

**Service Binding for Remote Access**
- **Issue**: Services bound to 127.0.0.1 not accessible via Tailscale/VPN
- **Solution**: Bind services to 0.0.0.0 for remote access
- **Verification**: Check listening address with `ss -tlnp | grep <port>`
- **Security**: Use authentication (passwords, basic auth) when binding to 0.0.0.0

**ACME Certificate Generation Timing**
- **Issue**: Domain returns SSL errors immediately after Traefik setup
- **Cause**: Let's Encrypt certificate generation takes 10-30 seconds
- **Solution**: Wait before testing domain access
- **Verification**: Check certificate in `/etc/traefik/acme.json`

**Local Flag for DNS/Traefik Setup**
- **Issue**: Remote daemon authentication failures (401 errors) prevent DNS/Traefik setup
- **Solution**: Use `--local` flag to execute commands directly on local machine
- **When to use**: Remote daemon has auth issues, faster local execution
- **Example**: `hotify-cli setup-dns --id <app> --ip <ip> --local`

**Domain Naming Patterns**
- **Pattern**: Include machine name in subdomain for clarity
- **Format**: `<service>.<machine>.<domain>`
- **Examples**: `hermes.rbm21.intrane.fr`, `cmdcenter.dk1.intrane.fr`
- **Benefits**: Clear service location identification, prevents domain conflicts

## Architecture

- **Local CLI**: All CRUD, authentication, deployment, and Docker commands
- **Daemon Mode** (`hotify-cli start -daemon`): HTTP server on port 8080 with embedded web UI + REST API
- **Remote Targets**: Manage multiple servers via API with encrypted tokens
- **Transport**: All commands use HTTP API (no SSH required) — hotify-cli is fully SSH-independent

## Benefits of HTTP API Over SSH

- **No SSH keys required**: Developers don't need SSH access to infrastructure
- **Audit logging**: All operations logged via hotify's audit system
- **Consistent interface**: Same commands work locally and remotely
- **No tunneling required**: Works through firewalls/NAT without SSH tunnels
- **Team collaboration**: Multiple developers can work via shared API tokens

## Important Note: Permission Enforcement (v2.7.4+)

**✅ Permission Enforcement Implemented**: As of v2.7.4, hotify-cli enforces permissions at the server level. The auth middleware validates tokens AND checks specific permissions for each endpoint.

**Current State**:
- Permissions are enforced for all authenticated endpoints
- Permission types: `deploy`, `start`, `stop`, `restart`, `logs`, `config`, `admin`
- Wildcard support: `all` or `*` grants full access
- Admin permission automatically grants all permissions
- 403 Forbidden responses for insufficient permissions

**Permission Mapping**:
- `/api/status` → requires `logs`
- `/api/config` → requires `config`
- `/api/apps/*/start` → requires `start`
- `/api/apps/*/stop` → requires `stop`
- `/api/deploy` → requires `deploy`
- `/api/api-keys/*` → requires `admin`
- And more... (see permissions.go for full mapping)

**Wildcard Usage**:
```bash
# Create full access key using wildcard
hotify-cli api-keys --action add --name fullaccess --permissions all

# Alternative wildcard syntax
hotify-cli api-keys --action add --name fullaccess --permissions "*"
```

**Recommendation for Agents**:
- Use appropriate permission scoping for security
- Create keys with minimum required permissions
- Use wildcards (`all`/`*`) only for trusted administrative access
- Monitor audit logs for permission denials

**Implementation Details**:
- Permission system defined in `permissions.go`
- Endpoint-to-permission mapping in `EndpointPermissions` array
- Global permission checking functions: `CheckTokenPermission`, `CheckTokenPermissions`
- Auth middleware in `server.go` enforces permissions per endpoint
- Wildcard expansion in both `ParsePermissions` and `AddKey` functions

## Version Information

Current version: v2.10.0

### v2.10.0 Features (Complete Remote/Targets Roadmap)
- **Deployment commands --local flag**: All deployment commands (`deploy`, `deploy-compose`, `compose-sync`, `compose-copy-dir`, `volume-init`, `setup-compose`) now accept `--local` to execute directly on current machine without remote target
- **Remote app config management**: `setup`, `add`, `edit`, `remove`, `list` all support `--target` for remote execution via HTTP API
  - New server endpoints: `POST /api/remote/apps/{id}/config-setup`, `GET|DELETE /api/remote/apps/{id}/config`
  - Client methods: `SetupAppConfig`, `GetAppConfig`, `RemoveAppConfig`, `ListAppsRemote`
- **Remote Docker management**: All `docker` subcommands (`list`, `start`, `stop`, `restart`, `status`, `logs`, `enable-traefik`, `disable-traefik`) support `--target` / `--local`
  - New server_docker_remote.go with full container CRUD + Traefik Docker provider toggle
  - Client methods: `DockerListRemote`, `DockerStartRemote`, `DockerStopRemote`, `DockerRestartRemote`, `DockerStatusRemote`, `DockerLogsRemote`, `DockerEnableTraefikRemote`, `DockerDisableTraefikRemote`
- **Remote prune support**: `prune --target <name>` sends prune operation to remote daemon, `--local` forces local execution
  - New server handler: `POST /api/remote/apps/{id}/prune`
- **Remote compose passthrough**: `compose --target <name>` routes docker compose commands to remote daemon, `--local` forces local execution
  - New server endpoint: `POST /api/remote/compose/exec` with subcommand allowlist and audit logging
  - Client method: `ComposeExecRemote`

### v2.9.0 Features
- **Backend URL support**: Added `--backend-url` parameter for proxying remote services
- **Local DNS/Traefik setup**: Enhanced `--local` flag support for DNS and Traefik commands
- **Remote service proxying**: Enable exposing services on remote machines via local Traefik
- **Use case**: Proxy services running on Tailscale/VPN-connected machines

### v2.8.2 Features
- **Bug fix**: Fixed APR1-MD5 hash generation for basic-auth
- Now uses OpenSSL command for correct APR1-MD5 hash generation
- Ensures compatibility with Traefik basic auth middleware
- Resolves hash mismatch issues reported by rbm2 agent

### v2.8.1 Features
- **External reverse proxy support**: Apps can now run on external machines while hotify handles DNS, TLS, and Traefik routing
- **--backend-url parameter**: Configure custom backend URLs for apps (e.g., http://100.114.4.57:8080)
- **Web UI support**: Backend URL field added to add/edit app forms
- **Traefik integration**: Automatic routing to custom backend URLs when configured

### v2.7.4 Features
- Implemented server-side permission enforcement for all API endpoints
- Added wildcard support (`all`/`*`) for full access keys
- Comprehensive endpoint-to-permission mapping for all API routes
- Permission denials logged in audit system for security monitoring
- Fine-grained access control for multi-team deployments

### v2.7.3 Features
- Remote execution for `basic-auth`, `setup-traefik`, `setup-dns` commands
- `--target` and `--local` flags for app configuration commands
- New remote API endpoints for app-specific operations
- Enhanced "no-humans mindset" documentation

## Common Workflows

### Quick App Setup
```bash
# 1. Register app
hotify-cli setup --id myapp --name "My App" --domain myapp --port 3000 --cmd "/usr/local/bin/myapp start"

# 2. Setup DNS (remote)
hotify-cli setup-dns --id myapp

# 3. Setup Traefik (remote)
hotify-cli setup-traefik --id myapp

# 4. Deploy and start (remote)
hotify-cli deploy --id myapp --source ./myapp
hotify-cli start --id myapp
```

### Docker Compose Deployment
```bash
# 1. Register app with compose config
hotify-cli setup --id cmdcenter --name "Command Center" --domain cmdcenter --port 3031 \
  --cmd "docker compose up -d" --compose-file compose.binary.yml --compose-path /home/dk1/cmdcenter

# 2. Deploy project files (remote)
hotify-cli deploy-compose --id cmdcenter --source /local/path/to/project

# 3. Start compose stack (remote)
hotify-cli compose --id cmdcenter up -d
```

## File Structure

- `main.go` - CLI entry point
- `config.go` - Configuration management
- `server.go` - HTTP server and API endpoints
- `deploy_client.go` - HTTP client for remote operations
- `basic_auth.go` - Basic auth management
- `dns_traefik_cli.go` - DNS and Traefik CLI commands
- `server_apps_remote.go` - Remote API endpoints for app operations
- `AGENTS.md` - Comprehensive agent documentation

## Testing

Smoke tests are performed on dk1@92.113.145.178. All remote execution commands should be tested with both `--target` and `--local` flags.

## Important Notes

- All remote operations require a configured target with valid authentication
- Use `--local` flag only when executing on the same machine as the hotify daemon
- Default output is JSON (machine-readable). Add `--human` for human-readable text
- Bootstrap tokens are generated when daemon starts with no API keys
- API keys are stored in memory only and reset on daemon restart