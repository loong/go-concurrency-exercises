# 5-session-cleaner

`SessionManager` grows forever. Start a background cleaner that deletes sessions not updated for more than 5 seconds. Removal may happen anytime between 5 and 7 seconds after the last update. Must be race-free.

## Files

- Edit: `main.go`
- Do not edit: `helper.go`, `check_test.go`

Start the cleaner from `NewSessionManager` so tests that only construct a manager still get GC.

## Verify

```bash
go test
go test -race
```

`TestSessionManagersCleaner` sleeps 7s then expects `ErrSessionNotFound`. `TestSessionManagersCleanerAfterUpdate` refreshes at 3s, expects the session still present 3s later, then gone 4s after that.

## Hint direction (not a solution)

A `time.Ticker` loop plus `sync.Mutex` / `sync.RWMutex` on the `sessions` map. Store last-updated time per session. Do not range over the map while another goroutine writes without a lock.
