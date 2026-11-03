"""Tests for scripts/verify-release.sh, run with a fake `gh` (no network).

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import hashlib
import os
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "verify-release.sh"

DMG = "Pokedex-0.3.0-macos-arm64.dmg"
SBOM = "Pokedex-0.3.0-sbom.cdx.json"
SUMS = "SHA256SUMS.txt"

# The fake gh copies the fixture folder for `release download`, and logs
# every `attestation verify`. It fails when the file name contains
# $FAKE_ATTEST_FAIL.
FAKE_GH = """#!/bin/bash
if [ "$1 $2" = "release download" ]; then
  dir=""
  while [ $# -gt 0 ]; do [ "$1" = "--dir" ] && dir="$2"; shift; done
  cp -R "$FAKE_FIXTURE"/. "$dir"/
  exit 0
fi
if [ "$1 $2" = "attestation verify" ]; then
  echo "$*" >> "$FAKE_LOG"
  if [ -n "${FAKE_ATTEST_FAIL:-}" ] && [[ "$3" == *"$FAKE_ATTEST_FAIL"* ]]; then exit 1; fi
  exit 0
fi
exit 99
"""


class VerifyReleaseTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        root = Path(self.tmp.name)
        self.bin = root / "bin"
        self.bin.mkdir()
        gh = self.bin / "gh"
        gh.write_text(FAKE_GH)
        gh.chmod(gh.stat().st_mode | stat.S_IEXEC)
        self.fixture = root / "fixture"
        self.fixture.mkdir()
        self.log = root / "attest.log"
        (self.fixture / DMG).write_bytes(b"pretend dmg")
        (self.fixture / SBOM).write_text("{}")
        self.write_sums()

    def write_sums(self):
        h = hashlib.sha256((self.fixture / DMG).read_bytes()).hexdigest()
        (self.fixture / SUMS).write_text(f"{h}  {DMG}\n")

    def run_script(self, tag="v0.3.0", **env):
        e = {
            "PATH": f"{self.bin}:/usr/bin:/bin",
            "FAKE_FIXTURE": str(self.fixture),
            "FAKE_LOG": str(self.log),
            "REPO": "owner/repo",
            **env,
        }
        return subprocess.run([str(SCRIPT), tag], env=e, capture_output=True, text=True)

    def attested(self):
        return self.log.read_text() if self.log.exists() else ""

    def test_passes_and_verifies_every_asset(self):
        r = self.run_script()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        log = self.attested()
        for name in (DMG, SBOM, SUMS):
            self.assertIn(name, log)
        self.assertIn("--repo owner/repo", log)
        self.assertIn("--signer-workflow owner/repo/.github/workflows/release.yml", log)

    def test_fails_when_the_dmg_does_not_match_the_checksum(self):
        (self.fixture / DMG).write_bytes(b"tampered")
        r = self.run_script()
        self.assertEqual(r.returncode, 1)
        self.assertIn("checksum", r.stdout + r.stderr)

    def test_fails_when_an_asset_is_missing(self):
        for name in (SBOM, DMG, SUMS):
            with self.subTest(missing=name):
                shutil.rmtree(self.fixture)
                self.fixture.mkdir()
                (self.fixture / DMG).write_bytes(b"pretend dmg")
                (self.fixture / SBOM).write_text("{}")
                self.write_sums()
                (self.fixture / name).unlink()
                r = self.run_script()
                self.assertEqual(r.returncode, 1)
                self.assertIn(name, r.stdout + r.stderr)

    def test_fails_when_one_attestation_is_missing(self):
        r = self.run_script(FAKE_ATTEST_FAIL="sbom")
        self.assertEqual(r.returncode, 1)
        self.assertIn("attestation", r.stdout + r.stderr)

    def test_rejects_a_bad_tag(self):
        for tag in ("0.3.0", "", "v1; rm -rf x"):
            with self.subTest(tag=tag):
                self.assertNotEqual(self.run_script(tag).returncode, 0)


if __name__ == "__main__":
    unittest.main()
