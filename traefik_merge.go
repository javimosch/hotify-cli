package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ─── Traefik provider mode: single file OR watched directory ──────────────────
//
// Historically hotify assumed ONE dynamic file and regenerated it wholesale. That
// broke two things on a box where anything else also writes Traefik config:
//
//  1. other tools (e.g. hart-domain-sync) write per-domain routers into
//     /etc/traefik/dynamic.d/, which a `providers.file.filename` Traefik never reads;
//  2. anything hand-added to dynamic.yml was silently wiped on the next regenerate.
//
// So hotify now supports both provider layouts, and — see mergeForeignSections —
// preserves entries it did not author.

type TraefikMode string

const (
	TraefikModeFile      TraefikMode = "file"      // providers.file.filename
	TraefikModeDirectory TraefikMode = "directory" // providers.file.directory
)

const (
	traefikDynamicDir = "/etc/traefik/dynamic.d"
	// In directory mode hotify owns exactly this one file inside the directory. The
	// 00- prefix keeps it first in Traefik's load order, which makes hotify's
	// definitions the base that later files layer on top of.
	traefikDirOwnFile = traefikDynamicDir + "/00-hotify.yml"
)

// resolveTraefikMode reports the layout hotify should write.
//
// Order: explicit config wins; otherwise infer from the live traefik.yml so hotify
// never fights whatever the box is actually running; otherwise default to file mode
// (the historical behaviour, so upgrades are a no-op).
func resolveTraefikMode(config *Config) TraefikMode {
	if config != nil {
		switch TraefikMode(strings.ToLower(strings.TrimSpace(config.TraefikMode))) {
		case TraefikModeDirectory:
			return TraefikModeDirectory
		case TraefikModeFile:
			return TraefikModeFile
		}
	}
	return detectTraefikModeFromMainConfig()
}

// detectTraefikModeFromMainConfig reads traefik.yml and reports which provider form
// it uses. Unreadable or ambiguous => file mode.
func detectTraefikModeFromMainConfig() TraefikMode {
	data, err := os.ReadFile(traefikMain)
	if err != nil {
		return TraefikModeFile
	}
	inFile := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "file:":
			inFile = true
		case inFile && strings.HasPrefix(trimmed, "directory:"):
			return TraefikModeDirectory
		case inFile && strings.HasPrefix(trimmed, "filename:"):
			return TraefikModeFile
		// leaving the providers block entirely
		case trimmed != "" && !strings.HasPrefix(line, " ") && trimmed != "providers:":
			inFile = false
		}
	}
	return TraefikModeFile
}

// dynamicTargetPath is the file hotify writes its generated config to.
func dynamicTargetPath(config *Config) string {
	if resolveTraefikMode(config) == TraefikModeDirectory {
		return traefikDirOwnFile
	}
	return traefikDynamic
}

// providersBlockFor renders the providers stanza for traefik.yml in the given mode.
func providersBlockFor(mode TraefikMode, withDocker bool) string {
	fileProvider := fmt.Sprintf("  file:\n    filename: %s\n    watch: true", traefikDynamic)
	if mode == TraefikModeDirectory {
		fileProvider = fmt.Sprintf("  file:\n    directory: %s\n    watch: true", traefikDynamicDir)
	}
	if withDocker {
		return "providers:\n  docker:\n    endpoint: \"unix:///var/run/docker.sock\"\n    exposedByDefault: false\n" + fileProvider
	}
	return "providers:\n" + fileProvider
}

// ─── Preserving config hotify did not author ──────────────────────────────────

// yamlBlocks maps a top-level key inside an http.<section> to its raw YAML text
// (the key line plus everything indented under it). Working on raw text rather
// than a parsed tree keeps foreign entries byte-identical, including comments.
type yamlBlocks map[string]string

