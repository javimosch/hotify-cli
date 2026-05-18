package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	server        *http.Server
	mu            sync.Mutex
	apiKeyManager *APIKeyManager
	auditLogger   *AuditLogger
)

type ServerStatus struct {
	Status    string    `json:"status"`
	Port      int       `json:"port"`
	Uptime    string    `json:"uptime"`
	StartTime time.Time `json:"start_time"`
	Config    *Config   `json:"config"`
}

var serverStatus ServerStatus

func startServer(port int) {
	// Initialize security managers
	var err error
	apiKeyManager, err = NewAPIKeyManager()
	if err != nil {
		log.Fatalf("Error creating API key manager: %v", err)
	}

	// Create initial admin key if no keys exist
	keys := apiKeyManager.ListKeys()
	if len(keys) == 0 {
		log.Println("No API keys found, creating initial admin key...")
		security, err := NewSecurityManager()
		if err != nil {
			log.Fatalf("Error creating security manager for bootstrap: %v", err)
		}
		initialToken, err := security.GenerateToken()
		if err != nil {
			log.Fatalf("Error generating bootstrap token: %v", err)
		}
		_, err = apiKeyManager.AddKey("admin-bootstrap", AllPermissions, initialToken)
		if err != nil {
			log.Printf("Warning: Failed to create initial admin key: %v", err)
		} else {
			log.Println("==============================================")
			log.Println("Initial admin key created successfully!")
			log.Printf("Token: %s", initialToken)
			log.Println("==============================================")
			log.Println("IMPORTANT: Store this token securely!")
			log.Println("Use it to authenticate: hotify-cli auth --url <url> --token <token> --name admin-bootstrap")
			log.Println("Then create additional API keys and remove this bootstrap key.")
		}
	}

	auditLogger, err = NewAuditLogger()
	if err != nil {
		log.Fatalf("Error creating audit logger: %v", err)
	}

	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	serverStatus = ServerStatus{
		Status:    "running",
		Port:      port,
		StartTime: time.Now(),
		Config:    config,
	}

	mux := http.NewServeMux()

	// Authentication endpoints (no auth required)
	mux.HandleFunc("/api/auth/login", handleAuthLogin)
	mux.HandleFunc("/api/auth/validate", handleAuthValidate)
	mux.HandleFunc("/api/auth/refresh", handleAuthRefresh)
	mux.HandleFunc("/api/auth/logout", handleAuthLogout)

	// API key endpoints (auth required)
	mux.HandleFunc("/api/api-keys", authMiddleware(handleAPIKeys))
	mux.HandleFunc("/api/api-keys/", authMiddleware(handleAPIKeyDetail))

	// Existing endpoints (auth required)
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/api/status", authMiddleware(handleStatusAPI))
	mux.HandleFunc("/api/health", handleHealthAPI)
	mux.HandleFunc("/api/config", authMiddleware(handleConfigAPI))
	mux.HandleFunc("/api/apps", authMiddleware(handleAppsAPI))
	mux.HandleFunc("/api/apps/add", authMiddleware(handleAddAppAPI))

	// Deployment endpoints (auth required)
	mux.HandleFunc("/api/deploy", authMiddleware(handleDeployAPI))
	mux.HandleFunc("/api/apps/", authMiddleware(handleAppManagementAPI))
	mux.HandleFunc("/api/apps/edit", authMiddleware(handleEditAppAPI))
	mux.HandleFunc("/api/apps/remove", authMiddleware(handleRemoveAppAPI))
	mux.HandleFunc("/api/apps/setup-dns", authMiddleware(handleSetupDNSAPI))
	mux.HandleFunc("/api/apps/setup-traefik", authMiddleware(handleSetupTraefikAPI))

	server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	log.Printf("Hotify server starting on http://localhost:%d", port)
	log.Printf("Press Ctrl+C to stop")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Hotify - Traefik/Cloudflare App Manager</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            padding: 40px;
        }
        .header {
            margin-bottom: 30px;
            border-bottom: 1px solid #e9ecef;
            padding-bottom: 20px;
        }
        .header h1 {
            color: #333;
            font-size: 32px;
            margin-bottom: 8px;
        }
        .header p {
            color: #666;
            font-size: 14px;
        }
        .section {
            margin-bottom: 30px;
        }
        .section h2 {
            color: #333;
            font-size: 20px;
            margin-bottom: 15px;
        }
        .config-info {
            background: #f8f9fa;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
        }
        .config-item {
            display: flex;
            justify-content: space-between;
            padding: 10px 0;
            border-bottom: 1px solid #e9ecef;
        }
        .config-item:last-child {
            border-bottom: none;
        }
        .label { color: #666; font-size: 14px; }
        .value { color: #333; font-weight: 600; font-size: 14px; }
        .apps-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 20px;
        }
        .app-card {
            background: #f8f9fa;
            border-radius: 8px;
            padding: 20px;
            border: 1px solid #e9ecef;
        }
        .app-card h3 {
            color: #333;
            font-size: 16px;
            margin-bottom: 10px;
        }
        .app-card .domain {
            color: #667eea;
            font-size: 14px;
            margin-bottom: 8px;
        }
        .app-card .status {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 600;
            margin-bottom: 10px;
        }
        .status-running { background: #d4edda; color: #155724; }
        .status-stopped { background: #f8d7da; color: #721c24; }
        .app-card .info {
            font-size: 12px;
            color: #666;
            margin-bottom: 5px;
        }
        .btn {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 10px 20px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 14px;
            font-weight: 600;
            transition: transform 0.2s;
        }
        .btn:hover { transform: translateY(-2px); }
        .btn-sm { padding: 6px 12px; font-size: 12px; }
        .btn-danger { background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%); }
        .btn-success { background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%); }
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.5);
            z-index: 1000;
        }
        .modal.active { display: flex; align-items: center; justify-content: center; }
        .modal-content {
            background: white;
            border-radius: 12px;
            padding: 30px;
            max-width: 500px;
            width: 90%;
        }
        .modal-content h3 {
            color: #333;
            margin-bottom: 20px;
        }
        .form-group { margin-bottom: 15px; }
        .form-group label {
            display: block;
            color: #333;
            font-size: 14px;
            margin-bottom: 5px;
        }
        .form-group input {
            width: 100%;
            padding: 10px;
            border: 1px solid #e9ecef;
            border-radius: 6px;
            font-size: 14px;
        }
        .modal-actions {
            display: flex;
            gap: 10px;
            margin-top: 20px;
        }
        .empty-state {
            text-align: center;
            padding: 40px;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔥 Hotify</h1>
            <p>Traefik/Cloudflare App Manager</p>
        </div>

        <div class="section">
            <h2>Configuration</h2>
            <div class="config-info" id="configInfo">
                Loading...
            </div>
        </div>

        <div class="section">
            <h2>Apps <button class="btn btn-sm" onclick="openAddModal()">+ Add App</button></h2>
            <div class="apps-grid" id="appsGrid">
                Loading...
            </div>
        </div>
    </div>

    <div class="modal" id="addModal">
        <div class="modal-content">
            <h3>Add New App</h3>
            <div class="form-group">
                <label>App ID</label>
                <input type="text" id="appId" placeholder="myapp">
            </div>
            <div class="form-group">
                <label>App Name</label>
                <input type="text" id="appName" placeholder="My App">
            </div>
            <div class="form-group">
                <label>Subdomain</label>
                <input type="text" id="appDomain" placeholder="myapp">
            </div>
            <div class="form-group">
                <label>Port</label>
                <input type="number" id="appPort" placeholder="3000">
            </div>
            <div class="form-group">
                <label>Command</label>
                <input type="text" id="appCommand" placeholder="/path/to/app start">
            </div>
            <div class="form-group">
                <label>Source (optional)</label>
                <input type="text" id="appSource" placeholder="github.com/user/repo">
            </div>
            <div class="modal-actions">
                <button class="btn" onclick="closeAddModal()">Cancel</button>
                <button class="btn btn-success" onclick="addApp()">Add App</button>
            </div>
        </div>
    </div>

    <script>
        function loadConfig() {
            fetch('/api/config')
                .then(r => r.json())
                .then(data => {
                    const configInfo = document.getElementById('configInfo');
                    configInfo.innerHTML = 
                        '<div class="config-item">' +
                        '<span class="label">Domain</span>' +
                        '<span class="value">' + (data.domain || 'Not set') + '</span>' +
                        '</div>' +
                        '<div class="config-item">' +
                        '<span class="label">Admin Email</span>' +
                        '<span class="value">' + (data.admin_email || 'Not set') + '</span>' +
                        '</div>' +
                        '<div class="config-item">' +
                        '<span class="label">Cloudflare Token</span>' +
                        '<span class="value">' + (data.cloudflare_token ? '••••••••' : 'Not set') + '</span>' +
                        '</div>';
                })
                .catch(err => console.error('Error loading config:', err));
        }

        function loadApps() {
            fetch('/api/apps')
                .then(r => r.json())
                .then(data => {
                    const appsGrid = document.getElementById('appsGrid');
                    if (data.apps.length === 0) {
                        appsGrid.innerHTML = '<div class="empty-state">No apps configured. Click "Add App" to get started.</div>';
                        return;
                    }
                    appsGrid.innerHTML = data.apps.map(app => 
                        '<div class="app-card">' +
                        '<h3>' + app.name + '</h3>' +
                        '<div class="domain">' + app.domain + '</div>' +
                        '<div class="status status-' + (app.status === 'running' ? 'running' : 'stopped') + '">' + app.status + '</div>' +
                        '<div class="info">Port: ' + app.port + '</div>' +
                        '<div class="info">Command: ' + app.command + '</div>' +
                        (app.source ? '<div class="info">Source: ' + app.source + '</div>' : '') +
                        '<div style="margin-top: 15px; display: flex; gap: 8px; flex-wrap: wrap;">' +
                        '<button class="btn btn-sm btn-success" onclick="setupDNS(\'' + app.id + '\')">Setup DNS</button>' +
                        '<button class="btn btn-sm btn-success" onclick="setupTraefik(\'' + app.id + '\')">Setup Traefik</button>' +
                        '<button class="btn btn-sm btn-danger" onclick="removeApp(\'' + app.id + '\')">Remove</button>' +
                        '</div>' +
                        '</div>'
                    ).join('');
                })
                .catch(err => console.error('Error loading apps:', err));
        }

        function openAddModal() {
            document.getElementById('addModal').classList.add('active');
        }

        function closeAddModal() {
            document.getElementById('addModal').classList.remove('active');
        }

        function addApp() {
            const app = {
                id: document.getElementById('appId').value,
                name: document.getElementById('appName').value,
                domain: document.getElementById('appDomain').value,
                port: parseInt(document.getElementById('appPort').value),
                command: document.getElementById('appCommand').value,
                source: document.getElementById('appSource').value
            };

            fetch('/api/apps/add', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(app)
            })
            .then(r => r.json())
            .then(data => {
                if (data.success) {
                    closeAddModal();
                    loadApps();
                } else {
                    alert('Error adding app: ' + data.error);
                }
            })
            .catch(err => console.error('Error adding app:', err));
        }

        function setupDNS(appId) {
            if (!confirm('Setup DNS for this app?')) return;
            fetch('/api/apps/setup-dns', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: appId, ip: '92.113.145.178' })
            })
            .then(r => r.json())
            .then(data => {
                if (data.success) {
                    alert('DNS setup successful!');
                } else {
                    alert('Error setting up DNS: ' + data.error);
                }
            })
            .catch(err => console.error('Error setting up DNS:', err));
        }

        function setupTraefik(appId) {
            if (!confirm('Setup Traefik for this app?')) return;
            fetch('/api/apps/setup-traefik', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: appId })
            })
            .then(r => r.json())
            .then(data => {
                if (data.success) {
                    alert('Traefik setup successful!');
                } else {
                    alert('Error setting up Traefik: ' + data.error);
                }
            })
            .catch(err => console.error('Error setting up Traefik:', err));
        }

        function removeApp(appId) {
            if (!confirm('Remove this app?')) return;
            fetch('/api/apps/remove', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: appId })
            })
            .then(r => r.json())
            .then(data => {
                if (data.success) {
                    loadApps();
                } else {
                    alert('Error removing app: ' + data.error);
                }
            })
            .catch(err => console.error('Error removing app:', err));
        }

        // Load initial data
        loadConfig();
        loadApps();

        // Auto-refresh every 30 seconds
        setInterval(() => {
            loadConfig();
            loadApps();
        }, 30000);
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	serverStatus.Uptime = time.Since(serverStatus.StartTime).String()

	// Refresh config
	config, err := loadConfig()
	if err == nil {
		serverStatus.Config = config
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(serverStatus)
}

func handleHealthAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func handleConfigAPI(w http.ResponseWriter, r *http.Request) {
	config, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func handleAppsAPI(w http.ResponseWriter, r *http.Request) {
	config, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"apps": config.Apps})
}

func handleAddAppAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var app struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Domain  string `json:"domain"`
		Port    int    `json:"port"`
		Command string `json:"command"`
		Source  string `json:"source"`
	}

	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	config, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if app ID already exists
	for _, existingApp := range config.Apps {
		if existingApp.ID == app.ID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "App ID already exists"})
			return
		}
	}

	// Create full domain
	fullDomain := fmt.Sprintf("%s.%s", app.Domain, config.Domain)

	// Add app to config
	newApp := App{
		ID:      app.ID,
		Name:    app.Name,
		Domain:  fullDomain,
		Port:    app.Port,
		Command: app.Command,
		Source:  app.Source,
		Status:  "stopped",
	}

	config.Apps = append(config.Apps, newApp)

	if err := saveConfig(config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func handleEditAppAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var app struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Domain  string `json:"domain"`
		Port    int    `json:"port"`
		Command string `json:"command"`
		Source  string `json:"source"`
	}

	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	config, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Find and update app
	found := false
	for i := range config.Apps {
		if config.Apps[i].ID == app.ID {
			found = true
			if app.Name != "" {
				config.Apps[i].Name = app.Name
			}
			if app.Domain != "" {
				config.Apps[i].Domain = fmt.Sprintf("%s.%s", app.Domain, config.Domain)
			}
			if app.Port != 0 {
				config.Apps[i].Port = app.Port
			}
			if app.Command != "" {
				config.Apps[i].Command = app.Command
			}
			if app.Source != "" {
				config.Apps[i].Source = app.Source
			}
			break
		}
	}

	if !found {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "App not found"})
		return
	}

	if err := saveConfig(config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func handleRemoveAppAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	config, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Find and remove app
	found := false
	var updatedApps []App
	for _, app := range config.Apps {
		if app.ID != payload.ID {
			updatedApps = append(updatedApps, app)
		} else {
			found = true
		}
	}

	if !found {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "App not found"})
		return
	}

	config.Apps = updatedApps

	if err := saveConfig(config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func handleSetupDNSAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ID string `json:"id"`
		IP string `json:"ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := setupDNSForApp(payload.ID, payload.IP); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func handleSetupTraefikAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ID            string `json:"id"`
		ChallengeType string `json:"challenge_type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ct := ChallengeHTTP
	if payload.ChallengeType == "dns" {
		ct = ChallengeDNS
	}

	if err := setupTraefikForAppWithChallenge(payload.ID, ct); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// Authentication Middleware
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate token
		key, err := apiKeyManager.ValidateKey(token)
		if err != nil {
			auditLogger.LogEvent(AuditEvent{
				EventType: AuditEventAuthFailed,
				Details:   fmt.Sprintf("Token validation failed: %v", err),
				Success:   false,
			})
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add key info to request context
		r.Header.Set("X-API-Key-Name", key.Name)

		next(w, r)
	}
}

