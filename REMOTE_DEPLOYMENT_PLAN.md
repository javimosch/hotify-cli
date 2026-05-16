# Hotify CLI Remote Deployment Plan

## Current State (v1.0.0)
- Local configuration management
- CLI commands for app CRUD
- Web UI for app management
- Cloudflare DNS integration
- Traefik configuration management
- Requires manual rsync + SSH for deployment

## Proposed Improvement (v2.0.0)

### Goal
Transform hotify-cli into a complete remote deployment solution that handles:
- Binary deployment via rsync
- Remote daemon communication
- End-to-end deployment automation
- Remove need for manual SSH/rsync

### Architecture

#### Local Hotify CLI (Deployment Machine)
- App configuration management
- Binary building/compilation
- Rsync deployment to remote server
- Remote daemon communication (authenticated)
- Deployment status monitoring
- API token management

#### Remote Hotify Daemon (Target Server - dk1)
- App configuration sync
- Binary management
- Process management (start/stop/restart)
- DNS/Traefik setup
- Health monitoring
- API endpoints for remote control
- **API key authentication system**
- **Token-based access control**

### New Features

#### 0. Authentication & Security (NEW - CRITICAL)

**Local CLI Commands**:
```bash
# Authenticate with remote daemon
hotify-cli auth \
  --url http://dk1:3060 \
  --token xxx \
  --name dk1

# List authenticated remotes
hotify-cli auth list

# Remove authentication
hotify-cli auth remove --name dk1

# Test connection
hotify-cli auth test --name dk1
```

**Remote Daemon Commands**:
```bash
# Add API key (on remote server)
hotify-cli api-keys add \
  --name "p22" \
  --token xxx \
  --permissions "deploy,start,stop"

# List API keys
hotify-cli api-keys list

# Remove API key
hotify-cli api-keys remove --name "p22"

# Regenerate API key
hotify-cli api-keys regenerate --name "p22"

# Set key permissions
hotify-cli api-keys permissions \
  --name "p22" \
  --add "deploy,logs" \
  --remove "restart"
```

**Authentication Flow**:
1. Admin generates API key on remote daemon
2. Local CLI authenticates with remote daemon using token
3. Token stored securely in local config
4. All remote API calls include Bearer token
5. Token validation on every request
6. Automatic token refresh support

**Security Features**:
- Token-based authentication (JWT or opaque tokens)
- Permission system (deploy, start, stop, logs, config)
- Token expiration and refresh
- IP whitelisting support
- Rate limiting per API key
- Audit logging for all API calls
- Secure token storage (encrypted at rest)

#### 1. Remote Configuration
```bash
# Add remote server
hotify-cli remote add \
  --name dk1 \
  --host 92.113.145.178 \
  --user dk1 \
  --ssh-key ~/.ssh/id_rsa_srv

# List remotes
hotify-cli remote list

# Set default remote
hotify-cli remote set-default dk1
```

#### 2. App Deployment
```bash
# Deploy app (build + rsync + setup)
hotify-cli deploy \
  --id myapp \
  --source ./myapp \
  --remote dk1 \
  --port 3000 \
  --command "./myapp start"

# Quick deploy (uses config)
hotify-cli deploy --id myapp
```

#### 3. Remote Process Management
```bash
# Start remote app
hotify-cli remote start --id myapp --remote dk1

# Stop remote app
hotify-cli remote stop --id myapp --remote dk1

# Restart remote app
hotify-cli remote restart --id myapp --remote dk1

# Check remote status
hotify-cli remote status --id myapp --remote dk1
```

#### 4. Binary Management
```bash
# Build and deploy
hotify-cli build --id myapp --go
hotify-cli deploy --id myapp

# Deploy existing binary
hotify-cli deploy --id myapp --binary ./myapp

# Deploy from source
hotify-cli deploy --id myapp --source github.com/user/repo
```

#### 5. Remote Daemon API
New endpoints on remote daemon:
- `POST /api/deploy` - Deploy app binary
- `POST /api/apps/start` - Start app process
- `POST /api/apps/stop` - Stop app process
- `POST /api/apps/restart` - Restart app process
- `GET /api/apps/{id}/status` - Get app status
- `GET /api/apps/{id}/logs` - Get app logs
- `POST /api/apps/{id}/health` - Health check

### Configuration Structure

