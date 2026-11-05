#!/usr/bin/env python3
"""Coverage ratchet: fail when a package's statement coverage falls below its baseline.

Usage: scripts/coverage_ratchet.py coverage.out .github/coverage-baseline.txt

The baseline has one "<package> <percent>" per line (# comments allowed). A
package may lose up to TOLERANCE points (rounding, small refactors). When a
package is more than RAISE_HINT points above its baseline, the output says so
and the baseline should be raised in the same PR. The output is Markdown, so it
can go to the job summary.
Exit: 0 ok, 1 below baseline, 2 bad input.
"""
import os
import sys
from collections import defaultdict

TOLERANCE = 0.5
RAISE_HINT = 2.0


def package_coverage(path):
    total = defaultdict(int)
    covered = defaultdict(int)
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("mode:"):
                continue
            loc, stmts, count = line.rsplit(" ", 2)
            pkg = loc.split(":", 1)[0].rsplit("/", 1)[0]
            total[pkg] += int(stmts)
            if int(count) > 0:
                covered[pkg] += int(stmts)
    return {p: (100.0 * covered[p] / total[p] if total[p] else 100.0) for p in total}


def read_baseline(path):
    base = {}
    with open(path, encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            try:
                pkg, pct = line.split()
                base[pkg] = float(pct)
            except ValueError:
                raise SystemExit(f"{path}:{n}: expected '<package> <percent>', got '{line}'")
    return base


def main(argv):
    if len(argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    try:
        current = package_coverage(argv[1])
        base = read_baseline(argv[2])
    except SystemExit as e:
        print(e, file=sys.stderr)
        return 2
    failed = False
    rows = []
    notes = []
    for pkg in sorted(set(current) | set(base)):
        now, was = current.get(pkg), base.get(pkg)
        if now is None:
            rows.append(f"| {pkg} | - | {was:.1f}% | missing from the profile |")
            failed = True
        elif was is None:
            rows.append(f"| {pkg} | {now:.1f}% | - | no baseline yet |")
            notes.append(f"{pkg} has no baseline: add '{pkg} {now:.1f}' to the baseline file.")
        elif now < was - TOLERANCE:
            rows.append(f"| {pkg} | {now:.1f}% | {was:.1f}% | **below baseline** |")
            failed = True
        else:
            rows.append(f"| {pkg} | {now:.1f}% | {was:.1f}% | ok |")
            if now > was + RAISE_HINT:
                notes.append(f"{pkg} is {now - was:.1f} points above its baseline: raise it to {now:.1f}.")
    out = ["## Coverage ratchet", "", "| package | coverage | baseline | |", "|---|---|---|---|", *rows]
    out += [f"\n{n}" for n in notes]
    if failed:
        out.append("\nCoverage fell below the baseline. Add tests, or lower the baseline on purpose in the same PR.")
    text = "\n".join(out)
    print(text)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as f:
            f.write(text + "\n")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
