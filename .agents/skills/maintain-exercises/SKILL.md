---
name: maintain-exercises
description: Add or update Go concurrency exercises in this workshop without committing solutions. Use when maintaining the repo, adding a challenge, changing tests or mocks, or updating agent discovery files (AGENTS.md, llms.txt, exercises.json, skills).
license: WTFPL
metadata:
  author: Long Hoang
  version: "1.0"
---

# Maintain exercises

Starter `main.go` files must stay unsolved on the default branch.

## Adding an exercise

1. Create `N-slug/` with `main.go` (broken or sequential starter), support files marked `DO NOT EDIT THIS PART`, `README.md` (human), and `AGENTS.md` (agent).
2. Use `package main`. Do not add a nested `go.mod`.
3. Prefer `check_test.go` that fails on the starter and passes on a correct solution. If the behavior is interactive, document `go run .` instead.
4. Append the exercise to `/exercises.json`.
5. Add a row to the root `/AGENTS.md` layout table.
6. Add a bullet under **Exercises** in `/llms.txt` with raw.githubusercontent.com links to the README and AGENTS.md.
7. Link the new directory from `/README.md`.

## Editing existing exercises

- Keep learner-facing rules: only `main.go` is the solution file.
- Do not "fix" starter races or missing concurrency unless the task itself is changing.
- If you change verify commands, update the exercise `AGENTS.md`, `exercises.json`, and the root layout table together.

## Agent discovery files

| File | Role |
| --- | --- |
| `AGENTS.md` | Operational rules for every coding agent |
| `CLAUDE.md` | `@AGENTS.md` import for Claude Code |
| `.github/copilot-instructions.md` | Copilot pointer at AGENTS.md |
| `llms.txt` | Curated index for crawlers and chat agents |
| `exercises.json` | Machine-readable catalog |
| `.agents/skills/*/SKILL.md` | On-demand workflows |

Keep these four in sync: `exercises.json`, root `AGENTS.md`, `llms.txt`, `README.md`.
