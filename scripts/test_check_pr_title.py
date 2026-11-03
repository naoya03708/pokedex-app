"""Tests for scripts/check-pr-title.sh (PR titles become squash commits).

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import subprocess
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "check-pr-title.sh"


def run(title):
    return subprocess.run(
        [str(SCRIPT)], env={"PATH": "/usr/bin:/bin", "TITLE": title},
        capture_output=True, text=True,
    )


class CheckPRTitleTest(unittest.TestCase):
    def test_accepts_conventional_titles(self):
        for title in [
            "feat(pokeapi): add disk cache for name list",
            "fix: handle empty response",
            "ci(deps): Bump actions/upload-artifact from 7.0.1 to 7.0.2",
            "chore(deps): bump go.yaml.in/yaml/v3 from 3.0.5 to 3.0.6",
            "refactor(ui)!: rename the view",
            "chore(release): prepare v0.2.1",
            "docs: note a thing",
        ]:
            with self.subTest(title=title):
                self.assertEqual(run(title).returncode, 0, title)

    def test_rejects_other_titles(self):
        for title in [
            "Add disk cache",
            "feature: add disk cache",
            "feat add disk cache",
            "feat:no space",
            "feat(): empty scope",
            "feat: ",
            "Feat: capitalised type",
            "",
        ]:
            with self.subTest(title=title):
                r = run(title)
                self.assertEqual(r.returncode, 1, title)
                self.assertIn("Conventional Commits", r.stdout + r.stderr)

    def test_title_is_data_not_shell(self):
        r = run('fix: $(touch /tmp/pwned) `id`')
        self.assertEqual(r.returncode, 0)


if __name__ == "__main__":
    unittest.main()
