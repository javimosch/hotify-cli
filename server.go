package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
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
	mux.HandleFunc("/api/remotes/config", authMiddleware(handleRemotesConfigAPI))
	mux.HandleFunc("/api/apps", authMiddleware(handleAppsAPI))
	mux.HandleFunc("/api/apps/add", authMiddleware(handleAddAppAPI))

	// Deployment endpoints (auth required)
	mux.HandleFunc("/api/deploy", authMiddleware(handleDeployAPI))
	mux.HandleFunc("/api/apps/", authMiddleware(handleAppManagementAPI))

	// Docker Compose deployment endpoints (auth required)
	registerComposeRoutes(mux)

	// App-specific remote endpoints (auth required)
	registerAppRemoteRoutes(mux)

	// Traefik system management endpoints (auth required)
	registerTraefikSystemRoutes(mux)
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
    <title>Hotify</title>
    <style>
        :root {
            --canvas: #FBFBFA;
            --surface: #FFFFFF;
            --border: #EAEAEA;
            --border-subtle: rgba(0,0,0,0.06);
            --text-primary: #111111;
            --text-secondary: #787774;
            --text-muted: #ABABAB;
            --font-sans: 'SF Pro Display','Helvetica Neue',-apple-system,BlinkMacSystemFont,sans-serif;
            --font-mono: 'SF Mono','Geist Mono','JetBrains Mono',monospace;
            --radius: 8px;
            --radius-sm: 4px;
            --green-bg: #EDF3EC; --green-text: #346538;
            --red-bg: #FDEBEC;   --red-text: #9F2F2D;
            --yellow-bg: #FBF3DB; --yellow-text: #956400;
        }
        *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: var(--font-sans); background: var(--canvas); color: var(--text-primary); line-height: 1.6; min-height: 100vh; font-size: 14px; }
        #appShell { display: flex; flex-direction: column; min-height: 100vh; }
        #appShell .main { flex: 1; }
        a { color: inherit; text-decoration: none; }

        /* ── Topbar ── */
        .topbar {
            position: sticky; top: 0; z-index: 100;
            background: rgba(251,251,250,0.88);
            backdrop-filter: blur(10px);
            border-bottom: 1px solid var(--border);
            height: 52px; padding: 0 24px;
            display: flex; align-items: center; justify-content: space-between;
        }
        .topbar-brand { font-size: 15px; font-weight: 700; letter-spacing: -0.02em; display: flex; align-items: center; gap: 8px; color: var(--text-primary); }
        .topbar-brand svg { color: var(--text-secondary); }

        /* ── Buttons ── */
        .btn {
            font-family: var(--font-sans); font-size: 12px; font-weight: 500;
            padding: 6px 14px; border: none; border-radius: var(--radius-sm);
            cursor: pointer; transition: background 0.15s, transform 0.1s;
            display: inline-flex; align-items: center; gap: 6px; white-space: nowrap;
        }
        .btn:active { transform: scale(0.98); }
        .btn-primary { background: #111; color: #fff; }
        .btn-primary:hover { background: #333; }
        .btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
        .btn-ghost:hover { background: #F5F5F4; color: var(--text-primary); }
        .btn-danger { background: transparent; color: var(--red-text); border: 1px solid #F5C6C6; }
        .btn-danger:hover { background: var(--red-bg); }

        /* ── Main ── */
        .main { width: 100%; max-width: 1400px; margin: 0 auto; padding: 28px 24px; }

        /* ── Target sections ── */
        .target-section { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; margin-bottom: 20px; overflow: hidden; }
        .target-header { padding: 14px 20px; border-bottom: 1px solid var(--border); display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
        .target-icon { width: 28px; height: 28px; border-radius: var(--radius-sm); border: 1px solid var(--border); background: #F7F6F3; display: flex; align-items: center; justify-content: center; flex-shrink: 0; color: var(--text-secondary); }
        .target-name { font-size: 13px; font-weight: 600; }
        .target-url { font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary); }
        .target-url:hover { color: var(--text-primary); }
        .target-header-actions { margin-left: auto; display: flex; gap: 8px; }

        /* ── Config meta row ── */
        .target-meta { display: flex; flex-wrap: wrap; border-bottom: 1px solid var(--border); }
        .meta-item { padding: 10px 20px; border-right: 1px solid var(--border); }
        .meta-item:last-child { border-right: none; }
        .meta-label { font-size: 10px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 2px; }
        .meta-value { font-size: 13px; font-weight: 500; color: var(--text-primary); font-family: var(--font-mono); }

        /* ── Apps grid ── */
        .apps-area { padding: 16px 20px; }
        .apps-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 10px; }
        .apps-empty { padding: 32px; text-align: center; font-size: 13px; color: var(--text-muted); }

        /* ── App card ── */
        .app-card {
            border: 1px solid var(--border); border-radius: var(--radius);
            padding: 14px 16px; background: #FAFAF9;
            transition: box-shadow 0.2s, border-color 0.2s;
        }
        .app-card:hover { box-shadow: 0 2px 8px rgba(0,0,0,0.04); border-color: #D4D4D4; }
        .app-card-top { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; margin-bottom: 4px; }
        .app-name { font-size: 13px; font-weight: 600; }
        .app-domain { font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary); display: block; margin-bottom: 10px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .app-domain:hover { color: var(--text-primary); }
        .app-meta { font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin-bottom: 3px; }
        .app-internal { font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); display: flex; align-items: center; gap: 4px; margin-bottom: 8px; text-decoration: none; }
        .app-internal:hover { color: var(--text-secondary); }
        .app-actions { display: flex; gap: 5px; margin-top: 12px; flex-wrap: wrap; }

        /* ── Status badge ── */
        .badge { display: inline-flex; align-items: center; gap: 4px; padding: 2px 7px; border-radius: 9999px; font-size: 10px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.05em; flex-shrink: 0; }
        .badge-dot { width: 5px; height: 5px; border-radius: 50%; }
        .badge-running { background: var(--green-bg); color: var(--green-text); }
        .badge-running .badge-dot { background: var(--green-text); }
        .badge-stopped { background: var(--red-bg); color: var(--red-text); }
        .badge-stopped .badge-dot { background: var(--red-text); }
        .badge-unreachable,.badge-unknown { background: var(--yellow-bg); color: var(--yellow-text); }
        .badge-unreachable .badge-dot,.badge-unknown .badge-dot { background: var(--yellow-text); }

        /* ── Login screen ── */
        .login-screen { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px; }
        .login-card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 36px; width: 100%; max-width: 360px; }
        .login-title { font-size: 17px; font-weight: 700; letter-spacing: -0.02em; margin-bottom: 4px; }
        .login-sub { font-size: 13px; color: var(--text-secondary); margin-bottom: 24px; }

        /* ── Modals ── */
        .modal-overlay { display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.3); z-index: 1000; align-items: center; justify-content: center; padding: 16px; }
        .modal-overlay.active { display: flex; }
        .modal-box { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 28px; width: 100%; max-width: 440px; }
        .modal-title { font-size: 15px; font-weight: 600; margin-bottom: 20px; }
        .form-group { margin-bottom: 14px; }
        .form-label { display: block; font-size: 11px; font-weight: 500; color: var(--text-secondary); margin-bottom: 5px; text-transform: uppercase; letter-spacing: 0.05em; }
        .form-input { width: 100%; font-family: var(--font-sans); font-size: 13px; padding: 8px 11px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text-primary); outline: none; transition: border-color 0.15s; }
        .form-input:focus { border-color: #888; }
        .form-input::placeholder { color: var(--text-muted); }
        .modal-actions { display: flex; gap: 8px; margin-top: 20px; justify-content: flex-end; }
        .error-msg { display: none; font-size: 12px; color: var(--red-text); background: var(--red-bg); padding: 8px 11px; border-radius: var(--radius-sm); margin-bottom: 12px; }

        /* ── Privacy mode ── */
        .sensitive { transition: filter 0.2s; }
        .privacy-mode .sensitive { filter: blur(5px); user-select: none; pointer-events: none; }
        .privacy-mode .sensitive:hover { filter: none; pointer-events: auto; }
        .btn-privacy.active { background: #F7F6F3; color: var(--text-primary); border-color: #D4D4D4; }

        /* ── Footer ── */
        .site-footer {
            border-top: 1px solid var(--border);
            padding: 16px 24px;
            display: flex; align-items: center; gap: 6px;
            font-size: 12px; color: var(--text-muted);
            flex-wrap: wrap;
        }
        .footer-link { color: var(--text-secondary); text-decoration: none; display: inline-flex; align-items: center; gap: 4px; }
        .footer-link:hover { color: var(--text-primary); }
        .footer-sep { color: var(--border); }

        /* ── Responsive ── */
        @media (max-width: 600px) {
            .topbar { padding: 0 16px; }
            .main { padding: 16px; }
            .apps-grid { grid-template-columns: 1fr; }
            .apps-area { padding: 12px 16px; }
            .target-meta { flex-direction: column; }
            .meta-item { border-right: none; border-bottom: 1px solid var(--border); }
            .meta-item:last-child { border-bottom: none; }
            .target-header { padding: 12px 16px; }
        }
    </style>
</head>
<body>

<!-- Login screen -->
<div id="loginScreen" style="display:none;">
    <div class="login-screen">
        <div class="login-card">
            <p class="login-title">Hotify</p>
            <p class="login-sub">Traefik &amp; Cloudflare app manager</p>
            <div class="form-group">
                <label class="form-label">API Token</label>
                <input type="password" id="loginToken" class="form-input" placeholder="Paste your token" autofocus>
            </div>
            <div id="loginError" class="error-msg"></div>
            <button class="btn btn-primary" style="width:100%;justify-content:center;padding:9px 14px;" onclick="login()">Sign in</button>
        </div>
    </div>
</div>

<!-- App shell -->
<div id="appShell" style="display:none;">
    <header class="topbar">
        <div class="topbar-brand">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z"/>
            </svg>
            Hotify
        </div>
        <div style="display:flex;gap:8px;align-items:center;">
            <button id="privacyBtn" class="btn btn-ghost btn-privacy" onclick="togglePrivacy()" title="Toggle privacy mode">
                <svg id="privacyIconEye" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
                <svg id="privacyIconEyeOff" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display:none"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>
                Privacy
            </button>
            <button class="btn btn-ghost" onclick="logout()">Sign out</button>
        </div>
    </header>
    <main class="main" id="sections">
        <p style="color:var(--text-muted);font-size:13px;">Loading...</p>
    </main>
    <footer class="site-footer">
        <span>Created with</span>
        <svg width="13" height="13" viewBox="0 0 24 24" fill="#9F2F2D" stroke="none"><path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/></svg>
        <span>by <a href="https://github.com/jarancibia" target="_blank" rel="noopener" class="footer-link">Javier Leandro Arancibia</a></span>
        <span class="footer-sep">&middot;</span>
        <a href="https://github.com/jarancibia/hotify-cli" target="_blank" rel="noopener" class="footer-link">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2z"/></svg>
            hotify-cli
        </a>
    </footer>
</div>

<!-- Edit modal -->
<div id="editModal" class="modal-overlay">
    <div class="modal-box">
        <p class="modal-title">Edit app</p>
        <input type="hidden" id="editAppId">
        <div class="form-group"><label class="form-label">Name</label><input type="text" id="editAppName" class="form-input"></div>
        <div class="form-group"><label class="form-label">Domain</label><input type="text" id="editAppDomain" class="form-input"></div>
        <div class="form-group"><label class="form-label">Port</label><input type="number" id="editAppPort" class="form-input"></div>
        <div class="form-group"><label class="form-label">Command</label><input type="text" id="editAppCommand" class="form-input"></div>
        <div class="form-group"><label class="form-label">Source</label><input type="text" id="editAppSource" class="form-input" placeholder="optional"></div>
        <div class="form-group"><label class="form-label">Backend URL</label><input type="text" id="editAppBackendURL" class="form-input" placeholder="http://100.114.4.57:8080 (optional)"></div>
        <div class="modal-actions">
            <button class="btn btn-ghost" onclick="closeEditModal()">Cancel</button>
            <button class="btn btn-primary" onclick="editApp()">Save</button>
        </div>
    </div>
</div>

<!-- Add modal -->
<div id="addModal" class="modal-overlay">
    <div class="modal-box">
        <p class="modal-title">Add app</p>
        <div class="form-group"><label class="form-label">App ID</label><input type="text" id="appId" class="form-input" placeholder="my-app"></div>
        <div class="form-group"><label class="form-label">Name</label><input type="text" id="appName" class="form-input" placeholder="My App"></div>
        <div class="form-group"><label class="form-label">Subdomain</label><input type="text" id="appDomain" class="form-input" placeholder="myapp"></div>
        <div class="form-group"><label class="form-label">Port</label><input type="number" id="appPort" class="form-input" placeholder="3000"></div>
        <div class="form-group"><label class="form-label">Command</label><input type="text" id="appCommand" class="form-input" placeholder="/path/to/app start"></div>
        <div class="form-group"><label class="form-label">Source</label><input type="text" id="appSource" class="form-input" placeholder="github.com/user/repo (optional)"></div>
        <div class="form-group"><label class="form-label">Backend URL</label><input type="text" id="appBackendURL" class="form-input" placeholder="http://100.114.4.57:8080 (optional)"></div>
        <div class="modal-actions">
            <button class="btn btn-ghost" onclick="closeAddModal()">Cancel</button>
            <button class="btn btn-primary" onclick="addApp()">Add</button>
        </div>
    </div>
</div>

<script>
    let authToken = localStorage.getItem('hotify_token') || '';
    let _appsCache = [];
    let _remoteMap = {};
    let _privacyMode = localStorage.getItem('hotify_privacy') === '1';

    function togglePrivacy() {
        _privacyMode = !_privacyMode;
        localStorage.setItem('hotify_privacy', _privacyMode ? '1' : '0');
        applyPrivacyMode();
    }

    function applyPrivacyMode() {
        document.body.classList.toggle('privacy-mode', _privacyMode);
        var btn = document.getElementById('privacyBtn');
        if (btn) btn.classList.toggle('active', _privacyMode);
        var eye    = document.getElementById('privacyIconEye');
        var eyeOff = document.getElementById('privacyIconEyeOff');
        if (eye)    eye.style.display    = _privacyMode ? 'none' : '';
        if (eyeOff) eyeOff.style.display = _privacyMode ? ''     : 'none';
    }

    applyPrivacyMode();

    // ── Icons ──────────────────────────────────────
    const ICON_MONITOR = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>';
    const ICON_SERVER  = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>';
    const ICON_PLUS    = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>';
    const ICON_LINK    = '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>';
    const ICON_LAN     = '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>';

    // ── Auth ───────────────────────────────────────
    function authHeaders(extra) {
        return Object.assign({ 'Authorization': 'Bearer ' + authToken }, extra || {});
    }

    function showLogin(msg) {
        document.getElementById('loginScreen').style.display = '';
        document.getElementById('appShell').style.display = 'none';
        if (msg) {
            var el = document.getElementById('loginError');
            el.textContent = msg;
            el.style.display = 'block';
        }
    }

    function hideLogin() {
        document.getElementById('loginScreen').style.display = 'none';
        document.getElementById('appShell').style.display = '';
        document.getElementById('loginError').style.display = 'none';
    }

    function handleUnauthorized() {
        localStorage.removeItem('hotify_token');
        authToken = '';
        showLogin('Session expired — please sign in again.');
    }

    function login() {
        var token = document.getElementById('loginToken').value.trim();
        if (!token) return;
        fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ token: token })
        }).then(function(r) {
            if (!r.ok) throw new Error('bad');
            return r.json();
        }).then(function(d) {
            if (d.success) {
                authToken = token;
                localStorage.setItem('hotify_token', token);
                hideLogin();
                loadAll();
            } else {
                showLoginError('Invalid token');
            }
        }).catch(function() { showLoginError('Invalid token'); });
    }

    function showLoginError(msg) {
        var el = document.getElementById('loginError');
        el.textContent = msg;
        el.style.display = 'block';
    }

    function logout() {
        localStorage.removeItem('hotify_token');
        authToken = '';
        showLogin();
    }

    // ── Data loading ───────────────────────────────
    function loadAll() {
        Promise.all([
            fetch('/api/config',         { headers: authHeaders() }),
            fetch('/api/apps',           { headers: authHeaders() }),
            fetch('/api/remotes/config', { headers: authHeaders() })
        ]).then(function(rs) {
            if (rs.some(function(r) { return r.status === 401; })) { handleUnauthorized(); throw new Error('unauthorized'); }
            return Promise.all(rs.map(function(r) { return r.json(); }));
        }).then(function(results) {
            var cfg        = results[0];
            var data       = results[1];
            var remoteCfgs = results[2].remotes || [];
            _appsCache = data.apps || [];
            renderSections(cfg, _appsCache, data.remotes || [], remoteCfgs);
        }).catch(function(e) { if (e.message !== 'unauthorized') console.error(e); });
    }

    // ── Rendering ──────────────────────────────────
    function renderSections(cfg, apps, remotes, remoteCfgs) {
        var groups = { __local__: [] };
        remotes.forEach(function(r) { groups[r.name] = []; });
        apps.forEach(function(a) {
            var k = a.remote || '__local__';
            if (!groups[k]) groups[k] = [];
            groups[k].push(a);
        });
        _remoteMap = {};
        remotes.forEach(function(r) { _remoteMap[r.name] = r; });
        var remoteCfgMap = {};
        (remoteCfgs || []).forEach(function(rc) { remoteCfgMap[rc.name] = rc; });

        var html = renderLocalSection(cfg, groups['__local__']);
        remotes.forEach(function(r) { html += renderRemoteSection(r, groups[r.name] || [], remoteCfgMap[r.name]); });
        document.getElementById('sections').innerHTML = html;
    }

    function metaItem(label, value, sensitive) {
        var cls = sensitive ? ' sensitive' : '';
        return '<div class="meta-item"><div class="meta-label">' + label + '</div><div class="meta-value' + cls + '">' + value + '</div></div>';
    }

    function renderLocalSection(cfg, apps) {
        var meta = metaItem('Domain', cfg.domain || '—', true) +
                   metaItem('Email', cfg.admin_email || '—', true) +
                   metaItem('Cloudflare', cfg.cloudflare_token ? '••••••••' : 'Not set');
        return '<div class="target-section">' +
            '<div class="target-header">' +
            '<div class="target-icon">' + ICON_MONITOR + '</div>' +
            '<span class="target-name">Local</span>' +
            '<div class="target-header-actions"><button class="btn btn-primary" onclick="openAddModal()">' + ICON_PLUS + 'Add app</button></div>' +
            '</div>' +
            '<div class="target-meta">' + meta + '</div>' +
            renderAppsArea(apps) +
            '</div>';
    }

    function renderRemoteSection(remote, apps, rc) {
        var meta = '';
        if (rc) {
            var reachBadge = rc.reachable
                ? '<span class="badge badge-running"><span class="badge-dot"></span>reachable</span>'
                : '<span class="badge badge-stopped"><span class="badge-dot"></span>unreachable</span>';
            meta = '<div class="target-meta">' +
                metaItem('Status', reachBadge) +
                metaItem('Domain', rc.domain || '—', true) +
                metaItem('Email', rc.admin_email || '—', true) +
                metaItem('Cloudflare', rc.cloudflare_set ? '••••••••' : 'Not set') +
                '</div>';
        }
        return '<div class="target-section">' +
            '<div class="target-header">' +
            '<div class="target-icon">' + ICON_SERVER + '</div>' +
            '<span class="target-name">' + remote.name + '</span>' +
            '<a href="' + remote.url + '" target="_blank" rel="noopener" class="target-url sensitive">' + remote.url + '</a>' +
            '<div class="target-header-actions"><button class="btn btn-primary" onclick="openAddModal()">' + ICON_PLUS + 'Add app</button></div>' +
            '</div>' +
            meta +
            renderAppsArea(apps) +
            '</div>';
    }

    function renderAppsArea(apps) {
        if (!apps || apps.length === 0) {
            return '<div class="apps-empty">No apps configured.</div>';
        }
        return '<div class="apps-area"><div class="apps-grid">' + apps.map(renderAppCard).join('') + '</div></div>';
    }

    function badgeHTML(status) {
        var cls = 'badge-' + (status || 'unknown').replace(/[^a-z]/g, '');
        return '<span class="badge ' + cls + '"><span class="badge-dot"></span>' + (status || 'unknown') + '</span>';
    }

    function internalURL(app) {
        if (!app.port) return '';
        var host = 'localhost';
        if (app.remote && _remoteMap[app.remote]) {
            try { host = new URL(_remoteMap[app.remote].url).hostname; } catch(e) {}
        }
        return 'http://' + host + ':' + app.port;
    }

    function renderAppCard(app) {
        var id = app.id;
        var domain = app.domain || '';
        var domainLink = domain
            ? '<a href="https://' + domain + '" target="_blank" rel="noopener" class="app-domain sensitive">' + ICON_LINK + ' ' + domain + '</a>'
            : '<span class="app-domain" style="color:var(--text-muted)">No domain</span>';
        var iurl = internalURL(app);
        var internalLink = iurl
            ? '<a href="' + iurl + '" target="_blank" rel="noopener" class="app-internal sensitive">' + ICON_LAN + ' ' + iurl + '</a>'
            : '';
        return '<div class="app-card">' +
            '<div class="app-card-top">' +
            '<span class="app-name">' + (app.name || id) + '</span>' +
            badgeHTML(app.status) +
            '</div>' +
            domainLink +
            internalLink +
            (app.command ? '<div class="app-meta">' + app.command + '</div>' : '') +
            (app.source  ? '<div class="app-meta">' + app.source  + '</div>' : '') +
            '<div class="app-actions">' +
            '<button class="btn btn-ghost" onclick="openEditModalById(\'' + id + '\')">Edit</button>' +
            '<button class="btn btn-ghost" onclick="setupDNS(\'' + id + '\')">DNS</button>' +
            '<button class="btn btn-ghost" onclick="setupTraefik(\'' + id + '\')">Traefik</button>' +
            '<button class="btn btn-danger" onclick="removeApp(\'' + id + '\')">Remove</button>' +
            '</div>' +
            '</div>';
    }

    // ── Modals ─────────────────────────────────────
    function openAddModal()  { document.getElementById('addModal').classList.add('active'); }
    function closeAddModal() { document.getElementById('addModal').classList.remove('active'); }
    function closeEditModal(){ document.getElementById('editModal').classList.remove('active'); }

    function openEditModalById(id) {
        var app = _appsCache.find(function(a) { return a.id === id; });
        if (!app) return;
        document.getElementById('editAppId').value        = app.id;
        document.getElementById('editAppName').value      = app.name    || '';
        document.getElementById('editAppDomain').value    = app.domain  || '';
        document.getElementById('editAppPort').value      = app.port    || '';
        document.getElementById('editAppCommand').value   = app.command || '';
        document.getElementById('editAppSource').value    = app.source  || '';
        document.getElementById('editAppBackendURL').value = app.backend_url || '';
        document.getElementById('editModal').classList.add('active');
    }

    // ── CRUD ───────────────────────────────────────
    function apiFetch(url, opts) {
        return fetch(url, Object.assign({ headers: authHeaders(opts && opts.json ? { 'Content-Type': 'application/json' } : {}) }, opts))
            .then(function(r) {
                if (r.status === 401) { handleUnauthorized(); throw new Error('unauthorized'); }
                return r.json();
            });
    }

    function editApp() {
        var app = {
            id:         document.getElementById('editAppId').value,
            name:       document.getElementById('editAppName').value,
            domain:     document.getElementById('editAppDomain').value,
            port:       parseInt(document.getElementById('editAppPort').value) || 0,
            command:    document.getElementById('editAppCommand').value,
            source:     document.getElementById('editAppSource').value,
            backend_url: document.getElementById('editAppBackendURL').value
        };
        apiFetch('/api/apps/edit', { method: 'POST', json: true, body: JSON.stringify(app) })
            .then(function(d) { if (d.success) { closeEditModal(); loadAll(); } else alert(d.error); })
            .catch(function(e) { if (e.message !== 'unauthorized') console.error(e); });
    }

    function addApp() {
        var app = {
            id:         document.getElementById('appId').value,
            name:       document.getElementById('appName').value,
            domain:     document.getElementById('appDomain').value,
            port:       parseInt(document.getElementById('appPort').value),
            command:    document.getElementById('appCommand').value,
            source:     document.getElementById('appSource').value,
            backend_url: document.getElementById('appBackendURL').value
        };
        apiFetch('/api/apps/add', { method: 'POST', json: true, body: JSON.stringify(app) })
            .then(function(d) { if (d.success) { closeAddModal(); loadAll(); } else alert(d.error); })
            .catch(function(e) { if (e.message !== 'unauthorized') console.error(e); });
    }

    function setupDNS(id) {
        if (!confirm('Setup DNS for this app?')) return;
        apiFetch('/api/apps/setup-dns', { method: 'POST', json: true, body: JSON.stringify({ id: id, ip: '92.113.145.178' }) })
            .then(function(d) { alert(d.success ? 'DNS setup successful!' : 'Error: ' + d.error); })
            .catch(function(e) { if (e.message !== 'unauthorized') console.error(e); });
    }

    function setupTraefik(id) {
        if (!confirm('Setup Traefik for this app?')) return;
        apiFetch('/api/apps/setup-traefik', { method: 'POST', json: true, body: JSON.stringify({ id: id }) })
            .then(function(d) { alert(d.success ? 'Traefik setup successful!' : 'Error: ' + d.error); })
            .catch(function(e) { if (e.message !== 'unauthorized') console.error(e); });
    }

    function removeApp(id) {
        if (!confirm('Remove this app?')) return;
        apiFetch('/api/apps/remove', { method: 'POST', json: true, body: JSON.stringify({ id: id }) })
            .then(function(d) { if (d.success) loadAll(); else alert(d.error); })
            .catch(function(e) { if (e.message !== 'unauthorized') console.error(e); });
    }

    // ── Boot ───────────────────────────────────────
    if (!authToken) {
        showLogin();
    } else {
        hideLogin();
        loadAll();
    }

    setInterval(function() { if (authToken) loadAll(); }, 30000);

    document.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') {
            if (document.getElementById('loginScreen').style.display !== 'none') login();
            else if (document.getElementById('editModal').classList.contains('active')) editApp();
        }
        if (e.key === 'Escape') { closeAddModal(); closeEditModal(); }
    });
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

	type RemoteInfo struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	remotes := make([]RemoteInfo, len(config.Remotes))
	for i, r := range config.Remotes {
		remotes[i] = RemoteInfo{Name: r.Name, URL: r.URL}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cloudflare_token": config.CloudflareToken,
		"domain":           config.Domain,
		"admin_email":      config.AdminEmail,
		"remotes":          remotes,
	})
}

