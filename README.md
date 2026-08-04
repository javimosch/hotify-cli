# Hotify CLI

Traefik/Cloudflare app management CLI for quick deployment of web apps with automatic SSL and DNS setup.

## Overview

Hotify is an opinionated CLI+UI tool that simplifies deploying web apps behind Traefik with Cloudflare DNS and Let's Encrypt SSL certificates. It assumes apps follow the boilerplate-cli-ui-go pattern: a binary with daemon start/stop that exposes an HTTP server on a port.

## Features

- **CLI Commands**: Add, edit, remove, and list apps
- **Web UI**: Browser-based app management
- **DNS Setup**: Automatic Cloudflare DNS record creation
- **SSL Certificates**: Automatic Let's Encrypt certificates via Traefik
- **Traefik Integration**: Automatic Traefik configuration updates
- **Daemon Mode**: Background service for persistent management

## Installation

### Quick Install (Linux)

```bash
curl -fsSL https://github.com/javimosch/hotify-cli/releases/latest/download/hotify-cli-linux-amd64 -o /tmp/hotify-cli
sudo install -m 755 /tmp/hotify-cli /usr/local/bin/hotify-cli
rm /tmp/hotify-cli
```

### From Source

```bash
cd /home/jarancibia/ai/hotify-cli
chmod +x build.sh
./build.sh
sudo cp hotify-cli /usr/local/bin/
```

### Usage

```bash
hotify-cli --help
```

## Configuration

### Initialize Configuration

```bash
hotify-cli init
```

You'll be prompted for:
- Cloudflare API token (legacy format recommended)
- Base domain (e.g., example.com)
- Admin email for Let's Encrypt

## CLI Commands

### Add an App

```bash
hotify-cli add \
  --id myapp \
  --name "My App" \
  --domain myapp \
  --port 3000 \
  --command "/path/to/myapp start" \
  --source "github.com/user/repo"
```

### Edit an App

```bash
hotify-cli edit --id myapp --name "Updated Name"
```

### Remove an App

```bash
hotify-cli remove --id myapp
```

### List Apps

```bash
hotify-cli list
```

### Start Web UI

```bash
# Foreground
hotify-cli start --port 8080

# Background (daemon)
hotify-cli start --daemon
```

### Check Daemon Status

```bash
hotify-cli status
```

### Stop Daemon

```bash
hotify-cli stop
```

## Web UI

Start the daemon and access the web UI:

```bash
hotify-cli start --daemon
# Access at http://localhost:8080
```

The web UI provides:
- Configuration overview
- App management (add/edit/remove)
- One-click DNS setup
- One-click Traefik configuration
- Real-time status updates

## DNS Setup

After adding an app, set up the DNS record:

```bash
hotify-cli setup-dns --id myapp --ip 92.113.145.178
```

This creates an A record in Cloudflare pointing to your server IP.

## Traefik Setup

Configure Traefik for your app:

```bash
hotify-cli setup-traefik --id myapp
```

This:
- Creates/updates `/etc/traefik/traefik.yml`
- Creates/updates `/etc/traefik/dynamic.yml`
- Sets up Cloudflare environment variables
- Configures systemd service
- Restarts Traefik

## App Requirements

Apps must follow this pattern:

1. **Binary**: Single executable file
2. **Daemon Mode**: Support `start`/`stop` commands
3. **HTTP Server**: Expose HTTP on a specific port
4. **No SSL**: Let Traefik handle SSL/TLS

Example app structure:
```bash
myapp start --port 3000  # Starts HTTP server on port 3000
myapp stop               # Stops the daemon
```

## Configuration File

Configuration is stored at `~/.hotify/config.json`:

```json
{
  "cloudflare_token": "your_token",
  "domain": "example.com",
  "admin_email": "admin@example.com",
  "apps": [
    {
      "id": "myapp",
      "name": "My App",
      "domain": "myapp.example.com",
      "port": 3000,
      "command": "/path/to/myapp start",
      "source": "github.com/user/repo",
      "status": "stopped"
    }
  ]
}
```

### External Backend (Remote Apps)

For apps running on a different machine behind the reverse proxy (e.g. via Tailscale),
set `backend_url` to the remote machine's address:

```json
{
  "id": "hermes-webui",
  "name": "Hermes WebUI",
  "domain": "hermes.rbm21.intrane.fr",
  "port": 8787,
  "command": "true",
  "backend_url": "http://100.123.0.125:8787"
}
```

When set, Traefik routes to this URL instead of the default `http://127.0.0.1:<port>`.

### Proxy Services

For external services that only need Traefik reverse-proxy routing (no hotify process management), register the app with `--backend-url` and then run `setup-traefik`:

```bash
# Example: sl-cli static site running on port 3100 with a path prefix
hotify-cli setup \
  --id slv2 \
  --name "SL v2" \
  --domain slv2 \
  --port 3100 \
  --cmd "/usr/local/bin/sl-cli serve" \
  --backend-url "http://127.0.0.1:3100" \
  --path-prefix "/slv2"

hotify-cli setup-dns --id slv2
hotify-cli setup-traefik --id slv2
```

