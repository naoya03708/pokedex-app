#!/usr/bin/env python3
"""PostToolUse hook for Edit/Write: the inner loop. Formats an edited Go file
and vets its package, and reports problems back to Claude at once.

Exit 2 shows stderr to Claude (the edit itself has already happened).
"""
import json
import os
import subprocess
import sys


def run(args, cwd):
    return subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=120)


def main():
    data = json.load(sys.stdin)
    path = data.get("tool_input", {}).get("file_path", "")
    if not path.endswith(".go") or not os.path.isfile(path):
        return
    root = os.environ.get("CLAUDE_PROJECT_DIR") or data.get("cwd") or os.getcwd()

    fmt = run(["gofmt", "-l", "-w", path], root)
    if fmt.returncode != 0:
        print(f"gofmt failed for {path}:\n{fmt.stderr}", file=sys.stderr)
        sys.exit(2)
    notes = []
    if fmt.stdout.strip():
        notes.append(f"gofmt reformatted {path}; re-read it before the next edit.")

    pkg = "./" + os.path.relpath(os.path.dirname(os.path.abspath(path)), root)
    vet = run(["go", "vet", pkg], root)
    if vet.returncode != 0:
        print("\n".join(notes + [f"go vet {pkg}:", vet.stderr.strip()]), file=sys.stderr)
        sys.exit(2)
    if notes:
        print("\n".join(notes), file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