// extractBearerToken extracts Bearer token from Authorization header
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.Split(auth, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}

	return parts[1]
}

// Authentication API Handlers
func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate token
	key, err := apiKeyManager.ValidateKey(payload.Token)
	if err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			Details:   fmt.Sprintf("Login failed: %v", err),
			Success:   false,
		})
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventAuthLogin,
		TokenName: key.Name,
		Details:   "Successful login",
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"key_name": key.Name,
		"permissions": key.Permissions,
	})
}

func handleAuthValidate(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	key, err := apiKeyManager.ValidateKey(token)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid": true,
		"key_name": key.Name,
		"permissions": key.Permissions,
	})
}

func handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	key, err := apiKeyManager.ValidateKey(token)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Regenerate token
	newKey, err := apiKeyManager.RegenerateKey(key.Name)
	if err != nil {
		http.Error(w, "Error regenerating token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"token": newKey.Token,
	})
}

func handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	key, err := apiKeyManager.ValidateKey(token)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventAuthLogout,
		TokenName: key.Name,
		Details:   "Logged out",
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// API Key Management Handlers
func handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		keys := apiKeyManager.ListKeys()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": keys,
		})
	case "POST":
		var payload struct {
			Name        string   `json:"name"`
			Token       string   `json:"token"`
			Permissions []string `json:"permissions"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Convert string permissions to Permission type
		var perms []Permission
		for _, perm := range payload.Permissions {
			perms = append(perms, Permission(perm))
		}

		key, err := apiKeyManager.AddKey(payload.Name, perms, payload.Token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"key":     key,
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPIKeyDetail(w http.ResponseWriter, r *http.Request) {
	// Extract key name from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/api-keys/")
	parts := strings.Split(path, "/")
	keyName := parts[0]

	switch r.Method {
	case "GET":
		key, err := apiKeyManager.GetKey(keyName)
		if err != nil {
			http.Error(w, "API key not found", http.StatusNotFound)
			return
		}

		// Mask token for display
		key.Token = maskToken(key.Token)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(key)

	case "DELETE":
		if err := apiKeyManager.RemoveKey(keyName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})

	case "POST":
		// Check if it's a regenerate or permissions update
		path := r.URL.Path
		if strings.HasSuffix(path, "/regenerate") || strings.HasSuffix(path, "/regenerate/") {
			key, err := apiKeyManager.RegenerateKey(keyName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"token":   key.Token,
			})
		} else if strings.HasSuffix(path, "/permissions") || strings.HasSuffix(path, "/permissions/") {
			var payload struct {
				Add    []string `json:"add"`
				Remove []string `json:"remove"`
			}

			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			var addPerms, removePerms []Permission
			for _, perm := range payload.Add {
				addPerms = append(addPerms, Permission(perm))
			}
			for _, perm := range payload.Remove {
				removePerms = append(removePerms, Permission(perm))
			}

			if err := apiKeyManager.UpdatePermissions(keyName, addPerms, removePerms); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
			})
		} else {
			http.Error(w, "Unknown action", http.StatusBadRequest)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeployAPI handles deployment requests
func handleDeployAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		AppID       string `json:"app_id"`
		Data        string `json:"data"`        // base64 encoded binary or tar.gz
		DataType    string `json:"data_type"`  // "binary" or "folder"
		TargetPath  string `json:"target_path"` // where to deploy
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config, err := loadConfig()
	if err != nil {
		http.Error(w, "Error loading config", http.StatusInternalServerError)
		return
	}

	// Find app
	var app *App
	for i := range config.Apps {
		if config.Apps[i].ID == payload.AppID {
			app = &config.Apps[i]
			break
		}
	}

	if app == nil {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	// Set default target path if not specified
	targetPath := payload.TargetPath
	if targetPath == "" {
		targetPath = fmt.Sprintf("/home/%s/apps/%s", "dk1", payload.AppID)
	}

	// Decode base64 data
	decodedData, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		http.Error(w, "Error decoding data", http.StatusBadRequest)
		return
	}

	// Handle deployment based on type
	var deployError error
	switch payload.DataType {
	case "binary":
		deployError = deployBinary(decodedData, targetPath)
	case "folder":
		deployError = deployFolder(decodedData, targetPath)
	default:
		deployError = fmt.Errorf("unknown data type: %s", payload.DataType)
	}

	if deployError != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Deployment failed for app %s: %v", payload.AppID, deployError),
			Success:   false,
		})
		http.Error(w, deployError.Error(), http.StatusInternalServerError)
		return
	}

	// Update app remote path in config
	for i := range config.Apps {
		if config.Apps[i].ID == payload.AppID {
			config.Apps[i].RemotePath = targetPath
			config.Apps[i].Status = "deployed"
			saveConfig(config)
			break
		}
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Deployment successful for app: %s to %s", payload.AppID, targetPath),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"message":     "Deployment successful",
		"app_id":      payload.AppID,
		"target_path":  targetPath,
		"data_type":   payload.DataType,
	})
}

// deployBinary deploys a single binary file
func deployBinary(data []byte, targetPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	// Write binary
	if err := os.WriteFile(targetPath, data, 0755); err != nil {
		return fmt.Errorf("error writing binary: %v", err)
	}

	return nil
}

// deployFolder deploys a tar.gz folder
func deployFolder(data []byte, targetPath string) error {
	// Ensure directory exists
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	// Create temporary file for tar.gz
	tmpFile := "/tmp/deploy_" + fmt.Sprintf("%d", time.Now().Unix()) + ".tar.gz"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("error writing temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	// Extract tar.gz
	cmd := exec.Command("tar", "-xzf", tmpFile, "-C", targetPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error extracting tar.gz: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// handleAppManagementAPI handles app-specific operations (start/stop/restart/status/logs)
func handleAppManagementAPI(w http.ResponseWriter, r *http.Request) {
	// Extract app ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/apps/")
	parts := strings.Split(path, "/")
	appID := parts[0]

	if len(parts) < 2 {
		http.Error(w, "Invalid request path", http.StatusBadRequest)
		return
	}

	action := parts[1]

	config, err := loadConfig()
	if err != nil {
		http.Error(w, "Error loading config", http.StatusInternalServerError)
		return
	}

	// Find app
	var app *App
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			app = &config.Apps[i]
			break
		}
	}

	if app == nil {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	switch action {
	case "start":
		handleAppStart(w, r, app)
	case "stop":
		handleAppStop(w, r, app)
	case "restart":
		handleAppRestart(w, r, app)
	case "status":
		handleAppStatus(w, r, app)
	case "logs":
		handleAppLogs(w, r, app)
	case "pause":
		handleAppPause(w, r, app)
	case "resume":
		handleAppResume(w, r, app)
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
	}
}

func handleAppStart(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute the command
	cmd := exec.Command("sh", "-c", app.Command)
	if err := cmd.Start(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to start app: %v", err),
		})
		return
	}

	// Resolve actual daemon PID using two-strategy resolver
	actualPID := resolveActualPID(cmd.Process.Pid, app.Port)

	// Update config with resolved PID and status
	config, _ := loadConfig()
	for i := range config.Apps {
		if config.Apps[i].ID == app.ID {
			config.Apps[i].PID = actualPID
			config.Apps[i].Status = "running"
			saveConfig(config)
			break
		}
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventPermissionAdd,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Started app: %s (PID: %d)", app.ID, actualPID),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"status":     "running",
		"pid":        actualPID,
		"shell_pid":  cmd.Process.Pid,
	})
}

func handleAppStop(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config, _ := loadConfig()
	var appPID int
	for i := range config.Apps {
		if config.Apps[i].ID == app.ID {
			appPID = config.Apps[i].PID
			if appPID > 0 {
				// Send SIGTERM to the process
				process, err := os.FindProcess(appPID)
				if err == nil {
					process.Signal(syscall.SIGTERM)
				}
			}
			config.Apps[i].Status = "stopped"
			config.Apps[i].PID = 0
			saveConfig(config)
			break
		}
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventPermissionAdd,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Stopped app: %s (PID: %d)", app.ID, appPID),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "stopped",
		"pid":     appPID,
	})
}

func handleAppRestart(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For now, just update status
	config, _ := loadConfig()
	for i := range config.Apps {
		if config.Apps[i].ID == app.ID {
			config.Apps[i].Status = "running"
			saveConfig(config)
			break
		}
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventPermissionAdd,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Restarted app: %s", app.ID),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "running",
	})
}

func handleAppStatus(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     app.ID,
		"name":   app.Name,
		"status": app.Status,
		"pid":    app.PID,
	})
}

func handleAppLogs(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For now, return empty logs
	// In full implementation, this would stream log files
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"app_id": app.ID,
		"logs":   []string{},
	})
}

func handleAppPause(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if app.PID == 0 || app.Status != "running" {
		http.Error(w, "App is not running", http.StatusBadRequest)
		return
	}

	process, err := os.FindProcess(app.PID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to find process: %v", err), http.StatusInternalServerError)
		return
	}

	if err := process.Signal(syscall.SIGSTOP); err != nil {
		http.Error(w, fmt.Sprintf("Failed to pause process: %v", err), http.StatusInternalServerError)
		return
	}

	config, _ := loadConfig()
	for i := range config.Apps {
		if config.Apps[i].ID == app.ID {
			config.Apps[i].Status = "paused"
			saveConfig(config)
			break
		}
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventPermissionAdd,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Paused app: %s (PID %d)", app.ID, app.PID),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "paused",
		"pid":     app.PID,
	})
}

func handleAppResume(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if app.PID == 0 || app.Status != "paused" {
		http.Error(w, "App is not paused", http.StatusBadRequest)
		return
	}

	process, err := os.FindProcess(app.PID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to find process: %v", err), http.StatusInternalServerError)
		return
	}

	if err := process.Signal(syscall.SIGCONT); err != nil {
		http.Error(w, fmt.Sprintf("Failed to resume process: %v", err), http.StatusInternalServerError)
		return
	}

	config, _ := loadConfig()
	for i := range config.Apps {
		if config.Apps[i].ID == app.ID {
			config.Apps[i].Status = "running"
			saveConfig(config)
			break
		}
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventPermissionAdd,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Resumed app: %s (PID %d)", app.ID, app.PID),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "running",
		"pid":     app.PID,
	})
}
