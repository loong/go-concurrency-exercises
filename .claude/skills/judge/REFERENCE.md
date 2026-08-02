# Judging rubric

Judge each dimension against the code actually submitted, not against an
idealized rewrite. Skip a dimension entirely if it doesn't apply (e.g. no
error handling to check in a pure channel-plumbing exercise). Cite every
finding as `file:line`.

## 1. Correctness & concurrency safety (weigh heaviest)

- Does it solve the problem the exercise's `README.md` actually states,
  including edge cases it calls out explicitly (timing windows, partial
  failure, cancellation) — not just the happy path?
- Every read/write of shared state (maps, slices, counters, structs) goes
  through the same `sync.Mutex`/`sync.RWMutex`/channel — no lock covers only
  some of the access paths.
- Goroutines have a clear, bounded lifetime. Every goroutine started has a
  way to exit: closed channel, `context.Context` cancellation, `done` channel,
  or a bounded amount of work. No goroutine leaks (started but nothing ever
  causes it to return) or forever-blocking sends/receives with no unblocking
  path.
- Channel direction and closing follow Go's ownership convention: the sender
  closes, the receiver never closes; only the goroutine that owns a channel
  closes it, and it's closed exactly once (not via `close()` racing another
  `close()` or a send).
- If `context.Context` is in play: it's the first parameter, named `ctx`, and
  actually checked (`<-ctx.Done()` / `ctx.Err()`), not just threaded through
  unused.
- `-race` is clean. A vet warning or race report is a correctness bug, not a
  style nit — surface it as the top finding.

## 2. Idiomatic Go style

- **Naming**: `MixedCaps`/`mixedCaps`, not snake_case; short receiver names
  consistent across a type's methods; no stutter (`session.SessionID` instead
  of `session.ID`); exported identifiers only when the exercise's `main()` or
  tests need them exported.
- **Errors**: returned, not panicked (except truly unrecoverable init
  failures) or silently swallowed with `_`; wrapped with `fmt.Errorf("...: %w",
  err)` when context is added; sentinel errors (`var ErrX = errors.New(...)`)
  compared with `errors.Is`, not string matching.
- **Comments**: exported identifiers that need a doc comment start with the
  identifier's name (`// SessionManager keeps ...`), per godoc convention.
  Comments explain *why*, not restate the line below them.
- **Zero-value & construction**: a `New*` constructor is used where a zero
  value isn't ready to use; struct literals use field names for anything with
  more than 2-3 fields.
- **Formatting**: `gofmt -l` is empty (checked in step 4, report here if not).
- No dead code, unused imports/params, or copy-pasted boilerplate the
  language already gives you (e.g. hand-rolled retry loop where
  `errgroup`/`sync.WaitGroup` fits).

## 3. Concurrency primitive choice

- Right tool for the shape of the problem: `sync.Mutex` for shared mutable
  state, channels for handoff/signaling/pipelines, `sync.WaitGroup` for
  fan-out completion, `context` for cancellation/deadlines,
  `sync.Once`/`atomic` where a mutex would be overkill. Flag both directions —
  a mutex doing a channel's job, and vice versa — since either is a sign the
  primitive wasn't chosen for the problem's shape.
- Buffered vs unbuffered channel choice is deliberate (not just "made the
  test pass"), and sized/justified if buffered.
- No busy-waiting (`for { select { default: ... } }` spin loops) where a
  blocking receive or timer would do.

## 4. Simplicity / not over-engineered

- No abstraction, interface, or generic param the exercise doesn't need.
- Solution scoped to `main.go`'s actual job — no speculative extensibility
  (config options, plugin points) nothing in the repo calls for.

## Output format

```
## Verdict: <one line — e.g. "Idiomatic and correct" | "Correct but has style issues" | "Has a concurrency bug">

**Empirical checks** (folder, from repo root):
- gofmt: <clean | files listed>
- go vet: <clean | warnings>
- go test -race: <pass | failures/race report, verbatim>

**Findings** (grouped by rubric section above, most severe first; omit
empty sections):

### Correctness & concurrency safety
- `main.go:NN` — <what's wrong> — <why it matters / failure scenario>

### Idiomatic Go style
- `main.go:NN` — ...

### Concurrency primitive choice
- ...

### Simplicity
- ...

**Reference solution notes** (only if solutions/NN-*.md exists and adds
something the rubric pass didn't already surface — e.g. a known test-harness
gotcha like a synctest goroutine-leak trap):
- ...
```

If there are zero findings in a section, omit that section rather than
writing "no issues found." If the whole review is clean, say so in one line
and skip straight to empirical checks.
