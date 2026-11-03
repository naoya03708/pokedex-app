#!/bin/bash
# Downloads the assets of a published GitHub Release and checks them the way a
# user would: the files are all there, the disk image matches SHA256SUMS.txt,
# and every file has a build-provenance attestation signed by this
# repository's release workflow.
# Usage: scripts/verify-release.sh vX.Y.Z      (REPO=owner/name, default $GITHUB_REPOSITORY)
set -uo pipefail

tag=${1:-}
repo=${REPO:-${GITHUB_REPOSITORY:-}}
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "::error::not a release tag: '$tag'"; exit 2
fi
[ -n "$repo" ] || { echo "::error::set REPO=owner/name"; exit 2; }

version=${tag#v}
files=("Pokedex-$version-macos-arm64.dmg" "SHA256SUMS.txt" "Pokedex-$version-sbom.cdx.json")

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
gh release download "$tag" --repo "$repo" --dir "$dir" || {
  echo "::error::could not download the assets of $tag"; exit 1; }

for f in "${files[@]}"; do
  [ -s "$dir/$f" ] || { echo "::error::$tag has no asset $f"; exit 1; }
done

echo "== checksum"
if command -v sha256sum > /dev/null; then sum=(sha256sum -c); else sum=(shasum -a 256 -c); fi
(cd "$dir" && "${sum[@]}" SHA256SUMS.txt) || {
  echo "::error::checksum of the disk image does not match SHA256SUMS.txt"; exit 1; }

echo "== attestation"
for f in "${files[@]}"; do
  gh attestation verify "$dir/$f" --repo "$repo" \
    --signer-workflow "$repo/.github/workflows/release.yml" || {
    echo "::error::no valid attestation for $f"; exit 1; }
done
echo "$tag: all assets verified"
