#!/bin/bash
# Build hotify-cli
echo "Building hotify-cli..."

FILES="main.go config.go daemon.go server.go server_compose.go server_traefik.go cloudflare.go traefik.go security.go permissions.go audit.go auth.go api_keys.go targets.go deploy.go deploy_client.go traefik_system.go dns_traefik_cli.go apps.go process.go local_ops.go remote_ops.go helpers.go docker_ops.go docker_cli.go compose_cli.go compose_deploy_client.go compose_deploy_cmds1.go compose_deploy_cmds2.go basic_auth.go server_apps_remote.go"

# Build default version
go build -o hotify-cli $FILES
ls -lh hotify-cli

# Build optimized version
go build -ldflags "-s -w" -o hotify-cli-optimized $FILES
ls -lh hotify-cli-optimized

echo "Build complete!"
