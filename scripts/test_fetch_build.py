"""Tests for scripts/fetch-build.sh, run with a fake `gh` on PATH (no network).

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

SCRIPT = Path(__file__).resolve().parent / "fetch-build.sh"

DMG_NAME = "Pokedex-0.1.0-macos-arm64.dmg"
DMG_BYTES = b"pretend this is a disk image"

# The fake gh answers the few calls the script makes. FAKE_* variables choose
# what it says; FAKE_FIXTURE is the folder `run download` copies from.
FAKE_GH = """#!/bin/bash
case "$1" in
  repo) echo "owner/repo" ;;
  api) echo "${FAKE_STATE:-false}" ;;
  run)
    case "$2" in
      list) echo "${FAKE_RUN-111}" ;;
      view) printf 'run:    https://example.test/runs/111\\nbranch: main\\ncommit: abc123\\nbuilt:  2026-10-18T00:00:00Z\\n' ;;
      download)
        dir=""
        while [ $# -gt 0 ]; do [ "$1" = "--dir" ] && dir="$2"; shift; done
        cp -R "$FAKE_FIXTURE"/. "$dir"/
        if [ -n "${FAKE_DL_FAIL:-}" ]; then rm -f "$dir"/SHA256SUMS.txt; exit 1; fi
        ;;
    esac ;;
esac
"""


class FetchBuildTest(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        # A throwaway project: the script works relative to its own folder.
        self.project = self.tmp / "project"
        (self.project / "scripts").mkdir(parents=True)
        shutil.copy(SCRIPT, self.project / "scripts" / "fetch-build.sh")
        self.bin = self.tmp / "bin"
        self.bin.mkdir()
        gh = self.bin / "gh"
        gh.write_text(FAKE_GH)
        gh.chmod(gh.stat().st_mode | stat.S_IEXEC)
        # What a successful artifact contains.
        self.fixture = self.tmp / "fixture"
        self.fixture.mkdir()
        (self.fixture / DMG_NAME).write_bytes(DMG_BYTES)
        digest = hashlib.sha256(DMG_BYTES).hexdigest()
        (self.fixture / "SHA256SUMS.txt").write_text(f"{digest}  {DMG_NAME}\n")

    def run_script(self, *args, **env):
        e = {**os.environ, "PATH": f"{self.bin}:{os.environ['PATH']}",
             "FAKE_FIXTURE": str(self.fixture), **env}
        return subprocess.run(
            ["bash", str(self.project / "scripts" / "fetch-build.sh"), *args],
            capture_output=True, text=True, env=e, timeout=60)

    def out(self, *parts):
        return self.project / "out-artifacts" / "builds" / Path(*parts)

    def test_downloads_verifies_and_records_the_build(self):
        r = self.run_script()

        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn(f"{DMG_NAME}: OK", r.stdout)
        self.assertTrue(self.out("run-111", DMG_NAME).is_file())
        info = self.out("run-111", "BUILD_INFO.txt").read_text()
        self.assertIn("commit: abc123", info)
        self.assertIn("branch: main", info)

    def test_an_existing_download_is_verified_again_even_after_the_artifact_expired(self):
        self.assertEqual(self.run_script().returncode, 0)

        # Now the artifact has expired and any download would fail.
        r = self.run_script("--run", "111", FAKE_STATE="true", FAKE_DL_FAIL="1")

        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("already exists", r.stdout)
        self.assertIn(f"{DMG_NAME}: OK", r.stdout)

    def test_an_expired_artifact_that_was_never_downloaded_says_so(self):
        r = self.run_script(FAKE_STATE="true")

        self.assertEqual(r.returncode, 1)
        self.assertIn("expired", r.stderr)
        self.assertIn("Run workflow", r.stderr)
        self.assertFalse(self.out("run-111").exists())

    def test_a_run_without_the_artifact_is_not_reported_as_expired(self):
        r = self.run_script("--run", "222", FAKE_STATE="missing")

        self.assertEqual(r.returncode, 1)
        self.assertIn("not a dry run", r.stderr)
        self.assertNotIn("expired", r.stderr)

    def test_an_interrupted_download_leaves_no_folder_that_looks_complete(self):
        r = self.run_script(FAKE_DL_FAIL="1")
        self.assertNotEqual(r.returncode, 0)
        self.assertFalse(self.out("run-111").exists(), "a half download must not be taken as a cache")

        # The next attempt starts clean and succeeds.
        r = self.run_script()

        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertTrue(self.out("run-111", DMG_NAME).is_file())

    def test_a_changed_disk_image_fails_the_checksum(self):
        self.assertEqual(self.run_script().returncode, 0)
        (self.out("run-111", DMG_NAME)).write_bytes(b"tampered")

        r = self.run_script("--run", "111")

        self.assertNotEqual(r.returncode, 0)
        self.assertIn("FAILED", r.stdout + r.stderr)

    def test_a_folder_without_checksums_is_called_incomplete(self):
        self.out("run-111").mkdir(parents=True)

        r = self.run_script("--run", "111")

        self.assertEqual(r.returncode, 1)
        self.assertIn("incomplete", r.stderr)

    def test_checksums_that_do_not_list_the_disk_image_are_rejected(self):
        (self.fixture / "SHA256SUMS.txt").write_text(
            hashlib.sha256(b"x").hexdigest() + "  other-file.txt\n")
        (self.fixture / "other-file.txt").write_bytes(b"x")

        r = self.run_script()

        self.assertEqual(r.returncode, 1)
        self.assertIn("no disk image listed", r.stderr)

    def test_no_successful_dry_run_says_how_to_start_one(self):
        r = self.run_script("--branch", "feat/x", FAKE_RUN="")

        self.assertEqual(r.returncode, 1)
        self.assertIn("feat/x", r.stderr)
        self.assertIn("Run workflow", r.stderr)

    def test_bad_arguments_are_refused_before_anything_runs(self):
        for args in (["--run", "abc"], ["--run", "1; rm -rf /"], ["--branch", "x", "--run", "1"],
                     ["--nope"], ["--branch"]):
            with self.subTest(args=args):
                r = self.run_script(*args)
                self.assertIn(r.returncode, (1, 2))
                self.assertNotEqual(r.returncode, 0)
                self.assertFalse((self.project / "out-artifacts").exists())

    def test_help_prints_the_usage_and_exits_cleanly(self):
        r = self.run_script("--help")

        self.assertEqual(r.returncode, 0)
        self.assertIn("Usage:", r.stdout)
        self.assertNotIn("set -euo", r.stdout)


if __name__ == "__main__":
    unittest.main()
