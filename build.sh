#!/bin/bash
# Build hotify-cli
echo "Building hotify-cli..."

# Build default version
go build -o hotify-cli main.go config.go daemon.go server.go cloudflare.go traefik.go security.go permissions.go audit.go auth.go api_keys.go targets.go deploy.go traefik_system.go
ls -lh hotify-cli

# Build optimized version
go build -ldflags "-s -w" -o hotify-cli-optimized main.go config.go daemon.go server.go cloudflare.go traefik.go security.go permissions.go audit.go auth.go api_keys.go targets.go deploy.go traefik_system.go
ls -lh hotify-cli-optimized

echo "Build complete!"
