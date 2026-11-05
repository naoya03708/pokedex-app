"""Tests for scripts/test_report.py.

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "test_report.py"
MOD = "example.com/m"


def ev(action, pkg, test=None, output=None, elapsed=None):
    e = {"Action": action, "Package": f"{MOD}/{pkg}"}
    if test:
        e["Test"] = test
    if output is not None:
        e["Output"] = output
    if elapsed is not None:
        e["Elapsed"] = elapsed
    return json.dumps(e)


PASSING = [
    ev("run", "a", "TestOne"),
    ev("pass", "a", "TestOne", elapsed=0.1),
    ev("pass", "a", elapsed=0.3),
]
FAILING = [
    ev("run", "b", "TestBad"),
    ev("output", "b", "TestBad", "=== RUN   TestBad\n"),
    ev("output", "b", "TestBad", "    b_test.go:12: want 1, got 2\n"),
    ev("output", "b", "TestBad", "--- FAIL: TestBad (0.00s)\n"),
    ev("fail", "b", "TestBad", elapsed=0.0),
    ev("output", "b", None, "FAIL\n"),
    ev("fail", "b", elapsed=0.2),
]


class TestReportTest(unittest.TestCase):
    def run_script(self, lines):
        d = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(d))
        (d / "go.mod").write_text(f"module {MOD}\n\ngo 1.27\n")
        (d / "results.json").write_text("\n".join(lines) + "\n")
        summary = d / "summary.md"
        env = {"PATH": os.environ["PATH"], "GITHUB_STEP_SUMMARY": str(summary)}
        r = subprocess.run([sys.executable, "-I", str(SCRIPT), str(d / "results.json")],
                           cwd=d, env=env, capture_output=True, text=True)
        return r, summary.read_text() if summary.exists() else ""

    def test_passing_run_has_no_annotation(self):
        r, summary = self.run_script(PASSING)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertNotIn("::error", r.stdout)
        self.assertIn("a", summary)
        self.assertIn("ok", summary)

    def test_failing_test_becomes_an_annotation_with_file_and_line(self):
        r, _ = self.run_script(PASSING + FAILING)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("::error file=b/b_test.go,line=12::", r.stdout)
        self.assertIn("TestBad", r.stdout)
        self.assertIn("want 1, got 2", r.stdout)

    def test_summary_lists_failed_tests(self):
        _, summary = self.run_script(PASSING + FAILING)
        self.assertIn("TestBad", summary)
        self.assertIn("FAIL", summary)

    def test_failed_package_without_tests_shows_its_output(self):
        # a build error: the package fails and has output but no failed test
        lines = [ev("output", "c", None, "c.go:3:1: syntax error\n"), ev("fail", "c", elapsed=0.0)]
        r, summary = self.run_script(lines)
        self.assertIn("::error", r.stdout)
        self.assertIn("syntax error", r.stdout)
        self.assertIn("c", summary)

    def test_names_and_messages_cannot_inject_commands(self):
        lines = [
            ev("output", "b", "TestX", "    b_test.go:1: %0A::error::injected\n"),
            ev("fail", "b", "TestX"), ev("fail", "b"),
        ]
        r, _ = self.run_script(lines)
        self.assertNotIn("\n::error::injected", r.stdout)

    def test_non_json_lines_are_ignored(self):
        r, _ = self.run_script(["# example.com/m/x", "not json"] + PASSING)
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_missing_file_is_an_error(self):
        r = subprocess.run([sys.executable, "-I", str(SCRIPT), "/nonexistent"],
                           capture_output=True, text=True)
        self.assertEqual(r.returncode, 2)


if __name__ == "__main__":
    unittest.main()
