"""Tests for scripts/notify-ci-failure.sh, run with a fake `gh` (no network).

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "notify-ci-failure.sh"

# The fake gh appends every call to $FAKE_LOG, one record per call (the body
# spans lines, so records end with \x1e), and answers
# `issue list` with $FAKE_LIST (the
# "number<TAB>title" lines that gh prints for the --jq filter).
FAKE_GH = """#!/bin/bash
printf '%s\\x1e' "$*" >> "$FAKE_LOG"
if [ "$1 $2" = "issue list" ]; then echo "${FAKE_LIST:-}"; fi
"""

ENV = {
    "WORKFLOW_NAME": "CodeQL",
    "BRANCH": "main",
    "EVENT": "schedule",
    "SHA": "abc1234def",
    "RUN_URL": "https://example.test/runs/1",
}


class NotifyCIFailureTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        d = Path(self.tmp.name)
        gh = d / "gh"
        gh.write_text(FAKE_GH)
        gh.chmod(gh.stat().st_mode | stat.S_IEXEC)
        self.log = d / "calls.log"

    def run_script(self, list_lines="", **over):
        env = {
            "PATH": f"{self.tmp.name}:/usr/bin:/bin",
            "FAKE_LOG": str(self.log),
            "FAKE_LIST": list_lines,
            **ENV,
            **over,
        }
        r = subprocess.run([str(SCRIPT)], env=env, capture_output=True, text=True)
        calls = self.log.read_text().split("\x1e")[:-1] if self.log.exists() else []
        return r, calls

    def test_creates_an_issue_when_none_is_open(self):
        r, calls = self.run_script()
        self.assertEqual(r.returncode, 0, r.stderr)
        creates = [c for c in calls if c.startswith("issue create")]
        self.assertEqual(len(creates), 1, calls)
        self.assertIn("CI failure: CodeQL on main", creates[0])
        self.assertIn("--label ci-failure", creates[0])
        self.assertIn("https://example.test/runs/1", creates[0])
        self.assertFalse([c for c in calls if c.startswith("issue comment")])

    def test_comments_on_the_open_issue_instead_of_a_duplicate(self):
        existing = "7\tCI failure: CodeQL on main"
        r, calls = self.run_script(existing)
        self.assertEqual(r.returncode, 0, r.stderr)
        comments = [c for c in calls if c.startswith("issue comment")]
        self.assertEqual(len(comments), 1, calls)
        self.assertIn("issue comment 7", comments[0])
        self.assertFalse([c for c in calls if c.startswith("issue create")])

    def test_other_workflow_or_branch_gets_its_own_issue(self):
        other = "7\tCI failure: CI on main\n8\tCI failure: CodeQL on v1.0.0"
        _, calls = self.run_script(other)
        self.assertTrue([c for c in calls if c.startswith("issue create")], calls)

    def test_names_are_data_not_shell(self):
        r, calls = self.run_script(WORKFLOW_NAME="x $(touch /tmp/pwned) `id`")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("$(touch /tmp/pwned)", "\n".join(calls))

    def test_fails_without_required_input(self):
        r, _ = self.run_script(WORKFLOW_NAME="")
        self.assertNotEqual(r.returncode, 0)


if __name__ == "__main__":
    unittest.main()
