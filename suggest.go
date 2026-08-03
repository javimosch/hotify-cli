package main

import "strings"

// knownCommands is the canonical list of top-level hotify-cli commands.
// Keep in sync with the switch in main.go.
var knownCommands = []string{
	"init",
	"setup",
	"add",
	"edit",
	"remove",
	"list",
	"start",
	"stop",
	"restart",
	"status",
	"pause",
	"resume",
	"deploy",
	"setup-dns",
	"setup-traefik",
	"setup-routing",
	"docker",
	"compose",
	"deploy-compose",
	"compose-sync",
	"compose-copy-dir",
	"volume-init",
	"setup-compose",
	"prune",
	"traefik-system",
	"basic-auth",
	"import-traefik",
	"auth",
	"targets",
	"api-keys",
	"guide",
	"version",
	"help",
}

// suggestCommand returns the nearest known command to input, or "" when nothing
// is close enough. A unique prefix match (≥2 runes) wins outright; otherwise
// the nearest by Levenshtein distance is returned if it falls within a
// length-aware threshold: max(1, len(input)/3).
func suggestCommand(input string) string {
	if input == "" {
		return ""
	}
	runes := []rune(input)

	// Prefix match wins outright when exactly one command starts with the input.
	if len(runes) >= 2 {
		var prefixMatches []string
		for _, cmd := range knownCommands {
			if strings.HasPrefix(cmd, input) {
				prefixMatches = append(prefixMatches, cmd)
			}
		}
		if len(prefixMatches) == 1 {
			return prefixMatches[0]
		}
	}

	// Levenshtein-based fallback with a length-aware threshold.
	maxDist := len(runes) / 3
	if maxDist < 1 {
		maxDist = 1
	}

	best := ""
	bestDist := maxDist + 1
	for _, cmd := range knownCommands {
		d := levenshtein(input, cmd)
		if d < bestDist {
			bestDist = d
			best = cmd
		}
	}
	if bestDist <= maxDist {
		return best
	}
	return ""
}

// levenshtein returns the edit distance between a and b (rune-aware).
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = minInt(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func minInt(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
