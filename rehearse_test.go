package main

// Rehearsal against dk1's ACTUAL production config: the regression this guards is
// "setup-traefik wiped 10 hand-added routers", which unit fixtures can't reproduce
// at the scale (59 apps / 128 entries) where it actually bit.
// Skips unless HOTIFY_REHEARSE_DIR points at a copy of the real files.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRehearseAgainstRealDK1Config(t *testing.T) {
	dir := os.Getenv("HOTIFY_REHEARSE_DIR")
	if dir == "" {
		t.Skip("set HOTIFY_REHEARSE_DIR to a dir holding config.real.json + dynamic.work.yml")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.real.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	t.Logf("apps in config: %d", len(cfg.Apps))

	// the file as it stands AFTER hart-domain-sync merged its routers in
	target := filepath.Join(dir, "dynamic.work.yml")
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read dynamic: %v", err)
	}
	beforeSections := splitHTTPSections(string(before))

	generated, err := buildDynamicYAML(&cfg)
	if err != nil {
		t.Fatalf("buildDynamicYAML: %v", err)
	}
	out, preserved := mergeForeignSections(generated, target, &cfg)
	t.Logf("preserved %d foreign entries", len(preserved))

	afterSections := splitHTTPSections(out)

	// 1. hart-domain-sync's routers must survive hotify's regenerate
	for _, sec := range []string{"routers", "services"} {
		for name := range beforeSections[sec] {
			if !strings.HasPrefix(name, "hart-") {
				continue
			}
			if afterSections[sec][name] == "" {
				t.Errorf("hotify regenerate DROPPED foreign entry %s/%s", sec, name)
			}
		}
	}
	// 2. every app hotify manages must still have a router
	for _, app := range cfg.Apps {
		if afterSections["routers"][app.ID] == "" {
			t.Errorf("hotify-owned router %q missing after merge", app.ID)
		}
	}
	// 3. nothing at all may vanish
	for _, sec := range []string{"routers", "services", "middlewares"} {
		for name := range beforeSections[sec] {
			if afterSections[sec][name] == "" {
				t.Errorf("entry vanished: %s/%s", sec, name)
			}
		}
	}
	// 4. idempotent: merging the result again is a fixpoint
	if err := os.WriteFile(filepath.Join(dir, "dynamic.round2.yml"), []byte(out), 0644); err != nil {
		t.Fatal(err)
	}
	out2, _ := mergeForeignSections(generated, filepath.Join(dir, "dynamic.round2.yml"), &cfg)
	if out2 != out {
		t.Error("merge is not idempotent — repeated runs would churn the file")
	}
	t.Logf("routers %d -> %d", len(beforeSections["routers"]), len(afterSections["routers"]))
}
