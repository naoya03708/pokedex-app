#!/bin/bash
# Verifies the app built by `go tool mygo build -platform darwin/arm64` and
# gives the disk image its release name. macOS only (codesign, lipo).
# Shared by release.yml and macos-build.yml so both check the same things.
#
# Writes DMG= and SUMS= (paths) to $GITHUB_ENV when that is set.
# GitHub turns spaces in asset names into dots, so the disk image gets a
# space-free name before the checksum is written.
set -euo pipefail
cd "$(dirname "$0")/.."

dir=build/darwin-arm64
app=$(ls -d "$dir"/*.app)
codesign --verify --deep --strict --verbose=2 "$app"
exe=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$app/Contents/Info.plist")
test "$(lipo -archs "$app/Contents/MacOS/$exe")" = "arm64"
# The licenses of the bundled dependencies must travel with the app.
notices="$app/Contents/Resources/THIRD_PARTY_NOTICES.txt"
test -s "$notices"
grep -q "MIT License" "$notices"
version=$(jq -r .version mygo.json)
dmg="$dir/Pokedex-$version-macos-arm64.dmg"
mv -- "$(ls "$dir"/*.dmg)" "$dmg"
(cd "$dir" && shasum -a 256 -- "$(basename "$dmg")" > SHA256SUMS.txt)
cat "$dir/SHA256SUMS.txt"
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "DMG=$dmg" >> "$GITHUB_ENV"
  echo "SUMS=$dir/SHA256SUMS.txt" >> "$GITHUB_ENV"
fi
