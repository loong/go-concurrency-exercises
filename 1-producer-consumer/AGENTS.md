# 1-producer-consumer

Run the tweet producer and consumer **at the same time** so the mock stream's per-tweet delay overlaps with `IsTalkingAboutGo()` work.

## Files

- Edit: `main.go`
- Do not edit: `mockstream.go`

## Verify

No `*_test.go`. From this directory:

```bash
go run .
```

Sequential starter is about 3.5s. A concurrent solution is about 2s. Output lines (who tweets about golang) should match the README.

## Hint direction (not a solution)

A channel between `producer` and `consumer`, plus `go`, plus `close` when the stream hits `ErrEOF`. Do not wait for the full slice before consuming.