#### Enhanced Config
```json
{
  "cloudflare_token": "token",
  "domain": "intrane.fr",
  "admin_email": "admin@example.com",
  "remotes": [
    {
      "name": "dk1",
      "url": "http://92.113.145.178:3060",
      "auth_token": "encrypted_token_here",
      "permissions": ["deploy", "start", "stop", "logs"],
      "default": true,
      "last_used": "2026-05-16T20:00:00Z"
    }
  ],
  "apps": [
    {
      "id": "myapp",
      "name": "My App",
      "domain": "myapp.intrane.fr",
      "port": 3000,
      "command": "/home/dk1/apps/myapp/myapp start",
      "source": "./myapp",
      "build_type": "go",
      "remote_path": "/home/dk1/apps/myapp",
      "status": "running",
      "remote": "dk1"
    }
  ]
}
```

### Implementation Plan

#### Phase 0: Authentication & Security (CRITICAL - DO FIRST)
1. **Authentication System**
   - Add `auth` command to local CLI
   - Add `api-keys` command to remote daemon
   - Implement JWT/opaque token generation
   - Token validation middleware
   - Permission system implementation

2. **Security Infrastructure**
   - Token storage encryption (AES-256)
   - Token expiration handling
   - Permission-based access control
   - Rate limiting middleware
   - Audit logging system
   - IP whitelisting support

3. **API Key Management**
   - CRUD operations for API keys
   - Permission assignment per key
   - Key rotation/revocation
   - Key usage tracking
   - Admin key management

4. **Secure Communication**
   - HTTPS/TLS support for daemon
   - Bearer token authentication
   - Request signing (optional)
   - CORS configuration
   - Security headers

#### Phase 1: Remote Foundation
1. **Remote Management Commands**
   - Add `remote add/list/remove/set-default` commands
   - SSH connection handling
   - Remote daemon detection

2. **Remote Daemon Enhancement**
   - Add deployment API endpoints
   - Add process management API endpoints
   - Add health check endpoints
   - Add log streaming endpoints

3. **Configuration Update**
   - Add remotes section to config
   - Add remote-specific app fields
   - Migration script for v1.0.0 configs

#### Phase 2: Rsync Integration
1. **Rsync Commands**
   - Add `rsync` command for manual file sync
   - Add `deploy` command for automated deployment
   - Add binary building integration

2. **File Management**
   - Remote directory creation
   - Binary deployment
   - Configuration file sync
   - Rollback support

3. **Build Integration**
   - Go build support
   - Binary optimization
   - Cross-compilation support
   - Build caching

#### Phase 3: Process Management
1. **Remote Process Control**
   - Start/stop/restart commands
   - PID file management
   - Process monitoring
   - Auto-restart on failure

2. **Health Monitoring**
   - HTTP health checks
   - Process status monitoring
   - Resource usage tracking
   - Alert integration

3. **Log Management**
   - Remote log streaming
   - Log aggregation
   - Log rotation
   - Error tracking

#### Phase 4: End-to-End Deployment
1. **Deployment Pipeline**
   - Build → Rsync → Start → Health Check → DNS → Traefik
   - Atomic deployments
   - Zero-downtime deployments
   - Rollback capability

2. **Web UI Enhancement**
   - Remote server selection
   - Deployment status dashboard
   - Real-time log streaming
   - Process control interface

3. **CLI Enhancement**
   - One-command deployment
   - Deployment history
   - Rollback commands
   - Status monitoring

### Technical Details

#### Authentication System
```go
type AuthClient struct {
    BaseURL    string
    AuthToken  string
    HTTPClient *http.Client
}

type APIKey struct {
    Name        string
    Token       string
    Permissions []string
    CreatedAt   time.Time
    ExpiresAt   time.Time
    LastUsed    time.Time
}

func (a *AuthClient) Authenticate(url, token string) error
func (a *AuthClient) ValidateToken() (bool, error)
func (a *AuthClient) RefreshToken() error
func (a *AuthClient) HasPermission(perm string) bool

type SecurityManager struct {
    EncryptionKey []byte
}

func (s *SecurityManager) EncryptToken(token string) (string, error)
func (s *SecurityManager) DecryptToken(encrypted string) (string, error)
func (s *SecurityManager) GenerateToken() (string, error)
func (s *SecurityManager) ValidateToken(token string) (*APIKey, error)
```

