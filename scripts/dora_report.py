#!/usr/bin/env python3
"""A small DORA-style report from the published GitHub Releases, for the job summary.

Deployment frequency: published (not draft, not pre-release) releases in the
window. Lead time for changes: for each release in the window, the time from
each commit since the previous release to the release's publication; reported
as the median over all those commits. Change failure rate and time to restore
are not measured (they need an incident record this project does not keep).

Environment: GITHUB_REPOSITORY (owner/name), GITHUB_STEP_SUMMARY (optional),
DORA_DAYS (window, default 90), DORA_NOW (ISO time, for tests). Needs `gh`.
Exit: 0 ok, 1 gh failed, 2 bad setup.
"""
import json
import os
import statistics
import subprocess
import sys
from datetime import datetime, timedelta, timezone


def parse(s):
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def gh_api(path):
    r = subprocess.run(["gh", "api", path], capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"gh api {path} failed: {r.stderr.strip()}")
    return json.loads(r.stdout)


def main():
    repo = os.environ.get("GITHUB_REPOSITORY")
    if not repo:
        print("set GITHUB_REPOSITORY=owner/name", file=sys.stderr)
        return 2
    days = int(os.environ.get("DORA_DAYS", "90"))
    now = parse(os.environ["DORA_NOW"]) if os.environ.get("DORA_NOW") else datetime.now(timezone.utc)
    try:
        releases = gh_api(f"repos/{repo}/releases?per_page=100")
        pub = sorted(
            (r for r in releases if not r["draft"] and not r["prerelease"] and r.get("published_at")),
            key=lambda r: parse(r["published_at"]), reverse=True)
        since = now - timedelta(days=days)
        leads = []
        recent = 0
        for i, r in enumerate(pub):
            when = parse(r["published_at"])
            if when < since:
                continue
            recent += 1
            if i + 1 < len(pub):
                cmp = gh_api(f"repos/{repo}/compare/{pub[i + 1]['tag_name']}...{r['tag_name']}")
                leads += [(when - parse(c["commit"]["committer"]["date"])).total_seconds() / 86400
                          for c in cmp["commits"]]
    except RuntimeError as e:
        print(f"::error::{e}")
        return 1
    lead = f"{statistics.median(leads):.1f} days" if leads else "n/a"
    lines = [
        "## Delivery metrics (DORA)",
        "",
        f"- Releases in the last {days} days: {recent}",
        f"- Median lead time: {lead}" + (f" (over {len(leads)} commits)" if leads else ""),
        "",
        "Lead time = commit on main to the release being published. Change failure rate and time to restore are not measured.",
    ]
    text = "\n".join(lines)
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as f:
            f.write(text + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
