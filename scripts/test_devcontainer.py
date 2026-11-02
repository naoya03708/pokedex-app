"""The dev container must configure git the same way however it is started.

VS Code uses .devcontainer/devcontainer.json; scripts/dev.sh runs docker
directly. Git settings go in through GIT_CONFIG_* environment variables,
which are not overwritten when VS Code copies the host's ~/.gitconfig into
the container (that copy once brought a personal e-mail address back).

    python3 -m unittest discover -s scripts -p 'test_*.py'
"""
import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
NOREPLY = re.compile(r"^\d+\+[A-Za-z0-9-]+@users\.noreply\.github\.com$")


def git_config_from_env(env):
    """Turn GIT_CONFIG_COUNT/KEY_n/VALUE_n into a {key: value} dict."""
    count = int(env.get("GIT_CONFIG_COUNT", "0"))
    return {env[f"GIT_CONFIG_KEY_{i}"]: env[f"GIT_CONFIG_VALUE_{i}"] for i in range(count)}


def devcontainer():
    return json.loads((ROOT / ".devcontainer" / "devcontainer.json").read_text())


def dev_sh_env():
    """The -e NAME=value flags of scripts/dev.sh that set GIT_CONFIG_*."""
    text = (ROOT / "scripts" / "dev.sh").read_text()
    return dict(re.findall(r"-e (GIT_CONFIG_\w+)=(\S+)", text))


class DevContainerGitConfigTest(unittest.TestCase):
    def test_devcontainer_marks_the_workspace_as_a_safe_directory(self):
        cfg = devcontainer()
        got = git_config_from_env(cfg["containerEnv"])

        self.assertEqual(got.get("safe.directory"), cfg["workspaceFolder"])

    def test_devcontainer_commits_with_a_github_noreply_address(self):
        got = git_config_from_env(devcontainer()["containerEnv"])

        self.assertRegex(got.get("user.email", ""), NOREPLY)

    def test_dev_sh_sets_the_same_git_config_as_the_devcontainer(self):
        from_dev_sh = git_config_from_env(dev_sh_env())
        from_devcontainer = git_config_from_env(devcontainer()["containerEnv"])

        self.assertTrue(from_dev_sh, "scripts/dev.sh sets no git config")
        self.assertEqual(from_dev_sh, from_devcontainer)

    def test_no_personal_address_is_written_in_the_container_setup(self):
        for path in [ROOT / ".devcontainer" / "devcontainer.json", ROOT / "scripts" / "dev.sh"]:
            for addr in re.findall(r"[\w.+-]+@[\w.-]+\.\w+", path.read_text()):
                with self.subTest(path=path.name, addr=addr):
                    self.assertRegex(addr, r"@users\.noreply\.github\.com$")


if __name__ == "__main__":
    unittest.main()
