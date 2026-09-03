# 0-limit-crawler

Limit crawls to **at most one page per second** while still calling `Crawl()` concurrently. Do not remove the `go` keyword.

## Files

- Edit: `main.go`
- Do not edit: `mockfetcher.go`, `check_test.go`

## Verify

```bash
go test
```

A correct run takes about 13s. A solution that fetches too fast fails with crawls less than 1s apart.

## Hint direction (not a solution)

A ticker or token bucket that serializes `Fetch` (not the whole `Crawl` goroutine) is enough. The Go wiki page on rate limiting is linked from `README.md`. This exercise can be three lines.