#### Permission System
```go
type Permission string

const (
    PermissionDeploy Permission = "deploy"
    PermissionStart  Permission = "start"
    PermissionStop   Permission = "stop"
    PermissionRestart Permission = "restart"
    PermissionLogs   Permission = "logs"
    PermissionConfig Permission = "config"
    PermissionAdmin  Permission = "admin"
)

type PermissionManager struct {
    APIKeys map[string]*APIKey
}

func (p *PermissionManager) CheckPermission(token, permission string) bool
func (p *PermissionManager) GrantPermission(keyName, permission string) error
func (p *PermissionManager) RevokePermission(keyName, permission string) error
```

#### SSH Communication
```go
type SSHClient struct {
    Host     string
    User     string
    KeyPath  string
    Client   *ssh.Client
}

func (c *SSHClient) Connect() error
func (c *SSHClient) Execute(cmd string) (string, error)
func (c *SSHClient) Rsync(local, remote string) error
func (c *SSHClient) Close() error
```

#### Remote Daemon Client
```go
type RemoteDaemon struct {
    BaseURL string
    Client  *http.Client
}

func (r *RemoteDaemon) Deploy(appID, binaryData string) error
func (r *RemoteDaemon) StartApp(appID string) error
func (r *RemoteDaemon) StopApp(appID string) error
func (r *RemoteDaemon) GetStatus(appID string) (*AppStatus, error)
```

#### Process Manager
```go
type ProcessManager struct {
    PidDir string
    LogDir string
}

func (p *ProcessManager) Start(appID, command string) error
func (p *ProcessManager) Stop(appID string) error
func (p *ProcessManager) Restart(appID string) error
func (p *ProcessManager) Status(appID string) (*ProcessStatus, error)
```

### File Structure Changes

#### New Files
- `auth.go` - Authentication client (local CLI)
- `api_keys.go` - API key management (remote daemon)
- `security.go` - Security utilities (token encryption, validation)
- `permissions.go` - Permission system
- `remote.go` - Remote server management
- `ssh.go` - SSH client implementation
- `deploy.go` - Deployment logic
- `build.go` - Build integration
- `process.go` - Process management
- `remote_daemon.go` - Remote daemon enhancements
- `health.go` - Health monitoring
- `audit.go` - Audit logging system

#### Modified Files
- `config.go` - Add remotes and deployment fields
- `main.go` - Add new commands
- `server.go` - Add deployment API endpoints

### API Endpoints

#### Remote Daemon (New - Authentication)
```
POST   /api/auth/login          Authenticate with token
POST   /api/auth/refresh       Refresh authentication token
GET    /api/auth/validate      Validate current token
POST   /api/auth/logout        Invalidate token

POST   /api/api-keys           Create new API key
GET    /api/api-keys           List all API keys
DELETE /api/api-keys/{name}    Delete API key
POST   /api/api-keys/{name}/regenerate Regenerate API key
PUT    /api/api-keys/{name}/permissions Update permissions
GET    /api/api-keys/{name}    Get API key details
```

#### Remote Daemon (New - App Management)
```
POST   /api/deploy              Deploy app binary
POST   /api/apps/{id}/start    Start app
POST   /api/apps/{id}/stop     Stop app
POST   /api/apps/{id}/restart Restart app
GET    /api/apps/{id}/status   Get app status
GET    /api/apps/{id}/logs     Get app logs
GET    /api/apps/{id}/health   Health check
POST   /api/apps/{id}/rollback Rollback deployment
GET    /api/deployments        Deployment history
```

### Security Considerations

#### Authentication & Authorization (CRITICAL)
1. **Token Security**
   - Tokens stored encrypted at rest (AES-256)
   - Token expiration (configurable, default 30 days)
   - Token rotation support
   - Secure token generation (crypto/rand)
   - No token logging in audit logs

2. **Permission System**
   - Granular permissions (deploy, start, stop, logs, config, admin)
   - Permission inheritance (admin includes all)
   - Per-token permission assignment
   - Permission validation on every API call
   - Audit trail for permission changes

3. **Communication Security**
   - HTTPS/TLS for all daemon communication
   - Bearer token authentication header
   - CORS properly configured
   - Security headers (CSP, X-Frame-Options, etc.)
   - Request rate limiting per token
   - IP whitelisting support (optional)

4. **API Key Management**
   - Admin-only API key creation
   - Key usage tracking (last used, request count)
   - Key revocation support
   - Key expiration warnings
   - Audit logging for all key operations

