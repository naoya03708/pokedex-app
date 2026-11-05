#!/usr/bin/env python3
"""Turns `go test -json` output into a readable log, PR annotations and a job summary.

Usage: scripts/test_report.py test-results.json
Always exits 0 when the file can be read: the caller keeps go test's own exit
status. Failures become `::error file=...,line=...::` workflow commands, so they
show on the changed lines of a pull request. Exit 2: unreadable input.
"""
import json
import os
import re
import sys
from collections import defaultdict

LOCATION = re.compile(r"^\s*(\S+\.go):(\d+):(?:\d+:)?\s*(.*)$")


def module_path():
    try:
        with open("go.mod", encoding="utf-8") as f:
            for line in f:
                if line.startswith("module "):
                    return line.split(None, 1)[1].strip()
    except OSError:
        pass
    return ""


def escape_data(s):
    return s.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")


def escape_prop(s):
    return escape_data(s).replace(":", "%3A").replace(",", "%2C")


def load(path):
    pkgs = {}
    tests = defaultdict(list)
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                e = json.loads(line)
            except ValueError:
                continue
            if not isinstance(e, dict) or "Package" not in e:
                continue
            p = pkgs.setdefault(e["Package"], {"result": None, "elapsed": 0.0, "output": [], "failed": []})
            t = e.get("Test")
            if e["Action"] == "output":
                (tests[(e["Package"], t)] if t else p["output"]).append(e.get("Output", ""))
            elif e["Action"] in ("pass", "fail", "skip"):
                if t:
                    if e["Action"] == "fail":
                        p["failed"].append(t)
                else:
                    p["result"] = e["Action"]
                    p["elapsed"] = e.get("Elapsed", 0.0)
    return pkgs, tests


def main(argv):
    if len(argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    try:
        pkgs, tests = load(argv[1])
    except OSError as e:
        print(e, file=sys.stderr)
        return 2
    mod = module_path()
    rows = []
    failures = []
    for name in sorted(pkgs):
        p = pkgs[name]
        short = name[len(mod) + 1:] if mod and name.startswith(mod + "/") else (name if name != mod else ".")
        status = {"pass": "ok", "skip": "no tests", None: "no result"}.get(p["result"], "FAIL")
        rows.append(f"| {short} | {status} | {p['elapsed']:.2f}s |")
        if p["result"] == "fail":
            if not p["failed"]:
                text = "".join(p["output"]).strip()
                print(f"::error::{escape_data(f'package {short} failed: {text}')}")
                print(text)
            for t in p["failed"]:
                out = "".join(tests[(name, t)])
                print(out, end="")
                failures.append((short, t))
                located = False
                for ln in out.splitlines():
                    m = LOCATION.match(ln)
                    if m:
                        located = True
                        path = m.group(1) if "/" in m.group(1) or short == "." else f"{short}/{m.group(1)}"
                        print(f"::error file={escape_prop(path)},line={m.group(2)}::{escape_data(f'{t}: {m.group(3)}')}")
                if not located:
                    print(f"::error::{escape_data(f'{t} failed in {short}')}")
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as f:
            f.write("## Test results\n\n| package | result | time |\n|---|---|---|\n" + "\n".join(rows) + "\n")
            if failures:
                f.write("\n**Failed tests**\n\n" + "\n".join(f"- `{p}` {t}" for p, t in failures) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
