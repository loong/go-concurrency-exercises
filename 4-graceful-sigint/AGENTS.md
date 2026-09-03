# 4-graceful-sigint

`proc.Run()` blocks forever. Make SIGINT (Ctrl-C) graceful:

1. First SIGINT: call `proc.Stop()` (it will not actually stop this mock).
2. Second SIGINT: exit the program.

## Files

- Edit: `main.go`
- Do not edit: `mockprocess.go`

## Verify

No `*_test.go`. Interactive only:

```bash
go run .
```

Then Ctrl-C once (should print the mock stop attempt and keep running) and Ctrl-C again (process exits). Do not leave `go run` hanging in unattended agent sessions; explain the sequence instead.

## Hint direction (not a solution)

`signal.Notify` on `os.Interrupt` / `syscall.SIGINT` with a signal channel. First receive → `proc.Stop()` on another goroutine so `Run()` can stay blocking. Second receive → `os.Exit` or return from `main`.
