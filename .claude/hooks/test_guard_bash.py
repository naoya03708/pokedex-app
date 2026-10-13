"""Tests for guard_bash.py. Run: python3 -m unittest discover -s .claude/hooks"""
import unittest
from unittest import mock

import guard_bash

BLOCKED = [
    "git push origin main",
    "git push origin HEAD:main",
    "git push origin refs/heads/main",
    "cd /workspace && git push origin main",
    "git push --force-with-lease origin feat/x",
    "git push -f",
    "git push origin +feat/x",
    "git push --tags",
    "git push origin v0.1.0",
    "git branch -D main",
    "gh pr merge 3 --squash",
    "gh release create v0.1.0",
    "gh repo edit --delete-branch-on-merge",
    "gh api -X PATCH repos/o/r",
    "gh api repos/o/r/labels -f name=x",
    "cat <<'EOF'\nnotes\nEOF\ngh pr merge 1",
]

ALLOWED = [
    "git push -u origin feat/search",
    "git push origin main-fix",
    "gh pr create --title 'feat: x' --body-file b.md",
    "gh pr checks 3 --watch",
    "gh api repos/o/r/pulls",
    "go test ./...",
    "python3 - <<'EOF'\ntext = 'gh pr merge and git push origin main'\nEOF",
    "git commit -F - <<EOF\ndocs: mention gh release\nEOF",
]


class GuardTest(unittest.TestCase):
    def setUp(self):
        patcher = mock.patch.object(guard_bash, "current_branch", return_value="feat/x")
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_blocked(self):
        for command in BLOCKED:
            with self.subTest(command=command):
                self.assertIsNotNone(guard_bash.check(command))

    def test_allowed(self):
        for command in ALLOWED:
            with self.subTest(command=command):
                self.assertIsNone(guard_bash.check(command))

    def test_commit_and_push_on_main(self):
        with mock.patch.object(guard_bash, "current_branch", return_value="main"):
            self.assertIsNotNone(guard_bash.check("git commit -m x"))
            self.assertIsNotNone(guard_bash.check("git push"))
            self.assertIsNone(guard_bash.check("git switch -c feat/x"))


if __name__ == "__main__":
    unittest.main()
