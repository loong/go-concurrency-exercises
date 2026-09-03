# 2-race-in-cache

The LRU cache's map and list are not safe for concurrent writers. Make `Get` thread-safe without destroying cache performance.

## Files

- Edit: `main.go`
- Do not edit: `mockdb.go`, `mockserver.go`, `check_test.go`

## Verify

```bash
go test -race
```

Aim for the suite under 30s; under 5s is the stretch goal. The `-race` build must report no data races. Cache size must stay `CacheSize` (100). Avoid extra DB loads: `db.Calls` must stay within the test budget (`callsPerCycle`).

## Hint direction (not a solution)

`sync.Mutex` or `sync.RWMutex` around map/list mutation. Holding the lock during the slow DB `Load` will pass correctness and still be too slow — load outside the critical section, then insert carefully (double-check). Map reads are safe only when no writers exist; these tests have writers.
