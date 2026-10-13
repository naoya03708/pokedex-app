#!/usr/bin/env python3
"""PreToolUse hook for Bash: refuses operations that docs/development.md
reserves for the user, so Claude can loop without asking for each command.

Exit 2 blocks the command and shows the reason to Claude.
"""
import json
import re
import subprocess
import sys

RULES = [
    (r"\bgit\s+push\b[^|;&]*\s(--force\b|--force-with-lease\b|-f\b|\+\S)",
     "force push is not allowed"),
    (r"\bgit\s+push\b[^|;&]*\s(\S*:)?(refs/heads/)?main(?=$|[\s;&|])",
     "pushing to main is not allowed; open a PR instead"),
    (r"\bgit\s+push\b[^|;&]*\s(--tags\b|--follow-tags\b|refs/tags/|v\d+\.\d+)",
     "pushing tags is the user's job (releases)"),
    (r"\bgit\s+(branch|push)\b[^|;&]*\s(-D|-d|--delete)\b[^|;&]*\smain(?=$|[\s;&|])",
     "deleting main is not allowed"),
    (r"\bgh\s+pr\s+merge\b", "merging PRs is the user's job"),
    (r"\bgh\s+release\b", "releases are the user's job"),
    (r"\bgh\s+repo\s+(edit|delete|rename|archive)\b",
     "changing repository settings is the user's job"),
    (r"\bgh\s+api\b[^|;&]*\s(-X|--method)\s*(?!GET\b)[A-Z]+",
     "gh api is read-only here (GET); writes go through the user"),
    (r"\bgh\s+api\b[^|;&]*\s(-f|-F|--field|--raw-field|--input)\b",
     "gh api with fields sends a write request; writes go through the user"),
]


def current_branch():
    try:
        return subprocess.run(["git", "branch", "--show-current"], capture_output=True,
                              text=True, timeout=5).stdout.strip()
    except Exception:
        return ""


HEREDOC = re.compile(r"<<-?\s*(['\"]?)(\w+)\1[^\n]*\n.*?^\s*\2\s*$", re.S | re.M)


def strip_heredocs(command):
    """Drops heredoc bodies, which are data (file contents, messages), not
    commands, so text that mentions e.g. "gh pr merge" is not blocked."""
    return HEREDOC.sub(lambda m: m.group(0).split("\n", 1)[0], command)


def check(command):
    command = strip_heredocs(command)
    for pattern, reason in RULES:
        if re.search(pattern, command):
            return reason
    # A bare "git push" or a commit while on main would land on main.
    if re.search(r"\bgit\s+(push|commit)\b", command) and current_branch() == "main":
        return ("on main: create a branch (feat/…, fix/…) before committing or pushing; "
                "run the branch switch as its own command")
    return None


def main():
    data = json.load(sys.stdin)
    command = data.get("tool_input", {}).get("command", "")
    reason = check(command)
    if reason:
        print(f"Blocked by .claude/hooks/guard_bash.py: {reason}. "
              "See docs/development.md (役割分担).", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
