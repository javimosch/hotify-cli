package main

// basic-auth command — manage Traefik basicAuth credentials per app.
//
// Usage:
//   hotify-cli basic-auth --id <app> --action add    --user <u> --password <p>
//   hotify-cli basic-auth --id <app> --action add    --hash "user:$apr1$..."
//   hotify-cli basic-auth --id <app> --action remove --user <u>
//   hotify-cli basic-auth --id <app> --action list
//
// After add/remove the local config is updated. Run setup-traefik --id <app>
// (or apply via the remote daemon) to push the new dynamic.yml to Traefik.

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func handleBasicAuth() {
	cmd := flag.NewFlagSet("basic-auth", flag.ExitOnError)
	appID  := cmd.String("id",       "", "App ID (required)")
	action := cmd.String("action",   "", "Action: add | remove | list (required)")
	user   := cmd.String("user",     "", "Username for add/remove")
	pass   := cmd.String("password", "", "Plaintext password (hashed client-side with APR1-MD5)")
	hash   := cmd.String("hash",     "", "Pre-hashed htpasswd entry (e.g. user:$apr1$...). Skips hashing.")
	dryRun := cmd.Bool("dry-run",  false, "Preview changes without applying")
	target := cmd.String("target",   "", "Target name (uses default if not specified)")
	local  := cmd.Bool("local",    false, "Execute directly on local server")
	cmd.Parse(filterHumanFlag(os.Args[2:]))
	format := getOutputFormat()

	if *appID == "" || *action == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flags: --id and --action",
				Recoverable: false,
				Suggestions: []string{
					"hotify-cli basic-auth --id <app> --action add --user <u> --password <p>",
					"hotify-cli basic-auth --id <app> --action list",
					"hotify-cli basic-auth --id <app> --action remove --user <u>",
				},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if *local {
		// Local mode: execute directly on local server
		handleBasicAuthLocal(*appID, *action, *user, *pass, *hash, *dryRun, format)
		return
	}

	// Remote mode: use HTTP API
	targetObj, err := getActiveTarget(*target)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	handleBasicAuthRemote(*appID, *action, *user, *pass, *hash, targetObj, format)
}

