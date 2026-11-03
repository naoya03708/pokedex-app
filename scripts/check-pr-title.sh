#!/bin/bash
# Checks that the pull request title (in $TITLE) is a Conventional Commit.
# The PR is squash-merged, so the title becomes the commit message on main.
# See "コミットメッセージ" in docs/development.md.
set -u

pattern='^(feat|fix|docs|test|refactor|ci|chore|perf|build|revert)(\([a-z0-9._/-]+\))?!?: [^ ].*$'

if [[ "${TITLE-}" =~ $pattern ]]; then
  exit 0
fi

echo "::error::PR title is not a Conventional Commit: '${TITLE-}'"
echo "Use: <type>(<scope>): <summary>, e.g. 'feat(pokeapi): add disk cache for name list'."
echo "Types: feat fix docs test refactor ci chore perf build revert. See Conventional Commits (docs/development.md)."
exit 1