// splitHTTPSections parses a Traefik dynamic file into
// section name ("routers"/"services"/"middlewares") -> key -> raw block.
//
// Deliberately text-based: a round-trip through a YAML marshaller would reformat
// and drop the comments of config another tool owns.
func splitHTTPSections(data string) map[string]yamlBlocks {
	out := map[string]yamlBlocks{}
	var section string
	var key string
	var buf []string

	flush := func() {
		if section != "" && key != "" && len(buf) > 0 {
			if out[section] == nil {
				out[section] = yamlBlocks{}
			}
			out[section][key] = strings.TrimRight(strings.Join(buf, "\n"), " \n") + "\n"
		}
		key, buf = "", nil
	}

	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		// "  routers:" / "  services:" / "  middlewares:" — a new section
		if indent == 2 && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
			flush()
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		// "http:" or any other column-0 key ends the current section
		if indent == 0 && trimmed != "" {
			flush()
			if trimmed != "http:" {
				section = ""
			}
			continue
		}
		if section == "" {
			continue
		}
		// "    name:" — a new entry within the section
		if indent == 4 && strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
			flush()
			key = strings.TrimSuffix(trimmed, ":")
			buf = []string{line}
			continue
		}
		// continuation of the current entry (or a blank line inside it)
		if key != "" && (indent > 4 || trimmed == "") {
			buf = append(buf, line)
		}
	}
	flush()
	return out
}

// hotifyOwnedNames lists every router/service/middleware name hotify generates for
// this config. Anything else found in the target file is FOREIGN and preserved.
func hotifyOwnedNames(config *Config) map[string]bool {
	owned := map[string]bool{}
	if config == nil {
		return owned
	}
	for _, app := range config.Apps {
		owned[app.ID] = true
		owned[app.ID+"-basic-auth"] = true
		owned[app.ID+"-rate-limit"] = true
		owned[app.ID+"-addprefix"] = true
	}
	return owned
}

// mergeForeignSections re-injects entries present in the existing target file that
// hotify does not author, into freshly generated YAML.
//
// Precedence is explicit: on a name collision HOTIFY WINS — its config.json is the
// source of truth for anything it manages, so a stale foreign copy of the same
// router must not shadow it. Everything else is carried through untouched, which is
// what lets another tool (hart-domain-sync, or a hand-written middleware) coexist
// with `setup-traefik` instead of being erased by it.
func mergeForeignSections(generated string, existingPath string, config *Config) (string, []string) {
	existingData, err := os.ReadFile(existingPath)
	if err != nil {
		return generated, nil // nothing to preserve
	}
	existing := splitHTTPSections(string(existingData))
	if len(existing) == 0 {
		return generated, nil
	}
	gen := splitHTTPSections(generated)
	owned := hotifyOwnedNames(config)

	var preserved []string
	// section order is fixed so output is deterministic
	for _, section := range []string{"routers", "services", "middlewares"} {
		for name, block := range existing[section] {
			if owned[name] {
				continue // hotify authors this one — its version wins
			}
			if gen[section] != nil && gen[section][name] != "" {
				continue // already present in the generated output
			}
			if gen[section] == nil {
				gen[section] = yamlBlocks{}
			}
			gen[section][name] = block
			preserved = append(preserved, section+"/"+name)
		}
	}
	sort.Strings(preserved)
	if len(preserved) == 0 {
		return generated, nil
	}
	return renderHTTPSections(gen), preserved
}

// renderHTTPSections rebuilds a dynamic file from section -> key -> raw block,
// keys sorted so repeated runs produce identical bytes (no spurious diffs/reloads).
func renderHTTPSections(sections map[string]yamlBlocks) string {
	var sb strings.Builder
	sb.WriteString("http:\n")
	for _, section := range []string{"routers", "services", "middlewares"} {
		blocks := sections[section]
		if len(blocks) == 0 {
			continue
		}
		sb.WriteString("  " + section + ":\n")
		names := make([]string, 0, len(blocks))
		for n := range blocks {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			body := strings.TrimRight(blocks[n], "\n")
			sb.WriteString(body)
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}