// handleBasicAuthLocal executes basic auth operations locally.
func handleBasicAuthLocal(appID, action, user, pass, hash string, dryRun bool, format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		exitConfigError(format, err)
	}

	app := findApp(config, appID)
	if app == nil {
		exitAppNotFound(format, appID)
	}

	switch action {
	case "list":
		entries := make([]string, len(app.BasicAuth))
		copy(entries, app.BasicAuth)
		// Mask passwords in output: show only "user:***"
		masked := make([]string, len(entries))
		for i, e := range entries {
			parts := strings.SplitN(e, ":", 2)
			if len(parts) == 2 {
				masked[i] = parts[0] + ":***"
			} else {
				masked[i] = e
			}
		}
		printOutput(CommandResult{
			Version: Version, Success: true,
			Data: map[string]interface{}{
				"app_id":  appID,
				"count":   len(entries),
				"users":   masked,
				"enabled": len(entries) > 0,
			},
			Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
		}, format)

	case "add":
		var entry string
		if hash != "" {
			if !strings.Contains(hash, ":") {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{
						Code: ExitInvalidArgument, Type: "validation_error",
						Message:     "--hash must be in htpasswd format: user:$apr1$...",
						Recoverable: false,
					},
				}, format)
				os.Exit(ExitInvalidArgument)
			}
			entry = hash
		} else {
			if user == "" || pass == "" {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{
						Code: ExitInvalidArgument, Type: "validation_error",
						Message:     "add action requires --user and --password (or --hash for pre-hashed entry)",
						Recoverable: false,
						Suggestions: []string{
							"hotify-cli basic-auth --id <app> --action add --user alice --password secret",
							"hotify-cli basic-auth --id <app> --action add --hash 'alice:$apr1$...'",
						},
					},
				}, format)
				os.Exit(ExitInvalidArgument)
			}
			var hashErr error
			entry, hashErr = HtpasswdEntry(user, pass)
			if hashErr != nil {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{
						Code: ExitGenericFailure, Type: "hash_error",
						Message:     fmt.Sprintf("Failed to hash password: %v", hashErr),
						Recoverable: false,
					},
				}, format)
				os.Exit(ExitGenericFailure)
			}
		}

		entryUser := strings.SplitN(entry, ":", 2)[0]

		newAuth := make([]string, 0, len(app.BasicAuth)+1)
		replaced := false
		for _, e := range app.BasicAuth {
			if strings.SplitN(e, ":", 2)[0] == entryUser {
				replaced = true
				continue
			}
			newAuth = append(newAuth, e)
		}
		newAuth = append(newAuth, entry)

		if dryRun {
			verb := "added"
			if replaced { verb = "updated" }
			if format == OutputFormatText {
				fmt.Printf("📋 Dry-run — would %s user '%s' for app '%s'\n", verb, entryUser, appID)
				fmt.Printf("   Current users: %d\n", len(app.BasicAuth))
				fmt.Printf("   Resulting users: %d\n", len(newAuth))
				fmt.Println("   No changes were made.")
			} else {
				printOutput(CommandResult{
					Version: Version, Success: true,
					Data: map[string]interface{}{
						"app_id":  appID, "user": entryUser, "action": verb,
						"dry_run": true, "current_count": len(app.BasicAuth),
						"resulting_count": len(newAuth),
					},
				}, format)
			}
			return
		}

		for i := range config.Apps {
			if config.Apps[i].ID == appID {
				config.Apps[i].BasicAuth = newAuth
				break
			}
		}
		if err := saveConfig(config); err != nil {
			exitConfigError(format, err)
		}

		verb := "added"
		if replaced { verb = "updated" }
		printOutput(CommandResult{
			Version: Version, Success: true,
			Data: map[string]interface{}{
				"app_id":  appID, "user": entryUser, "action": verb, "count": len(newAuth),
			},
			Metadata: map[string]interface{}{
				"next_step": "run 'hotify-cli setup-traefik --id " + appID + "' to apply changes",
				"timestamp": time.Now().Unix(),
			},
		}, format)

	case "remove":
		if user == "" {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitInvalidArgument, Type: "validation_error",
					Message:     "remove action requires --user",
					Recoverable: false,
				},
			}, format)
			os.Exit(ExitInvalidArgument)
		}

		newAuth := make([]string, 0, len(app.BasicAuth))
		found := false
		for _, e := range app.BasicAuth {
			if strings.SplitN(e, ":", 2)[0] == user {
				found = true
				continue
			}
			newAuth = append(newAuth, e)
		}
		if !found {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitInvalidArgument, Type: "not_found",
					Message:     fmt.Sprintf("User '%s' not found in basic-auth for app '%s'", user, appID),
					Recoverable: false,
					Suggestions: []string{"hotify-cli basic-auth --id " + appID + " --action list"},
				},
			}, format)
			os.Exit(ExitInvalidArgument)
		}

		if dryRun {
			if format == OutputFormatText {
				fmt.Printf("📋 Dry-run — would remove user '%s' from app '%s'\n", user, appID)
				fmt.Printf("   Current users: %d → remaining: %d\n", len(app.BasicAuth), len(newAuth))
				fmt.Println("   No changes were made.")
			} else {
				printOutput(CommandResult{
					Version: Version, Success: true,
					Data: map[string]interface{}{
						"app_id": appID, "user": user, "action": "removed",
						"dry_run": true, "current_count": len(app.BasicAuth),
						"resulting_count": len(newAuth),
					},
				}, format)
			}
			return
		}

		for i := range config.Apps {
			if config.Apps[i].ID == appID {
				config.Apps[i].BasicAuth = newAuth
				break
			}
		}
		if err := saveConfig(config); err != nil {
			exitConfigError(format, err)
		}

		printOutput(CommandResult{
			Version: Version, Success: true,
			Data: map[string]interface{}{
				"app_id":          appID,
				"user":            user,
				"action":          "removed",
				"remaining_count": len(newAuth),
				"auth_enabled":    len(newAuth) > 0,
			},
			Metadata: map[string]interface{}{
				"next_step": "run 'hotify-cli setup-traefik --id " + appID + "' to apply changes",
				"timestamp": time.Now().Unix(),
			},
		}, format)

	default:
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     fmt.Sprintf("Unknown action '%s'. Valid actions: add, remove, list", action),
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}
}

