package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEventType represents the type of audit event
type AuditEventType string

const (
	AuditEventAuthLogin       AuditEventType = "auth_login"
	AuditEventAuthLogout      AuditEventType = "auth_logout"
	AuditEventAuthFailed      AuditEventType = "auth_failed"
	AuditEventAPIKeyCreate   AuditEventType = "api_key_create"
	AuditEventAPIKeyDelete   AuditEventType = "api_key_delete"
	AuditEventAPIKeyRegen    AuditEventType = "api_key_regenerate"
	AuditEventPermissionAdd  AuditEventType = "permission_add"
	AuditEventPermissionRemove AuditEventType = "permission_remove"
	AuditEventDeploy         AuditEventType = "deploy"
	AuditEventAppStart       AuditEventType = "app_start"
	AuditEventAppStop        AuditEventType = "app_stop"
	AuditEventAppRestart     AuditEventType = "app_restart"
	AuditEventConfigChange   AuditEventType = "config_change"
)

// AuditEvent represents a single audit event
type AuditEvent struct {
	Timestamp   time.Time      `json:"timestamp"`
	EventType  AuditEventType `json:"event_type"`
	TokenName  string         `json:"token_name,omitempty"`
	IPAddress string         `json:"ip_address,omitempty"`
	Details    string         `json:"details,omitempty"`
	Success    bool           `json:"success"`
}

// AuditLogger handles audit logging
type AuditLogger struct {
	logFile  string
	mu       sync.Mutex
	events   []AuditEvent
	maxSize  int
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger() (*AuditLogger, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error getting home directory: %v", err)
	}

	auditDir := filepath.Join(homeDir, ".hotify", "audit")
	if err := os.MkdirAll(auditDir, 0700); err != nil {
		return nil, fmt.Errorf("error creating audit directory: %v", err)
	}

	logFile := filepath.Join(auditDir, "audit.log")

	return &AuditLogger{
		logFile: logFile,
		maxSize: 10000, // Keep last 10,000 events in memory
	}, nil
}

// LogEvent logs an audit event
func (a *AuditLogger) LogEvent(event AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	event.Timestamp = time.Now()

	// Add to memory buffer
	a.events = append(a.events, event)
	if len(a.events) > a.maxSize {
		a.events = a.events[1:]
	}

	// Write to file
	return a.appendToFile(event)
}

// appendToFile appends an event to the log file
func (a *AuditLogger) appendToFile(event AuditEvent) error {
	file, err := os.OpenFile(a.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("error opening audit log file: %v", err)
	}
	defer file.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshaling audit event: %v", err)
	}

	_, err = file.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("error writing audit event: %v", err)
	}

	return nil
}

// GetRecentEvents returns recent events of a specific type
func (a *AuditLogger) GetRecentEvents(eventType AuditEventType, limit int) ([]AuditEvent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var filtered []AuditEvent
	count := 0

	// Iterate backwards to get most recent first
	for i := len(a.events) - 1; i >= 0 && count < limit; i-- {
		if eventType == "" || a.events[i].EventType == eventType {
			filtered = append(filtered, a.events[i])
			count++
		}
	}

	return filtered, nil
}

// GetFailedAuthAttempts returns failed authentication attempts
func (a *AuditLogger) GetFailedAuthAttempts(since time.Time) ([]AuditEvent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var failed []AuditEvent
	for _, event := range a.events {
		if event.EventType == AuditEventAuthFailed && 
		   event.Timestamp.After(since) && 
		   !event.Success {
			failed = append(failed, event)
		}
	}

	return failed, nil
}

// RotateLogs rotates audit logs based on retention policy
func (a *AuditLogger) RotateLogs(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	a.mu.Lock()
	defer a.mu.Unlock()

	// Filter in-memory events
	var filtered []AuditEvent
	for _, event := range a.events {
		if event.Timestamp.After(cutoff) {
			filtered = append(filtered, event)
		}
	}
	a.events = filtered

	// Rewrite log file with filtered events
	if err := os.Remove(a.logFile); err != nil {
		return fmt.Errorf("error removing old log file: %v", err)
	}

	for _, event := range filtered {
		if err := a.appendToFile(event); err != nil {
			return err
		}
	}

	return nil
}

// LoadEventsFromDisk loads events from the log file
func (a *AuditLogger) LoadEventsFromDisk() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := os.ReadFile(a.logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No log file yet
		}
		return fmt.Errorf("error reading audit log: %v", err)
	}

	lines := splitLines(data)
	a.events = make([]AuditEvent, 0, len(lines))

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var event AuditEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // Skip malformed lines
		}

		a.events = append(a.events, event)
	}

	// Trim to max size
	if len(a.events) > a.maxSize {
		a.events = a.events[len(a.events)-a.maxSize:]
	}

	return nil
}

// splitLines splits byte data by newlines
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0

	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}

	if start < len(data) {
		lines = append(lines, data[start:])
	}

	return lines
}
