"""Tests for scripts/dora_report.py, run with a fake `gh` (no network).

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "dora_report.py"

# The fake gh answers `gh api <path>` from files named after the path in $FAKE_DIR
# (slashes and ? replaced by _).
FAKE_GH = """#!/usr/bin/env python3
import os, sys
path = sys.argv[2].replace('/', '_').replace('?', '_')
f = os.path.join(os.environ['FAKE_DIR'], path)
if not os.path.exists(f):
    sys.stderr.write('no fake for ' + sys.argv[2]); sys.exit(1)
sys.stdout.write(open(f).read())
"""

NOW = "2026-11-10T00:00:00Z"


def rel(tag, published, draft=False, pre=False):
    return {"tag_name": tag, "published_at": published, "draft": draft, "prerelease": pre}


def commits(*dates):
    return {"commits": [{"commit": {"committer": {"date": d}}} for d in dates]}


class DoraReportTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = Path(self.tmp.name)
        gh = self.dir / "gh"
        gh.write_text(FAKE_GH)
        gh.chmod(gh.stat().st_mode | stat.S_IEXEC)
        self.summary = self.dir / "summary.md"

    def fake(self, path, data):
        (self.dir / path.replace("/", "_").replace("?", "_")).write_text(json.dumps(data))

    def run_script(self, days="90"):
        env = {
            "PATH": f"{self.dir}:{os.environ['PATH']}",
            "FAKE_DIR": str(self.dir),
            "GITHUB_REPOSITORY": "o/r",
            "GITHUB_STEP_SUMMARY": str(self.summary),
            "DORA_NOW": NOW,
            "DORA_DAYS": days,
        }
        r = subprocess.run([sys.executable, "-I", str(SCRIPT)], env=env, capture_output=True, text=True)
        return r, self.summary.read_text() if self.summary.exists() else ""

    def releases(self, rels):
        self.fake("repos/o/r/releases?per_page=100", rels)

    def test_counts_published_releases_in_the_window_only(self):
        self.releases([
            rel("v0.3.0", "2026-11-05T00:00:00Z"),
            rel("v0.2.1", "2026-11-02T00:00:00Z"),
            rel("v0.2.0", "2026-06-01T00:00:00Z"),          # outside 90 days
            rel("v0.4.0-rc1", "2026-11-06T00:00:00Z", pre=True),
            rel("v0.5.0", "2026-11-07T00:00:00Z", draft=True),
        ])
        self.fake("repos/o/r/compare/v0.2.1...v0.3.0", commits("2026-11-04T00:00:00Z"))
        self.fake("repos/o/r/compare/v0.2.0...v0.2.1", commits("2026-11-01T00:00:00Z"))
        r, s = self.run_script()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("Releases in the last 90 days: 2", s)

    def test_lead_time_is_release_time_minus_commit_time(self):
        self.releases([rel("v2", "2026-11-05T00:00:00Z"), rel("v1", "2026-11-01T00:00:00Z")])
        # commits 1 day and 3 days before the release: median 2 days
        self.fake("repos/o/r/compare/v1...v2", commits("2026-11-04T00:00:00Z", "2026-11-02T00:00:00Z"))
        r, s = self.run_script()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("Median lead time: 2.0 days", s)

    def test_first_release_has_no_lead_time(self):
        self.releases([rel("v1", "2026-11-01T00:00:00Z")])
        r, s = self.run_script()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("Releases in the last 90 days: 1", s)
        self.assertIn("Median lead time: n/a", s)

    def test_no_releases_is_reported_not_an_error(self):
        self.releases([])
        r, s = self.run_script()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("Releases in the last 90 days: 0", s)

    def test_gh_failure_fails_the_run(self):
        r, _ = self.run_script()  # nothing faked: gh exits 1
        self.assertNotEqual(r.returncode, 0)

    def test_requires_a_repository(self):
        r = subprocess.run([sys.executable, "-I", str(SCRIPT)], env={"PATH": os.environ["PATH"]},
                           capture_output=True, text=True)
        self.assertEqual(r.returncode, 2)


if __name__ == "__main__":
    unittest.main()
