"""Tests for scripts/coverage_ratchet.py.

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "coverage_ratchet.py"

# a: 4 of 5 statements covered (80%); b: 1 of 2 (50%). Total 5 of 7.
PROFILE = """mode: atomic
m/a/a.go:1.1,2.2 3 1
m/a/a.go:3.1,4.2 1 0
m/a/b.go:1.1,2.2 1 5
m/b/b.go:1.1,2.2 1 1
m/b/b.go:3.1,4.2 1 0
"""


class CoverageRatchetTest(unittest.TestCase):
    def run_script(self, baseline, profile=PROFILE):
        d = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(d))
        (d / "coverage.out").write_text(profile)
        (d / "baseline.txt").write_text(baseline)
        r = subprocess.run(
            [sys.executable, "-I", str(SCRIPT), str(d / "coverage.out"), str(d / "baseline.txt")],
            capture_output=True, text=True)
        return r

    def test_passes_when_every_package_meets_its_baseline(self):
        r = self.run_script("m/a 80.0\nm/b 50.0\n")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("m/a", r.stdout)

    def test_fails_when_a_package_drops_below_its_baseline(self):
        r = self.run_script("m/a 90.0\nm/b 50.0\n")
        self.assertEqual(r.returncode, 1)
        self.assertIn("m/a", r.stdout)

    def test_small_drop_within_tolerance_passes(self):
        r = self.run_script("m/a 80.4\nm/b 50.0\n")
        self.assertEqual(r.returncode, 0, r.stdout)

    def test_missing_package_in_profile_fails(self):
        r = self.run_script("m/a 80.0\nm/gone 10.0\n")
        self.assertEqual(r.returncode, 1)
        self.assertIn("m/gone", r.stdout)

    def test_new_package_without_baseline_is_reported_but_passes(self):
        r = self.run_script("m/a 80.0\n")
        self.assertEqual(r.returncode, 0, r.stdout)
        self.assertIn("m/b", r.stdout)

    def test_hints_when_coverage_rose_well_above_baseline(self):
        r = self.run_script("m/a 60.0\nm/b 50.0\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("raise", r.stdout.lower())

    def test_ignores_comments_and_blank_lines_in_baseline(self):
        r = self.run_script("# note\n\nm/a 80.0\nm/b 50.0\n")
        self.assertEqual(r.returncode, 0, r.stdout)

    def test_bad_baseline_line_is_an_error(self):
        r = self.run_script("m/a eighty\n")
        self.assertEqual(r.returncode, 2)


if __name__ == "__main__":
    unittest.main()