#### Data Protection
1. **Configuration Security**
   - Config file permissions (0600)
   - Sensitive data encryption
   - No plaintext tokens in config
   - Environment variable support
   - Secure credential storage

2. **File System Security**
   - Proper permissions on deployed binaries (0755)
   - App isolation (separate directories per app)
   - Log file permissions (0640)
   - PID file security (0600)
   - Temporary file cleanup

3. **Network Security**
   - Daemon bind to specific interface (127.0.0.1 or VPN)
   - Firewall rules for daemon port
   - VPN-only access (optional)
   - SSH key-based authentication for rsync
   - No plaintext credentials in URLs

#### Operational Security
1. **Audit Logging**
   - All API calls logged with timestamp
   - Token usage tracking
   - Permission changes logged
   - Failed authentication attempts logged
   - Configuration changes logged

2. **Monitoring & Alerting**
   - Failed authentication alerts
   - Unusual usage patterns detection
   - Token expiration warnings
   - Rate limiting breach alerts
   - Permission escalation alerts

3. **Backup & Recovery**
   - Encrypted config backups
   - API key backup mechanism
   - Disaster recovery procedures
   - Key recovery process (admin only)
   - Audit log backup and retention

#### Compliance & Best Practices
1. **Security Best Practices**
   - Regular security audits
   - Dependency vulnerability scanning
   - Secure coding practices
   - Regular token rotation policy
   - Principle of least privilege

2. **Compliance Considerations**
   - GDPR compliance (audit logs, data retention)
   - SOC 2 considerations (access controls, logging)
   - Industry best practices alignment
   - Security documentation
   - Incident response procedures

#### Threat Model
1. **Mitigated Threats**
   - Token theft (encryption, expiration, rotation)
   - Unauthorized access (permissions, validation)
   - Replay attacks (timestamp, nonce)
   - Man-in-the-middle (TLS)
   - Privilege escalation (permission system)

2. **Remaining Risks**
   - Compromised local machine (config access)
   - Social engineering (token sharing)
   - Zero-day vulnerabilities (regular updates)
   - Insider threats (audit logging, monitoring)

#### Security Configuration
```json
{
  "security": {
    "token_expiration_days": 30,
    "max_failed_attempts": 5,
    "rate_limit_per_minute": 60,
    "require_https": true,
    "allowed_ips": [],
    "audit_log_retention_days": 90,
    "encryption_algorithm": "AES-256-GCM",
    "token_length": 32
  }
}
```

### Authentication Workflow Example

#### Initial Setup (One-Time)

**On Remote Server (dk1)**:
```bash
# Start hotify daemon
hotify-cli start --daemon --port 3060

# Generate API key for local machine
hotify-cli api-keys add \
  --name "local-machine" \
  --permissions "deploy,start,stop,logs" \
  --token auto
# Output: Generated token: abc123xyz789...
```

**On Local Machine**:
```bash
# Authenticate with remote daemon
hotify-cli auth \
  --url http://92.113.145.178:3060 \
  --token abc123xyz789... \
  --name dk1

# Verify connection
hotify-cli auth test --name dk1
# Output: ✓ Connection successful to dk1
# Permissions: deploy, start, stop, logs
```

#### Daily Deployment Workflow

**On Local Machine**:
```bash
# Deploy app (authentication handled automatically)
hotify-cli deploy \
  --from-folder ~/ai/myapp \
  --cmd "./myapp daemon start --port 3031" \
  --port 3031 \
  --remote dk1

# Output: ✓ Authenticated as dk1
# ✓ Building binary...
# ✓ Deploying to dk1...
# ✓ Starting app on dk1...
# ✓ App running at https://myapp.intrane.fr
```

#### API Key Management

**On Remote Server**:
```bash
# List all API keys
hotify-cli api-keys list
# Output:
# local-machine  deploy,start,stop,logs  2026-05-16  active
# p22           admin                    2026-05-15  active

# Revoke compromised key
hotify-cli api-keys remove --name "local-machine"

# Generate new key with different permissions
hotify-cli api-keys add \
  --name "local-machine-v2" \
  --permissions "deploy,logs" \
  --token auto

# Update permissions on existing key
hotify-cli api-keys permissions \
  --name "p22" \
  --remove "admin" \
  --add "deploy,logs"
```

