---
name: feature
description: Implement one requirement (FR-n from docs/requirements.md) end to end with acceptance-criteria-driven TDD, then open a PR and watch CI until green. Use when the user asks to implement a feature, an FR, or the next TODO step.
---

# /feature <FR-ID or short description>

The outer loop of this repository: one requirement → one branch → one PR, finished when CI is green. Read `docs/development.md` once per session for the full rules; this is the working checklist. The hooks in `.claude/settings.json` handle formatting and vet on each edit (inner loop) and run `scripts/check.sh` before you finish (middle loop).

## 0. Scope
- Find the requirement in `docs/requirements.md`. If it is missing or its acceptance criteria are vague, stop and propose the criteria to the user first (a spec-only PR is fine). Do not invent behavior.
- One PR covers one theme. If the requirement is large, split it (e.g. `pokeapi` client first, UI next) and say which part this PR covers.
- Start from an up-to-date main: `git switch main`, `git pull --ff-only`, then `git switch -c feat/<short-name>` as separate commands.

## 1. Red
- Write the failing tests first. Name them after the requirement, e.g. `TestSearch_FR1_CaseInsensitive`; order the body Given / When / Then.
  - `internal/pokeapi`: `httptest.Server`, no real network.
  - `internal/app`: a fake `Source`.
  - UI: `ui.NewTester` driving the view as a user would.
- Run the new tests and **confirm they fail for the expected reason** (not a compile error elsewhere). Quote the failure in your notes for the PR.

## 2. Green
- Write the smallest implementation that passes. Respect the layering `main → internal/ui → internal/app → internal/pokeapi`.
- MyGo rules (`.claude/skills/mygo-maintenance/SKILL.md`): no elements or `*ui.Context` in state or goroutines; slow I/O off the UI thread, results through `Window.Update`; reject stale results.

## 3. Refactor
- Clean up with tests green. Run `scripts/check.sh` (the same steps as CI). Update `CHANGELOG.md` under `[Unreleased]` for user-visible changes, and docs if behavior or process changed.

## 4. Self-review
- Run the `reviewer` subagent on the branch diff. Fix what it confirms; note anything you deliberately leave.

## 5. PR and CI
- Commit with a Conventional Commit message in English. Push the branch (never main), then `gh pr create` with a Conventional Commit title and the template sections filled: the Requirements table maps each FR to its tests.
- Watch CI: `gh pr checks <n> --watch`. On failure, read the log (`gh run view <id> --log-failed`), fix, push, and watch again. After three failed rounds on the same problem, stop and report.

## 6. Hand-off
- Report to the user: PR link, which acceptance criteria are covered by which tests, CI status, and anything left open. Merging is the user's job.
