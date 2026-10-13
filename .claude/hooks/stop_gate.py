#!/usr/bin/env python3
"""Stop hook: the middle loop. Before Claude ends its turn with changed Go
code, runs scripts/check.sh (the CI "check" job). If it fails, Claude keeps
working on the fix, at most MAX_RETRIES times per change; after that the
turn ends and the failure is reported to the user.

State lives in .git/claude-stop-gate/ (never committed).
"""
import hashlib
import json
import os
import subprocess
import sys

MAX_RETRIES = 3
RELEVANT = (".go", "go.mod", "go.sum", ".github/workflows/ci.yml")


def git(root, *args):
    return subprocess.run(["git", *args], cwd=root, capture_output=True, text=True).stdout


def changed_files(root):
    """Relevant files that differ from origin/main, committed or not."""
    files = set(git(root, "diff", "--name-only", "origin/main").split())
    for line in git(root, "status", "--porcelain", "--untracked-files=all").splitlines():
        files.add(line[3:].split(" -> ")[-1])
    return sorted(f for f in files if f.endswith(RELEVANT))


def fingerprint(root, files):
    h = hashlib.sha256(git(root, "rev-parse", "HEAD").encode())
    for f in files:
        h.update(f.encode())
        try:
            with open(os.path.join(root, f), "rb") as fh:
                h.update(fh.read())
        except OSError:
            h.update(b"<deleted>")
    return h.hexdigest()


def main():
    data = json.load(sys.stdin)
    root = os.environ.get("CLAUDE_PROJECT_DIR") or data.get("cwd") or os.getcwd()
    files = changed_files(root)
    if not files:
        return

    state_dir = os.path.join(root, ".git", "claude-stop-gate")
    os.makedirs(state_dir, exist_ok=True)
    fp = fingerprint(root, files)
    ok_file = os.path.join(state_dir, "passed")
    tries_file = os.path.join(state_dir, "tries-" + data.get("session_id", "unknown"))
    if os.path.exists(ok_file) and open(ok_file).read() == fp:
        return

    result = subprocess.run([os.path.join(root, "scripts", "check.sh")], cwd=root,
                            capture_output=True, text=True)
    if result.returncode == 0:
        with open(ok_file, "w") as fh:
            fh.write(fp)
        if os.path.exists(tries_file):
            os.remove(tries_file)
        return

    tries = int(open(tries_file).read()) if os.path.exists(tries_file) else 0
    tail = "\n".join((result.stdout + result.stderr).splitlines()[-60:])
    if tries >= MAX_RETRIES:
        os.remove(tries_file)
        print(json.dumps({"systemMessage":
            f"scripts/check.sh still fails after {MAX_RETRIES} automatic fix attempts; "
            "stopping for your review."}))
        return
    with open(tries_file, "w") as fh:
        fh.write(str(tries + 1))
    print(f"scripts/check.sh failed (attempt {tries + 1}/{MAX_RETRIES}). "
          f"Fix it before finishing. If it cannot be fixed, explain why.\n\n{tail}",
          file=sys.stderr)
    sys.exit(2)


if __name__ == "__main__":
    main()
