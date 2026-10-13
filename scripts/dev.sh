#!/bin/bash
# Opens a shell in the dev container: Go, gh and Claude Code, with the
# project mounted at /workspace. Login state lives in Docker volumes.
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
  -e GIT_AUTHOR_NAME="$(git config user.name)" -e GIT_COMMITTER_NAME="$(git config user.name)" \
  -e GIT_AUTHOR_EMAIL="$(git config user.email)" -e GIT_COMMITTER_EMAIL="$(git config user.email)" \
  pokedex-dev "${@:-bash}"
