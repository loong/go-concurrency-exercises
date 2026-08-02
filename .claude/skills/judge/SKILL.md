---
name: judge
description: Judge a solved exercise in this go-concurrency-exercises repo for idiomatic Go and concurrency best practices. Use when the user says "/judge <problem>", "judge problem N", "review my solution for exercise N", or names an exercise by number, folder name, or topic and wants a code-quality/idiom review (not just "does it pass").
---

# Judge

Reviews a solved exercise's `main.go` against idiomatic Go and concurrency
best practices, grounded in `go vet` / `gofmt` / `go test -race` output. This
is a code-quality review, not a pass/fail gate — a solution can pass every
test and still get flagged for non-idiomatic style, and the rubric in
[REFERENCE.md](REFERENCE.md) matters more than the test result.

## Steps

1. **Resolve the target folder.** `args` may be a bare number (`5`, `05`), a
   phrase (`problem 5`), a full folder name (`05-session-cleaner`), or a topic
   keyword (`mutex`, `pipeline`). Match against directories at the repo root
   named `NN-*` (zero-padded two digits). If nothing matches unambiguously,
   list the numbered exercises (see `README.md`'s overview table) and ask
   which one they meant.

2. **Read the problem statement**: the folder's own `README.md`.

3. **Read the code**: `main.go` is the only file the repo's rules
   (`README.md` → "How to take this challenge") ask the solver to edit — treat
   it as the solution under review. Also read every other `.go` file in the
   folder (helpers, mock servers, `check_test.go`) for the interfaces/behavior
   the solution must satisfy. If `git diff` (or `git status`) shows changes to
   any non-`main.go` file in this folder, call that out explicitly as a
   deviation from the stated rules — it's not automatically wrong (e.g. a
   `check_test.go` conversion to `testing/synctest` is a legitimate repo-level
   change), but it needs its own justification, separate from judging `main.go`.

4. **Get empirical grounding before judging style.** From the repo root:
   - `gofmt -l <folder>` — anything listed is unformatted.
   - `go vet ./<folder>/...`
   - `go test -race ./<folder>/...` (add `-count=3` if the exercise involves
     timing/goroutine-lifecycle correctness — several of these do)

   Report actual pass/fail and any vet/race output verbatim. Don't guess at
   whether it compiles or races — run it.

5. **Form your own judgment first**, using [REFERENCE.md](REFERENCE.md)'s
   rubric, before looking at `solutions/NN-*.md` if it exists. That file is a
   spoiler write-up with a suggested solution and known gotchas for that
   specific exercise (e.g. a `synctest` goroutine-leak trap) — read it *after*
   your own pass, to sanity-check tradeoffs and catch anything the rubric
   alone wouldn't surface, not to diff-and-copy its solution. Never treat
   "doesn't match the reference implementation" as a finding by itself —
   different valid designs exist; judge the submitted code on its own terms.

6. **Report findings** using the format in REFERENCE.md: a one-line verdict,
   the empirical results, then concrete `file:line` findings grouped by
   rubric dimension, each with *why it matters* — not a restyle of the code.
