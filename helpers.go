package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// resolveServerIP returns (ip, warningMsg, error).
// If hint is non-empty it is returned as-is.
// Otherwise tries ifconfig.me; falls back to first non-loopback local IP with a warning.
func resolveServerIP(hint string) (string, string, error) {
	if hint != "" {
		return hint, "", nil
	}

	// Try public IP via ifconfig.me
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ifconfig.me")
	if err == nil {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		if readErr == nil && resp.StatusCode == 200 {
			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
				return ip, "", nil
			}
		}
	}

	// Fallback: first non-loopback local IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", "", fmt.Errorf("could not list network interfaces: %v", err)
	}
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String(),
					fmt.Sprintf("WARNING: ifconfig.me unreachable, using local IP %s — verify this is the correct public IP for DNS", ip4.String()),
					nil
			}
		}
	}

	return "", "", fmt.Errorf("unable to determine server IP: ifconfig.me failed and no local IPv4 found")
}

// checkTraefikInstalled returns ("path", nil) if traefik binary is found, else ("", error).
func checkTraefikInstalled() (string, error) {
	path, err := exec.LookPath("traefik")
	if err != nil {
		return "", fmt.Errorf("traefik binary not found in PATH")
	}
	return path, nil
}
