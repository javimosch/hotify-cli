package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	server *http.Server
	mu     sync.Mutex
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

	// API endpoints
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/api/status", handleStatusAPI)
	mux.HandleFunc("/api/health", handleHealthAPI)
	mux.HandleFunc("/api/config", handleConfigAPI)
	mux.HandleFunc("/api/apps", handleAppsAPI)
	mux.HandleFunc("/api/apps/add", handleAddAppAPI)
	mux.HandleFunc("/api/apps/edit", handleEditAppAPI)
	mux.HandleFunc("/api/apps/remove", handleRemoveAppAPI)
	mux.HandleFunc("/api/apps/setup-dns", handleSetupDNSAPI)
	mux.HandleFunc("/api/apps/setup-traefik", handleSetupTraefikAPI)

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
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := setupTraefikForApp(payload.ID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
