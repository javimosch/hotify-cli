package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The behaviour under test is the one that used to lose data: `setup-traefik`
// regenerates the whole dynamic file from config.json, so anything another tool
// wrote there (hart-domain-sync's per-domain routers, a hand-added middleware) was
// silently erased. Now foreign entries survive, and hotify still wins on collisions.

const foreignDynamic = `http:
  routers:
    hart-lbtt-intrane-fr:
      rule: "Host(` + "`lbtt.intrane.fr`" + `)"
      entryPoints: [websecure]
      service: hart-lbtt-intrane-fr
      tls:
        certResolver: letsencrypt
    myapp:
      rule: "Host(` + "`stale.example.com`" + `)"
      service: myapp
  services:
    hart-lbtt-intrane-fr:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8799"
    myapp:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:9999"
  middlewares:
    handrolled-headers:
      headers:
        customRequestHeaders:
          X-Custom: "yes"
`

func writeTmp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestMergeForeignSections_PreservesForeignEntries(t *testing.T) {
	dir := t.TempDir()
	existing := writeTmp(t, dir, "dynamic.yml", foreignDynamic)

	cfg := &Config{Apps: []App{{ID: "myapp", Domain: "myapp.example.com", Port: 8080}}}
	generated := "http:\n  routers:\n    myapp:\n      rule: \"Host(`myapp.example.com`)\"\n      service: myapp\n" +
		"  services:\n    myapp:\n      loadBalancer:\n        servers:\n          - url: \"http://127.0.0.1:8080\"\n"

	out, preserved := mergeForeignSections(generated, existing, cfg)

	// the other tool's router/service survive
	if !strings.Contains(out, "hart-lbtt-intrane-fr:") {
		t.Fatalf("foreign router was dropped:\n%s", out)
	}
	if !strings.Contains(out, "handrolled-headers:") {
		t.Fatalf("foreign middleware was dropped:\n%s", out)
	}
	if !strings.Contains(out, "lbtt.intrane.fr") {
		t.Fatalf("foreign router body was dropped:\n%s", out)
	}
	if len(preserved) == 0 {
		t.Fatal("expected preserved entries to be reported for logging")
	}
}

func TestMergeForeignSections_HotifyWinsOnCollision(t *testing.T) {
	dir := t.TempDir()
	existing := writeTmp(t, dir, "dynamic.yml", foreignDynamic)

	// `myapp` exists in BOTH: hotify's config.json says myapp.example.com:8080, the
	// file on disk has a stale stale.example.com:9999. Hotify's must win, otherwise a
	// stale copy would shadow the source of truth.
	cfg := &Config{Apps: []App{{ID: "myapp", Domain: "myapp.example.com", Port: 8080}}}
	generated := "http:\n  routers:\n    myapp:\n      rule: \"Host(`myapp.example.com`)\"\n      service: myapp\n" +
		"  services:\n    myapp:\n      loadBalancer:\n        servers:\n          - url: \"http://127.0.0.1:8080\"\n"

	out, _ := mergeForeignSections(generated, existing, cfg)

	if strings.Contains(out, "stale.example.com") {
		t.Fatalf("stale foreign copy of an owned router shadowed hotify's:\n%s", out)
	}
	if !strings.Contains(out, "myapp.example.com") {
		t.Fatalf("hotify's own router was lost:\n%s", out)
	}
	if strings.Contains(out, ":9999") {
		t.Fatalf("stale backend url survived for an owned service:\n%s", out)
	}
}

func TestMergeForeignSections_OwnedMiddlewaresNotDuplicated(t *testing.T) {
	dir := t.TempDir()
	// a previous hotify run's basic-auth middleware; hotify owns "<id>-basic-auth"
	prev := "http:\n  middlewares:\n    myapp-basic-auth:\n      basicAuth:\n        users:\n          - \"old:hash\"\n"
	existing := writeTmp(t, dir, "dynamic.yml", prev)
	cfg := &Config{Apps: []App{{ID: "myapp", Domain: "myapp.example.com", Port: 8080}}}
	generated := "http:\n  middlewares:\n    myapp-basic-auth:\n      basicAuth:\n        users:\n          - \"new:hash\"\n"

	out, preserved := mergeForeignSections(generated, existing, cfg)
	if strings.Contains(out, "old:hash") {
		t.Fatalf("stale owned middleware survived:\n%s", out)
	}
	for _, p := range preserved {
		if strings.Contains(p, "myapp-basic-auth") {
			t.Fatalf("owned middleware wrongly reported as preserved foreign entry")
		}
	}
}

func TestMergeForeignSections_NoExistingFileIsANoop(t *testing.T) {
	generated := "http:\n  routers:\n    a:\n      rule: \"Host(`a.example.com`)\"\n"
	out, preserved := mergeForeignSections(generated, filepath.Join(t.TempDir(), "absent.yml"), &Config{})
	if out != generated || preserved != nil {
		t.Fatalf("expected a no-op when the target does not exist")
	}
}

func TestResolveTraefikMode(t *testing.T) {
	dir := t.TempDir()
	oldMain := traefikMain
	defer func() { traefikMain = oldMain }()

	// explicit config wins over whatever is on disk
	traefikMain = writeTmp(t, dir, "traefik-dir.yml",
		"providers:\n  file:\n    directory: /etc/traefik/dynamic.d\n    watch: true\n")
	if got := resolveTraefikMode(&Config{TraefikMode: "file"}); got != TraefikModeFile {
		t.Fatalf("explicit config should win, got %q", got)
	}

	// inferred: directory
	if got := resolveTraefikMode(&Config{}); got != TraefikModeDirectory {
		t.Fatalf("should infer directory from traefik.yml, got %q", got)
	}

	// inferred: file
	traefikMain = writeTmp(t, dir, "traefik-file.yml",
		"providers:\n  file:\n    filename: /etc/traefik/dynamic.yml\n    watch: true\n")
	if got := resolveTraefikMode(&Config{}); got != TraefikModeFile {
		t.Fatalf("should infer file from traefik.yml, got %q", got)
	}

	// missing traefik.yml => the historical default, so upgrades are a no-op
	traefikMain = filepath.Join(dir, "does-not-exist.yml")
	if got := resolveTraefikMode(&Config{}); got != TraefikModeFile {
		t.Fatalf("missing traefik.yml should default to file mode, got %q", got)
	}
}

func TestProvidersBlockFor(t *testing.T) {
	if !strings.Contains(providersBlockFor(TraefikModeFile, false), "filename: /etc/traefik/dynamic.yml") {
		t.Fatal("file mode must emit filename:")
	}
	if !strings.Contains(providersBlockFor(TraefikModeDirectory, false), "directory: /etc/traefik/dynamic.d") {
		t.Fatal("directory mode must emit directory:")
	}
	withDocker := providersBlockFor(TraefikModeDirectory, true)
	if !strings.Contains(withDocker, "docker:") || !strings.Contains(withDocker, "directory:") {
		t.Fatal("docker + directory must both appear")
	}
}

func TestDynamicTargetPath(t *testing.T) {
	oldMain := traefikMain
	defer func() { traefikMain = oldMain }()
	traefikMain = filepath.Join(t.TempDir(), "absent.yml") // => file mode
	if got := dynamicTargetPath(&Config{}); got != traefikDynamic {
		t.Fatalf("file mode target = %q", got)
	}
	if got := dynamicTargetPath(&Config{TraefikMode: "directory"}); got != traefikDirOwnFile {
		t.Fatalf("directory mode target = %q", got)
	}
}
