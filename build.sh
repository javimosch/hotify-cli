#!/bin/bash
# Build hotify-cli.
#
# This used to pin an explicit FILES="main.go config.go ..." list, which silently
# rotted: seven source files added since it was last touched were missing from it, so
# the build failed on any symbol they defined. Building the package instead means new
# files are picked up automatically and can never drift out again.
#
# `set -e` matters too — the old script printed "Build complete!" even when the
# compiler had just failed.
set -euo pipefail

echo "Building hotify-cli..."

go build -o hotify-cli .
ls -lh hotify-cli

# Stripped build (-s -w drops the symbol table and DWARF): ~30% smaller, and it is
# this one that ships as the release asset.
go build -ldflags "-s -w" -o hotify-cli-optimized .
ls -lh hotify-cli-optimized

echo "Build complete!"
