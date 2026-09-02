# 3-limit-service-time

Free-tier users get 10 seconds of processing. Premium users are not killed.

- Beginner: 10s max **per request**
- Advanced: 10s max **per user** (accumulated on `User.TimeUsed`)

`HandleRequest` must return `false` when it kills the process.

## Files

- Edit: `main.go`
- Do not edit: `mockserver.go`

`mockserver.go` defines `shortProcess` (6s) and `longProcess` (11s) and prints `done` vs `killed. (No quota left)`.

## Verify

No `*_test.go`. From this directory:

```bash
go run .
```

Expect free-tier 11s work to be killed. Premium 11s work should finish. For the advanced level, a free user who already spent 6s should not complete another 6s+ request.

## Hint direction (not a solution)

`select` on the work finishing vs `time.After` / `context.WithTimeout`. For the advanced level, track remaining quota on `u.TimeUsed` (seconds) across requests.