// handleRemotesConfigAPI fetches /api/config from each configured remote and returns a map of name→config.
func handleRemotesConfigAPI(w http.ResponseWriter, r *http.Request) {
	config, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	security, _ := NewSecurityManager()
	type RemoteCfg struct {
		Name           string `json:"name"`
		URL            string `json:"url"`
		Domain         string `json:"domain"`
		AdminEmail     string `json:"admin_email"`
		CloudflareSet  bool   `json:"cloudflare_set"`
		Reachable      bool   `json:"reachable"`
	}
	results := make([]RemoteCfg, 0, len(config.Remotes))
	for _, remote := range config.Remotes {
		rc := RemoteCfg{Name: remote.Name, URL: remote.URL}
		token, decErr := security.DecryptToken(remote.AuthToken)
		if decErr == nil {
			client := &http.Client{Timeout: 5 * time.Second}
			req, reqErr := http.NewRequest("GET", remote.URL+"/api/config", nil)
			if reqErr == nil {
				req.Header.Set("Authorization", "Bearer "+token)
				resp, doErr := client.Do(req)
				if doErr == nil && resp.StatusCode == http.StatusOK {
					var cfg map[string]interface{}
					if json.NewDecoder(resp.Body).Decode(&cfg) == nil {
						rc.Reachable = true
						if v, ok := cfg["domain"].(string); ok { rc.Domain = v }
						if v, ok := cfg["admin_email"].(string); ok { rc.AdminEmail = v }
						if v, ok := cfg["cloudflare_token"].(string); ok { rc.CloudflareSet = v != "" }
					}
					resp.Body.Close()
				}
			}
		}
		results = append(results, rc)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"remotes": results})
}

