package main

import (
	"os/exec"
	"testing"
)

func TestCheckTraefikInstalled(t *testing.T) {
	path, err := checkTraefikInstalled()
	// We just verify that the function returns what exec.LookPath would
	expectedPath, expectedErr := exec.LookPath("traefik")
	if path != expectedPath {
		t.Errorf("checkTraefikInstalled() path = %q, want %q", path, expectedPath)
	}
	if (err != nil) != (expectedErr != nil) {
		t.Errorf("checkTraefikInstalled() error = %v, expected error = %v", err, expectedErr)
	}
}