**On Local Machine**:
```bash
# Update authentication after key rotation
hotify-cli auth \
  --url http://92.113.145.178:3060 \
  --token newtokenxyz789... \
  --name dk1

# Remove old authentication
hotify-cli auth remove --name dk1-old
```

#### Security Monitoring

**On Remote Server**:
```bash
# Check recent authentication attempts
hotify-cli audit recent --type auth
# Output:
# 2026-05-16 20:15:23  local-machine  SUCCESS  92.113.145.178
# 2026-05-16 20:10:11  unknown       FAILED   192.168.1.100
# 2026-05-16 20:05:45  p22           SUCCESS  92.113.145.178

# Check API key usage
hotify-cli api-keys usage --name "local-machine"
# Output:
# Last used: 2026-05-16 20:15:23
# Request count (24h): 127
# Failed attempts: 0
```

### Migration Path

#### v1.0.0 → v2.0.0
1. Backup existing configuration
2. Run migration script
3. Add remote servers
4. Update app configs with remote info
5. Test deployment workflow

### Testing Strategy

1. **Unit Tests**
   - SSH client tests
   - Rsync tests
   - Process manager tests
   - Remote daemon client tests

2. **Integration Tests**
   - End-to-end deployment
   - Remote communication
   - Rollback scenarios
   - Error handling

3. **Manual Testing**
   - Local deployment
   - Remote deployment
   - Multi-server deployment
   - Error recovery

### Documentation Updates

1. **README.md**
   - Remote deployment guide
   - SSH setup instructions
   - Migration guide
   - Troubleshooting

2. **AGENTS.md**
   - Agent deployment workflow
   - Remote command usage
   - Error handling patterns

3. **New Documentation**
   - REMOTE_DEPLOYMENT.md
   - SSH_SETUP.md
   - MIGRATION_GUIDE.md

### Rollout Plan

1. **Security Alpha** (v2.0.0-security-alpha)
   - Authentication system
   - API key management
   - Token encryption
   - Basic permission system
   - Security audit logging

2. **Remote Alpha** (v2.0.0-alpha)
   - Basic remote commands
   - SSH integration
   - Simple deployment
   - Authentication integration

3. **Beta Release** (v2.0.0-beta)
   - Process management
   - Health monitoring
   - Web UI updates
   - Advanced permissions

4. **Stable Release** (v2.0.0)
   - Full feature set
   - Complete documentation
   - Migration tools
   - Security audit complete

### Success Criteria

✅ Deploy app with single command
✅ No manual SSH/rsync required
✅ Remote process management
✅ Health monitoring
✅ Rollback capability
✅ Multi-server support
✅ Web UI integration
✅ Backward compatibility
✅ **Secure authentication between CLI and daemon**
✅ **Granular permission system**
✅ **API key management**
✅ **Audit logging for security events**
✅ **Token encryption at rest**

### Risks & Mitigation

1. **Authentication Security Risks**
   - Risk: Token theft or compromise
   - Mitigation: Encryption at rest, short expiration, rotation support
   - Risk: Permission bypass
   - Mitigation: Validation on every request, audit logging
   - Risk: Replay attacks
   - Mitigation: Timestamp validation, nonce support

2. **SSH Authentication Issues**
   - Risk: Key-based auth failures
   - Mitigation: Multiple auth methods, clear error messages

3. **Network Connectivity**
   - Risk: Connection failures during deployment
   - Mitigation: Retry logic, atomic deployments

4. **Process Management Complexity**
   - Risk: Process state inconsistencies
   - Mitigation: Robust PID management, health checks

5. **Backward Compatibility**
   - Risk: Breaking existing workflows
   - Mitigation: Migration scripts, compatibility mode

6. **Key Management Risks**
   - Risk: Lost admin keys
   - Mitigation: Backup mechanism, recovery process
   - Risk: Over-privileged keys
   - Mitigation: Principle of least privilege, regular audits

### Future Enhancements

1. **Multi-Cloud Support**
   - AWS, DigitalOcean, etc.
   - Cloud provider APIs

2. **Container Support**
   - Docker deployment
   - Kubernetes integration

3. **CI/CD Integration**
   - GitHub Actions
   - GitLab CI
   - Jenkins

4. **Advanced Monitoring**
   - Metrics collection
   - Alerting
   - Dashboard integration

## Conclusion

This enhancement will transform hotify-cli from a configuration management tool into a complete remote deployment solution, significantly simplifying the deployment workflow while maintaining security and reliability.
