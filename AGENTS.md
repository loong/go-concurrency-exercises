# AGENTS.md

This is a hands-on Go concurrency workshop, not a library. Each numbered directory is a standalone `package main` exercise. Starter `main.go` files are intentionally incomplete or racy.

Read `exercises.json` for the machine-readable catalog. Per-exercise details live in that directory's `AGENTS.md` (nearest file wins). Skills are in `.agents/skills/`.

## When the user is learning

- Edit **only** `main.go` in the chosen exercise directory.
- Do not modify `*_test.go`, `mock*.go`, `helper.go`, or other support files.
- Do not paste a complete solution unless the user explicitly asks to solve it, implement it, or show the answer.
- Prefer hints, the stdlib primitives listed in the exercise `AGENTS.md`, and the exercise `README.md`.
- After any code change, run the verify commands from that exercise's `AGENTS.md`.

## When maintaining this repository

- Keep starter `main.go` unsolved. Do not commit working solutions into the default branch.
- Put agent instructions in `AGENTS.md` (root and per exercise), not in the human README.
- Keep `exercises.json` and `llms.txt` in sync when adding or renaming exercises.
- Do not add per-exercise `go.mod` files. The module root is this directory: `github.com/loong/go-concurrency-exercises`.

## Layout

| Dir | Topic | Automated tests |
| --- | --- | --- |
| `0-limit-crawler` | Rate-limit concurrent crawls | `go test` |
| `1-producer-consumer` | Concurrent producer/consumer | none (`go run .`) |
| `2-race-in-cache` | Thread-safe LRU cache | `go test -race` |
| `3-limit-service-time` | Time-quota per request/user | none (`go run .`) |
| `4-graceful-sigint` | Graceful SIGINT then force-kill | none (`go run .`) |
| `5-session-cleaner` | Background session GC | `go test` and `go test -race` |

## Commands

Need Go 1.19+. Run tests from the exercise directory, not the repo root.

```bash
cd 0-limit-crawler && go test
cd 2-race-in-cache && go test -race
cd 5-session-cleaner && go test && go test -race
```

Exercises 1, 3, and 4 have no `*_test.go`. Run `go run .` and compare behavior with the README.

## Style

- Idiomatic Go: goroutines, channels, `select`, `context`, `sync`, `os/signal`.
- No trailing whitespace. Files end with a newline. Blank lines contain no spaces.
