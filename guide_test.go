package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// ─── buildGuide ─────────────────────────────────────────────────────────────

func TestBuildGuide_HasAllCommands(t *testing.T) {
	g := buildGuide()

	if g.Version != Version {
		t.Errorf("guide version %q, want %q", g.Version, Version)
	}
	if g.Schema != "hotify.guide/v1" {
		t.Errorf("guide schema %q, want hotify.guide/v1", g.Schema)
	}

	// Count commands by category
	catCount := make(map[string]int)
	for _, cmd := range g.Commands {
		catCount[cmd.Category]++
	}

	// Expect at least these categories
	expectedCats := []string{"meta", "configuration", "deployment", "dns-traefik", "process", "docker", "infrastructure", "remote"}
	for _, cat := range expectedCats {
		if catCount[cat] == 0 {
			t.Errorf("no commands in category %q", cat)
		}
	}

	// Verify essential commands exist
	essentialCmds := map[string]bool{
		"init": false, "setup": false, "setup-dns": false,
		"setup-traefik": false, "basic-auth": false, "guide": false,
		"start": false, "stop": false, "deploy": false,
	}
	for _, cmd := range g.Commands {
		if _, ok := essentialCmds[cmd.Name]; ok {
			essentialCmds[cmd.Name] = true
		}
	}
	for cmd, found := range essentialCmds {
		if !found {
			t.Errorf("essential command %q not found in guide", cmd)
		}
	}

	// Verify tips and gotchas exist
	if len(g.Tips) == 0 {
		t.Error("guide has no tips")
	}
	if len(g.Gotchas) == 0 {
		t.Error("guide has no gotchas")
	}
	if len(g.Workflows) == 0 {
		t.Error("guide has no workflows")
	}
}

func TestBuildGuide_CommandsHaveFlags(t *testing.T) {
	g := buildGuide()

	for _, cmd := range g.Commands {
		// Every non-meta command should have a summary
		if cmd.Summary == "" && cmd.Category != "meta" {
			t.Errorf("command %q (category %q) has no summary", cmd.Name, cmd.Category)
		}
		// Required flags should have the required field set
		for _, f := range cmd.Flags {
			if f.Name == "" {
				t.Errorf("command %q has a flag with empty name", cmd.Name)
			}
		}
	}
}

func TestBuildGuide_WorkflowsHaveSteps(t *testing.T) {
	g := buildGuide()

	for _, w := range g.Workflows {
		if len(w.Steps) == 0 {
			t.Errorf("workflow %q has no steps", w.Name)
		}
		if w.Detail == "" {
			t.Errorf("workflow %q has no detail", w.Name)
		}
	}
}

func TestBuildGuide_AllCommandNamesUnique(t *testing.T) {
	g := buildGuide()
	seen := make(map[string]bool)
	for _, cmd := range g.Commands {
		if seen[cmd.Name] {
			t.Errorf("duplicate command name %q", cmd.Name)
		}
		seen[cmd.Name] = true
	}
}

func TestBuildGuide_BeforeAfterReferToExistingCommands(t *testing.T) {
	g := buildGuide()
	cmdNames := make(map[string]bool)
	for _, cmd := range g.Commands {
		cmdNames[cmd.Name] = true
	}

	for _, cmd := range g.Commands {
		for _, before := range cmd.Before {
			if !cmdNames[before] {
				t.Errorf("command %q references Before=%q which doesn't exist", cmd.Name, before)
			}
		}
		for _, after := range cmd.After {
			if !cmdNames[after] {
				t.Errorf("command %q references After=%q which doesn't exist", cmd.Name, after)
			}
		}
	}
}

// ─── renderGuideText ────────────────────────────────────────────────────────

func TestRenderGuideText(t *testing.T) {
	g := buildGuide()
	text := renderGuideText(g)

	if text == "" {
		t.Fatal("renderGuideText returned empty")
	}

	// Should contain version
	if !strings.Contains(text, g.Version) {
		t.Errorf("rendered text missing version %q", g.Version)
	}

	// Should contain command names
	for _, cmd := range g.Commands {
		if !strings.Contains(text, cmd.Name) {
			t.Errorf("rendered text missing command %q", cmd.Name)
			break // one error is enough
		}
	}

	// Should contain section headers
	sections := []string{"Self-description", "Configuration", "Deployment", "DNS & Traefik",
		"Process Management", "Docker", "Infrastructure", "Remote Management"}
	for _, s := range sections {
		if !strings.Contains(text, s) {
			t.Errorf("rendered text missing section %q", s)
		}
	}

	// Should contain workflows
	for _, w := range g.Workflows {
		if !strings.Contains(text, w.Name) {
			t.Errorf("rendered text missing workflow %q", w.Name)
			break
		}
	}

	// Should contain tips and gotchas
	if !strings.Contains(text, "Tips:") {
		t.Error("rendered text missing Tips section")
	}
	if !strings.Contains(text, "Gotchas:") {
		t.Error("rendered text missing Gotchas section")
	}
	if !strings.Contains(text, "Workflows:") {
		t.Error("rendered text missing Workflows section")
	}
}

// ─── JSON output validity ──────────────────────────────────────────────────

func TestGuideJSONValid(t *testing.T) {
	g := buildGuide()
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("json.Marshal(guide) failed: %v", err)
	}

	var decoded guideCatalog
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(guide) failed: %v", err)
	}

	if decoded.Version != g.Version {
		t.Errorf("JSON round-trip changed version: %q → %q", g.Version, decoded.Version)
	}
	if len(decoded.Commands) != len(g.Commands) {
		t.Errorf("JSON round-trip changed command count: %d → %d", len(g.Commands), len(decoded.Commands))
	}
	if len(decoded.Tips) != len(g.Tips) {
		t.Errorf("JSON round-trip changed tip count: %d → %d", len(g.Tips), len(decoded.Tips))
	}
	if len(decoded.Gotchas) != len(g.Gotchas) {
		t.Errorf("JSON round-trip changed gotcha count: %d → %d", len(g.Gotchas), len(decoded.Gotchas))
	}
	if len(decoded.Workflows) != len(g.Workflows) {
		t.Errorf("JSON round-trip changed workflow count: %d → %d", len(g.Workflows), len(decoded.Workflows))
	}
}