// handleBasicAuthRemote executes basic auth operations via HTTP API.
func handleBasicAuthRemote(appID, action, user, pass, hash string, target *Remote, format OutputFormat) {
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}

	switch action {
	case "list":
		result, err := client.BasicAuthList(appID)
		if err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitGenericFailure, Type: "remote_error",
					Message:     fmt.Sprintf("Failed to list basic auth: %v", err),
					Recoverable: true,
				},
			}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{
			Version: Version, Success: true,
			Data:    result,
			Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
		}, format)

	case "add":
		if hash != "" {
			if err := client.BasicAuthAddHash(appID, hash); err != nil {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{
						Code: ExitGenericFailure, Type: "remote_error",
						Message:     fmt.Sprintf("Failed to add basic auth (hash): %v", err),
						Recoverable: true,
					},
				}, format)
				os.Exit(ExitGenericFailure)
			}
		} else {
			if user == "" || pass == "" {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{
						Code: ExitInvalidArgument, Type: "validation_error",
						Message:     "add action requires --user and --password (or --hash for pre-hashed entry)",
						Recoverable: false,
						Suggestions: []string{
							"hotify-cli basic-auth --id <app> --action add --user alice --password secret",
							"hotify-cli basic-auth --id <app> --action add --hash 'alice:$apr1$...'",
						},
					},
				}, format)
				os.Exit(ExitInvalidArgument)
			}
			if err := client.BasicAuthAdd(appID, user, pass); err != nil {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{
						Code: ExitGenericFailure, Type: "remote_error",
						Message:     fmt.Sprintf("Failed to add basic auth: %v", err),
						Recoverable: true,
					},
				}, format)
				os.Exit(ExitGenericFailure)
			}
		}
		printOutput(CommandResult{
			Version: Version, Success: true,
			Data: map[string]interface{}{
				"app_id":  appID,
				"target":  target.Name,
				"action":  "added",
			},
			Metadata: map[string]interface{}{
				"next_step": "run 'hotify-cli setup-traefik --id " + appID + " --target " + target.Name + "' to apply changes",
				"timestamp": time.Now().Unix(),
			},
		}, format)

	case "remove":
		if user == "" {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitInvalidArgument, Type: "validation_error",
					Message:     "remove action requires --user",
					Recoverable: false,
				},
			}, format)
			os.Exit(ExitInvalidArgument)
		}
		if err := client.BasicAuthRemove(appID, user); err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitGenericFailure, Type: "remote_error",
					Message:     fmt.Sprintf("Failed to remove basic auth: %v", err),
					Recoverable: true,
				},
			}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{
			Version: Version, Success: true,
			Data: map[string]interface{}{
				"app_id":  appID,
				"target":  target.Name,
				"user":    user,
				"action":  "removed",
			},
			Metadata: map[string]interface{}{
				"next_step": "run 'hotify-cli setup-traefik --id " + appID + " --target " + target.Name + "' to apply changes",
				"timestamp": time.Now().Unix(),
			},
		}, format)

	default:
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     fmt.Sprintf("Unknown action '%s'. Valid actions: add, remove, list", action),
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}
}