This generates both the router and the service in `/etc/traefik/dynamic.yml`, including the `addPrefix` middleware if `--path-prefix` is set.

**⚠️ Critical**: hotify-cli's `buildDynamicYAML()` regenerates the **entire** `/etc/traefik/dynamic.yml`
when `setup-traefik` is called for **any single app**. If an app lacks `backend_url`,
it falls back to `127.0.0.1:<port>`. This means remote apps lose their correct backend
URL whenever Traefik config is regenerated for any app.

**Two layers of protection against this regression:**

1. **Always set `backend_url`** for remote apps in `config.json` (prevents the fallback)
2. **Since v2.10.1**: `buildDynamicYAML` preserves existing backend URLs from the current
   `dynamic.yml` as a fallback via `readExistingBackendURLs()`, so even if `backend_url`
   is missing from config, the previously working URL is kept.

## Architecture

### File Structure
- `main.go` - CLI command handling
- `config.go` - Configuration management
- `daemon.go` - Daemon process management
- `server.go` - HTTP server and web UI
- `cloudflare.go` - Cloudflare API integration
- `traefik.go` - Traefik configuration management (contains `buildDynamicYAML` & `readExistingBackendURLs`)

### Traefik Integration

Hotify manages:
- `/etc/traefik/traefik.yml` - Main Traefik configuration
- `/etc/traefik/dynamic.yml` - Dynamic routing configuration
- `/etc/traefik/cloudflare.env` - Cloudflare credentials
- `/etc/traefik/acme.json` - SSL certificate storage
- `/etc/systemd/system/traefik.service` - Systemd service

### DNS Integration

Hotify uses Cloudflare API to:
- Get zone ID for domain
- Create A records for app subdomains
- Configure DNS-only mode (proxy disabled)

## API Endpoints

When the web UI is running, these API endpoints are available:

- `GET /` - Web UI
- `GET /api/status` - Server status
- `GET /api/health` - Health check
- `GET /api/config` - Current configuration
- `GET /api/apps` - List all apps
- `POST /api/apps/add` - Add new app
- `POST /api/apps/edit` - Edit existing app
- `POST /api/apps/remove` - Remove app
- `POST /api/apps/setup-dns` - Setup DNS for app
- `POST /api/apps/setup-traefik` - Setup Traefik for app

## Workflow Example

### Deploy a New App

1. **Initialize hotify** (first time only):
   ```bash
   hotify-cli init
   ```

2. **Add the app**:
   ```bash
   hotify-cli add \
     --id myapp \
     --name "My App" \
     --domain myapp \
     --port 3000 \
     --command "/home/user/myapp start" \
     --source "github.com/user/myapp"
   ```

3. **Deploy the app binary** to the server

4. **Setup DNS**:
   ```bash
   hotify-cli setup-dns --id myapp --ip 92.113.145.178
   ```

5. **Setup Traefik**:
   ```bash
   hotify-cli setup-traefik --id myapp
   ```

6. **Access the app** at https://myapp.example.com

### Using Web UI

1. Start the daemon:
   ```bash
   hotify-cli start --daemon
   ```

2. Open http://localhost:8080

3. Use the web interface to:
   - View configuration
   - Add/edit/remove apps
   - Setup DNS with one click
   - Setup Traefik with one click

## Requirements

- Go 1.21+ (for building)
- Linux server with systemd
- Cloudflare account with domain
- Traefik installed on server
- Server with public IP

## Troubleshooting

### Daemon Issues

```bash
# Check status
hotify-cli status

# View logs
cat /tmp/hotify-cli.log

# Restart daemon
hotify-cli stop
hotify-cli start --daemon
```

### Traefik Issues

```bash
# Check Traefik status
sudo systemctl status traefik

# View Traefik logs
sudo journalctl -u traefik -f

# Restart Traefik
sudo systemctl restart traefik
```

### DNS Issues

- Verify Cloudflare token has DNS edit permissions
- Check that domain is managed in Cloudflare
- Ensure server IP is correct
- Check Cloudflare dashboard for DNS records

### SSL Issues

- Verify admin email is correct
- Check Traefik logs for ACME errors
- Ensure DNS record is propagated
- Check Let's Encrypt rate limits

## Security Considerations

- Cloudflare token is stored in plaintext config file
- Set appropriate file permissions on `~/.hotify/config.json`
- Use API tokens with minimal required permissions
- Consider using environment variables for sensitive data
- Regularly rotate Cloudflare tokens

## Limitations

- Assumes specific app architecture (binary + HTTP server)
- Requires Traefik to be installed separately
- Only supports Cloudflare DNS provider
- No built-in app deployment (binary management)
- No automatic app process management
- Single server deployment only

## Future Enhancements

- Support for multiple DNS providers
- Built-in app deployment (git pull, build, install)
- App process management (start/stop/restart)
- Multi-server support
- SSL certificate monitoring
- Health check integration
- Rollback capabilities
- Configuration backup/restore

## License

MIT

## Contributing

Contributions welcome! Please read the contributing guidelines first.

## Support

For issues and questions:
- GitHub Issues: https://github.com/javimosch/hotify-cli/issues
- Documentation: https://github.com/javimosch/hotify-cli
