package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTraefikWatchesDynamicConfig(t *testing.T) {
	cases := map[string]bool{
		"providers:\n  file:\n    directory: /etc/traefik/dynamic.d\n    watch: true\n":  true,
		"providers:\n  file:\n    filename: /etc/traefik/dynamic.yml\n    watch: true\n": true,
		"providers:\n  file:\n    directory: /etc/traefik/dynamic.d\n":                   false,
		"providers:\n  file:\n    watch: true\n":                                         false,
		"entryPoints:\n  web:\n    address: :80\n":                                       false,
		"not: [valid": false,
	}
	dir := t.TempDir()
	for body, want := range cases {
		p := filepath.Join(dir, "traefik.yml")
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		if got := traefikWatchesDynamicConfig(p); got != want {
			t.Errorf("traefikWatchesDynamicConfig(%q) = %v, want %v", body, got, want)
		}
	}
	if traefikWatchesDynamicConfig(filepath.Join(dir, "missing.yml")) {
		t.Error("missing traefik.yml must not count as watched")
	}
}
