"""Tests for scripts/smoke-app.sh, run with fake executables (no GUI).

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import stat
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "smoke-app.sh"


class SmokeAppTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)

    def exe(self, body):
        p = Path(self.tmp.name) / "app"
        p.write_text("#!/bin/bash\n" + body)
        p.chmod(p.stat().st_mode | stat.S_IEXEC)
        return str(p)

    def run_script(self, *args):
        return subprocess.run([str(SCRIPT), *args], capture_output=True, text=True, timeout=30)

    def test_passes_when_the_app_stays_alive(self):
        r = self.run_script(self.exe("sleep 30"), "2")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_stops_the_app_it_started(self):
        marker = Path(self.tmp.name) / "alive"
        exe = self.exe(f"trap 'rm -f {marker}; exit 0' TERM\ntouch {marker}\nwhile true; do sleep 0.2; done")
        self.assertEqual(self.run_script(exe, "2").returncode, 0)
        time.sleep(0.5)
        self.assertFalse(marker.exists(), "the app was left running")

    def test_fails_when_the_app_exits_at_once(self):
        r = self.run_script(self.exe("echo boom >&2; exit 3"), "3")
        self.assertEqual(r.returncode, 1)
        self.assertIn("status 3", r.stdout + r.stderr)
        self.assertIn("boom", r.stdout + r.stderr)

    def test_fails_when_the_app_exits_cleanly_too_early(self):
        self.assertEqual(self.run_script(self.exe("exit 0"), "3").returncode, 1)

    def test_fails_when_the_app_crashes_later(self):
        self.assertEqual(self.run_script(self.exe("sleep 1; kill -SEGV $$"), "4").returncode, 1)

    def test_fails_without_an_executable(self):
        self.assertNotEqual(self.run_script().returncode, 0)
        self.assertNotEqual(self.run_script("/nonexistent/app", "1").returncode, 0)


if __name__ == "__main__":
    unittest.main()
