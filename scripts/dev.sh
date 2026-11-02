#!/bin/bash
# Opens a shell in the dev container: Go, gh and Claude Code, with the
# project mounted at /workspace. Login state lives in Docker volumes.
# Git settings (a safe /workspace and the GitHub noreply address) are passed as
# GIT_CONFIG_* variables, like devcontainer.json does: a personal ~/.gitconfig
# copied into the container cannot override them. Keep the two in step;
# scripts/test_devcontainer.py checks it.
# Usage: scripts/dev.sh [command...]   (default: bash)
set -e
cd "$(dirname "$0")/.."
docker build -q -t pokedex-dev .devcontainer >/dev/null
TTY=""
[ -t 0 ] && TTY="-t"
exec docker run --rm -i $TTY \
  -v "$PWD":/workspace -w /workspace \
  -v pokedex-app-gopath:/go \
  -v pokedex-app-gocache:/home/dev/.cache/go-build \
  -v pokedex-app-gh:/home/dev/.config/gh \
  -v pokedex-app-claude:/home/dev/.claude-config \
  -e GOFLAGS=-buildvcs=false \
  -e GIT_CONFIG_COUNT=2 \
  -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0=/workspace \
  -e GIT_CONFIG_KEY_1=user.email -e GIT_CONFIG_VALUE_1=172844198+naoya03708@users.noreply.github.com \
  -e GIT_AUTHOR_NAME="$(git config user.name)" -e GIT_COMMITTER_NAME="$(git config user.name)" \
  pokedex-dev "${@:-bash}"
