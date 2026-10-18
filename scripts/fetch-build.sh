#!/bin/bash
# Downloads the macOS build of a Release workflow dry run into out-artifacts/,
# without a browser. Browsers (Chrome in particular) block or warn about an
# unsigned dmg, and a file saved by gh carries no macOS quarantine flag.
#
# The project folder is shared with the Mac, so the dmg can be opened from
# out-artifacts/ in Finder right after this script ends. The folder is
# ignored by git (.gitignore: /out*).
#
# Usage:
#   scripts/fetch-build.sh                     # newest successful dry run
#   scripts/fetch-build.sh --branch feat/x     # newest successful dry run of a branch
#   scripts/fetch-build.sh --run 39088009828   # one run
#
# Start a dry run from GitHub: Actions > Release > Run workflow, choosing the
# branch. Its artifact expires after one day. Published releases are
# downloaded with `gh release download` by hand.
set -euo pipefail
cd "$(dirname "$0")/.."

ARTIFACT=macos-arm64-dry-run
OUT=out-artifacts
branch=""
run=""

while [ $# -gt 0 ]; do
  case "$1" in
    --branch) branch=${2:?--branch needs a name}; shift 2 ;;
    --run)    run=${2:?--run needs a run id};      shift 2 ;;
    -h|--help) sed -n '2,/^set -euo/p' "$0" | sed '$d; s/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1 (see --help)" >&2; exit 2 ;;
  esac
done

if [ -n "$branch" ] && [ -n "$run" ]; then
  echo "give --branch or --run, not both" >&2; exit 2
fi
case "$run" in *[!0-9]*) echo "--run takes a number, got: $run" >&2; exit 2 ;; esac

if [ -z "$run" ]; then
  args=(--workflow Release --event workflow_dispatch --status success --limit 1 --json databaseId -q '.[0].databaseId')
  [ -n "$branch" ] && args+=(--branch "$branch")
  run=$(gh run list "${args[@]}")
  if [ -z "$run" ] || [ "$run" = null ]; then
    echo "no successful dry run found${branch:+ for branch $branch}." >&2
    echo "Start one: Actions > Release > Run workflow${branch:+ (branch $branch)}." >&2
    exit 1
  fi
fi

dir="$OUT/run-$run"
if [ -d "$dir" ]; then
  # Already fetched: verify it. (This also works after the artifact expired.)
  echo "$dir already exists; verifying it instead of downloading again."
else
  # Is the artifact still there? (It expires after one day.)
  repo=$(gh repo view --json nameWithOwner -q .nameWithOwner)
  state=$(gh api "repos/$repo/actions/runs/$run/artifacts" \
    --jq "[.artifacts[] | select(.name == \"$ARTIFACT\")] | if length == 0 then \"missing\" else (.[0].expired | tostring) end")
  if [ "$state" != false ]; then
    if [ "$state" = true ]; then why="it has expired (they last one day)"; else why="the run is not a dry run, or it never made one"; fi
    echo "run $run has no downloadable $ARTIFACT artifact: $why." >&2
    echo "Start a new dry run: Actions > Release > Run workflow." >&2
    exit 1
  fi
  # Download beside the final place and move it in only when it is complete,
  # so an interrupted download never leaves a half-filled folder behind.
  tmp="$dir.partial"
  rm -rf "$tmp"
  mkdir -p "$tmp"
  gh run download "$run" --name "$ARTIFACT" --dir "$tmp"
  mv "$tmp" "$dir"
fi
if [ ! -f "$dir/BUILD_INFO.txt" ]; then
  gh run view "$run" --json headBranch,headSha,createdAt,url \
    -q '"run:    \(.url)\nbranch: \(.headBranch)\ncommit: \(.headSha)\nbuilt:  \(.createdAt)"' > "$dir/BUILD_INFO.txt"
fi

echo "== checksum"
(
  cd "$dir"
  if [ ! -f SHA256SUMS.txt ]; then
    echo "$dir has no SHA256SUMS.txt: it is incomplete. Delete it and run again." >&2
    exit 1
  fi
  dmg=$(ls Pokedex-*-macos-arm64.dmg 2>/dev/null | head -n 1 || true)
  if [ -z "$dmg" ] || ! grep -qF -- "$dmg" SHA256SUMS.txt; then
    echo "no disk image listed in SHA256SUMS.txt: the folder is not a complete build." >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null; then sha256sum -c SHA256SUMS.txt; else shasum -a 256 -c SHA256SUMS.txt; fi
)

echo "== build"
cat "$dir/BUILD_INFO.txt"
echo
echo "On the Mac, open $dir/ in the project folder (Finder)."
ls "$dir"
