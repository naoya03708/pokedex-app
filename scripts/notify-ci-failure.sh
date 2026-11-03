#!/bin/bash
# Opens an issue for a failed workflow run on main, a tag or a schedule, or
# comments on the open one for the same workflow and branch (no duplicates).
# Input comes from the environment, never from the script text:
#   WORKFLOW_NAME BRANCH EVENT SHA RUN_URL   (GH_TOKEN and GH_REPO for gh)
set -euo pipefail

: "${WORKFLOW_NAME:?}" "${BRANCH:?}" "${EVENT:?}" "${SHA:?}" "${RUN_URL:?}"

label="ci-failure"
title="CI failure: $WORKFLOW_NAME on $BRANCH"
body="The **$WORKFLOW_NAME** workflow failed on \`$BRANCH\` (trigger: $EVENT, commit \`${SHA:0:7}\`).

Run: $RUN_URL

This issue is opened automatically. Fix the cause (or re-run if it was transient), then close it. Later failures of the same workflow and branch are added as comments."

gh label create "$label" --color B60205 \
  --description "A workflow failed on main, a tag or a schedule" --force > /dev/null

# One "number<TAB>title" line per open issue; gh does the JSON handling.
existing=""
while IFS=$'\t' read -r number issue_title; do
  if [ "$issue_title" = "$title" ]; then
    existing="$number"
    break
  fi
done < <(gh issue list --label "$label" --state open --limit 100 \
  --json number,title --jq '.[] | "\(.number)\t\(.title)"')

if [ -n "$existing" ]; then
  gh issue comment "$existing" --body "$body"
else
  gh issue create --title "$title" --label "$label" --body "$body"
fi
