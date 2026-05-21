package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DeploymentClient handles deployment operations against a remote hotify daemon.
type DeploymentClient struct {
	Target     *Remote
	HTTPClient *HTTPClient
}

// NewDeploymentClient creates a new deployment client with decrypted auth token.
func NewDeploymentClient(target *Remote) (*DeploymentClient, error) {
	security, err := NewSecurityManager()
	if err != nil {
		return nil, err
	}
	token, err := security.DecryptToken(target.AuthToken)
	if err != nil {
		return nil, err
	}
	return &DeploymentClient{
		Target:     target,
		HTTPClient: &HTTPClient{BaseURL: target.URL, AuthToken: token},
	}, nil
}

func (d *DeploymentClient) DeployBinary(appID, sourcePath, targetPath string) error {
	binaryData, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("error reading binary file: %v", err)
	}
	return d.HTTPClient.Post("/api/deploy", map[string]interface{}{
		"app_id": appID, "data": base64.StdEncoding.EncodeToString(binaryData),
		"data_type": "binary", "target_path": targetPath,
	})
}

func (d *DeploymentClient) DeployFolder(appID, folderPath, targetPath string) error {
	tarData, err := createTarGz(folderPath)
	if err != nil {
		return fmt.Errorf("error creating tar.gz: %v", err)
	}
	return d.HTTPClient.Post("/api/deploy", map[string]interface{}{
		"app_id": appID, "data": base64.StdEncoding.EncodeToString(tarData),
		"data_type": "folder", "target_path": targetPath,
	})
}

func (d *DeploymentClient) DeployBinaryRsync(sourcePath, remotePath string) error {
	host := strings.Split(strings.TrimPrefix(d.Target.URL, "http://"), ":")[0]
	var stdout, stderr bytes.Buffer
	rsyncCmd := exec.Command("rsync", "-avz", "--progress", sourcePath, host+":"+remotePath)
	rsyncCmd.Stdout = &stdout
	rsyncCmd.Stderr = &stderr
	if err := rsyncCmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
	}
	fmt.Printf("✅ Rsync completed: %s -> %s:%s\n", sourcePath, host, remotePath)
	return nil
}

func (d *DeploymentClient) StartApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/start", appID), nil)
}
func (d *DeploymentClient) StopApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/stop", appID), nil)
}
func (d *DeploymentClient) RestartApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/restart", appID), nil)
}
func (d *DeploymentClient) PauseApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/pause", appID), nil)
}
func (d *DeploymentClient) ResumeApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/resume", appID), nil)
}
func (d *DeploymentClient) GetAppStatus(appID string) (map[string]interface{}, error) {
	return d.HTTPClient.Get(fmt.Sprintf("/api/apps/%s/status", appID))
}
func (d *DeploymentClient) GetAppLogs(appID string) (map[string]interface{}, error) {
	return d.HTTPClient.Get(fmt.Sprintf("/api/apps/%s/logs", appID))
}

// ─── App-specific remote operations ─────────────────────────────────────────

// BasicAuthAdd adds a user with plaintext password to app's basic auth.
func (d *DeploymentClient) BasicAuthAdd(appID, user, password string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/remote/apps/%s/basic-auth", appID), map[string]interface{}{
		"action":   "add",
		"user":     user,
		"password": password,
	})
}

// BasicAuthAddHash adds a pre-hashed htpasswd entry to app's basic auth.
func (d *DeploymentClient) BasicAuthAddHash(appID, hash string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/remote/apps/%s/basic-auth", appID), map[string]interface{}{
		"action": "add",
		"hash":   hash,
	})
}

// BasicAuthRemove removes a user from app's basic auth.
func (d *DeploymentClient) BasicAuthRemove(appID, user string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/remote/apps/%s/basic-auth", appID), map[string]interface{}{
		"action": "remove",
		"user":   user,
	})
}

// BasicAuthList lists users in app's basic auth (passwords masked).
func (d *DeploymentClient) BasicAuthList(appID string) (map[string]interface{}, error) {
	return d.HTTPClient.PostWithData(fmt.Sprintf("/api/remote/apps/%s/basic-auth", appID), map[string]interface{}{
		"action": "list",
	})
}

// SetupTraefikApp configures Traefik for a specific app remotely.
func (d *DeploymentClient) SetupTraefikApp(appID, challengeType string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/remote/apps/%s/setup-traefik", appID), map[string]interface{}{
		"challenge_type": challengeType,
	})
}

// SetupDNSApp configures DNS for a specific app remotely.
func (d *DeploymentClient) SetupDNSApp(appID, ip string) error {
	payload := map[string]interface{}{}
	if ip != "" {
		payload["ip"] = ip
	}
	return d.HTTPClient.Post(fmt.Sprintf("/api/remote/apps/%s/setup-dns", appID), payload)
}

// HTTPClient is a minimal HTTP client for hotify API calls.
type HTTPClient struct {
	BaseURL   string
	AuthToken string
}

func (h *HTTPClient) Post(path string, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %v", err)
	}
	req, err := http.NewRequest("POST", h.BaseURL+path, newRequestBody(data))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.AuthToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	return nil
}

func (h *HTTPClient) Get(path string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", h.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.AuthToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return result, nil
}

func (h *HTTPClient) PostWithData(path string, payload map[string]interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling payload: %v", err)
	}
	req, err := http.NewRequest("POST", h.BaseURL+path, newRequestBody(data))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.AuthToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}
	return result, nil
}

func LocalDeploy(sourcePath, remotePath string) error {
	if err := os.MkdirAll(filepath.Dir(remotePath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("error opening source file: %v", err)
	}
	defer src.Close()
	dst, err := os.Create(remotePath)
	if err != nil {
		return fmt.Errorf("error creating destination file: %v", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("error copying file: %v", err)
	}
	if err := os.Chmod(remotePath, 0755); err != nil {
		return fmt.Errorf("error setting permissions: %v", err)
	}
	fmt.Printf("✅ Deployed: %s -> %s\n", sourcePath, remotePath)
	return nil
}

func ValidateDeployment(binaryPath string) error {
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("binary not found at: %s", binaryPath)
	}
	if info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("binary is not executable: %s", binaryPath)
	}
	return nil
}

func ValidateSource(sourcePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("error accessing source: %v", err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(sourcePath)
		if err != nil {
			return fmt.Errorf("error reading directory: %v", err)
		}
		if len(entries) == 0 {
			return fmt.Errorf("source directory is empty: %s", sourcePath)
		}
	} else {
		f, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("error opening file: %v", err)
		}
		f.Close()
	}
	return nil
}

func CleanupTempFiles() {
	matches, _ := filepath.Glob("/tmp/deploy_*.tar.gz")
	for _, m := range matches {
		os.Remove(m)
	}
}

func createTarGz(sourcePath string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	err := filepath.Walk(sourcePath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(sourcePath, filePath)
		if err != nil {
			return err
		}
		header.Name = relPath
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tarWriter, f)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	tarWriter.Close()
	gzWriter.Close()
	return buf.Bytes(), nil
}
