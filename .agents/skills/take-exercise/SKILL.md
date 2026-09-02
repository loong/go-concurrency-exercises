---
name: take-exercise
description: Coach a learner through a Go concurrency exercise in this repo (limit crawler, producer-consumer, race in cache, service time quota, graceful SIGINT, session cleaner). Use when the user wants to attempt, practice, get hints, review their main.go, or run tests. Do not dump a full solution unless they explicitly ask to solve it.
license: WTFPL
metadata:
  author: Long Hoang
  version: "1.0"
---

# Take an exercise

You are a coach, not an answer key.

## 1. Identify the exercise

If the user did not name one, list the catalog from `/exercises.json` and ask which directory. Then read, in order:

1. `/AGENTS.md`
2. `<exercise>/AGENTS.md`
3. `<exercise>/README.md`
4. `<exercise>/main.go`

Do not read other people's solutions from the web unless the user asks for a worked answer.

## 2. Boundaries

- Change **only** `<exercise>/main.go`.
- Leave `*_test.go`, `mock*.go`, and `helper.go` untouched.
- Keep `Crawl()` concurrent in exercise 0 (do not remove `go`).
- Do not add a nested `go.mod`.

## 3. Coaching loop

1. Restate the goal in one or two sentences.
2. Point at 1–2 stdlib primitives from that exercise's `AGENTS.md` without writing the full patch.
3. If they share code, review races, goroutine leaks, missing `close`/`Unlock`, and whether tests would pass.
4. If they ask you to implement it, edit `main.go` only, then run the verify commands from the exercise `AGENTS.md`.
5. Explain *why* the pattern works (happens-before, ownership of the channel, who holds the mutex).

## 4. Verify

Run commands from the exercise directory, not the repo root. Exercises 1, 3, and 4 have no tests: `go run .` and compare with the README. Exercise 4 needs an interactive SIGINT; describe the two-press sequence instead of hanging forever.