func handleAppsAPI(w http.ResponseWriter, r *http.Request) {
	config, err := loadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	apps := make([]App, len(config.Apps))
	for i, app := range config.Apps {
		apps[i] = app
		if app.Domain != "" && app.Remote != "" {
			apps[i].Status = checkDomainStatus(app.Domain)
		} else if app.Port > 0 {
			apps[i].Status = checkLocalPortStatus(app.Port)
		}
	}

	type RemoteInfo struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	remotes := make([]RemoteInfo, len(config.Remotes))
	for i, r := range config.Remotes {
		remotes[i] = RemoteInfo{Name: r.Name, URL: r.URL}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"apps":    apps,
		"remotes": remotes,
	})
}

// checkDomainStatus probes the app's public domain over HTTPS then HTTP.
// Any HTTP response (even 4xx) means the app is up.
func checkDomainStatus(domain string) string {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for _, scheme := range []string{"https", "http"} {
		resp, err := client.Get(scheme + "://" + domain)
		if err == nil {
			resp.Body.Close()
			return "running"
		}
	}
	return "stopped"
}

// checkLocalPortStatus checks if something is listening on localhost:port.
func checkLocalPortStatus(port int) string {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 2*time.Second)
	if err != nil {
		return "stopped"
	}
	conn.Close()
	return "running"
}

func handleAddAppAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var app struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Domain     string `json:"domain"`
		Port       int    `json:"port"`
		Command    string `json:"command"`
		Source     string `json:"source"`
		BackendURL string `json:"backend_url"`
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
		ID:         app.ID,
		Name:       app.Name,
		Domain:     fullDomain,
		Port:       app.Port,
		Command:    app.Command,
		Source:     app.Source,
		Status:     "stopped",
		BackendURL: app.BackendURL,
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
		ID         string `json:"id"`
		Name       string `json:"name"`
		Domain     string `json:"domain"`
		Port       int    `json:"port"`
		Command    string `json:"command"`
		Source     string `json:"source"`
		BackendURL string `json:"backend_url"`
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
			if app.BackendURL != "" {
				config.Apps[i].BackendURL = app.BackendURL
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

	if err := setupTraefikForAppWithChallenge(payload.ID, ct, false); err != nil {
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

		// Check permissions for the endpoint
		requiredPermissions := GetRequiredPermissions(r.URL.Path, r.Method)
		if !CheckTokenPermissions(token, requiredPermissions) {
			auditLogger.LogEvent(AuditEvent{
				EventType: AuditEventAuthFailed,
				TokenName: key.Name,
				Details:   fmt.Sprintf("Permission denied for %s %s. Required: %v", r.Method, r.URL.Path, requiredPermissions),
				Success:   false,
			})
			http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
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

	// For Docker Compose apps, check actual compose stack status
	actualStatus := app.Status
	actualPID := app.PID

	if app.ComposeFile != "" && app.ComposePath != "" {
		composeStatus, composePID := checkComposeStatus(app.ComposePath, app.ComposeFile, app.Port)
		if composeStatus != "not_running" {
			actualStatus = composeStatus
			actualPID = composePID

			// Update cached status in config
			config, _ := loadConfig()
			for i := range config.Apps {
				if config.Apps[i].ID == app.ID {
					config.Apps[i].Status = actualStatus
					config.Apps[i].PID = actualPID
					saveConfig(config)
					break
				}
			}
		}
	} else if app.PID > 0 {
		// For regular apps, check if PID is still alive
		if !pidExists(app.PID) {
			actualStatus = "not_running"
			actualPID = 0

			// Update cached status
			config, _ := loadConfig()
			for i := range config.Apps {
				if config.Apps[i].ID == app.ID {
					config.Apps[i].Status = actualStatus
					config.Apps[i].PID = actualPID
					saveConfig(config)
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     app.ID,
		"name":   app.Name,
		"status": actualStatus,
		"pid":    actualPID,
	})
}

// checkComposeStatus checks if a Docker Compose stack is running
// Returns (status, pid) where pid is 0 if not found
func checkComposeStatus(composePath string, composeFile string, port int) (string, int) {
	// Check if compose file exists
	composeFilePath := filepath.Join(composePath, composeFile)
	if _, err := os.Stat(composeFilePath); err != nil {
		return "not_running", 0
	}

	// For Docker Compose apps, just check if the port is listening
	// This is simpler and more reliable than parsing docker compose ps output
	if port > 0 {
		if pid := findPIDByPort(port); pid > 0 {
			return "running", pid
		}
	}

	// Fallback: try docker compose ps (basic check)
	cmd := exec.Command("docker", "compose", "-f", composeFile, "ps", "--format", "{{.State}}")
	cmd.Dir = composePath
	output, err := cmd.Output()
	if err != nil {
		return "not_running", 0
	}

	// Check if any service is running
	states := strings.Split(string(output), "\n")
	for _, state := range states {
		if strings.TrimSpace(state) == "running" {
			return "running", 0
		}
	}

	return "not_running", 0
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
