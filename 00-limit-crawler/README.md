# Limit your crawler

Given is a crawler (modified from the Go tour) that requests pages
excessively. However, we don't want to burden the webserver too
much. Your task is to change the code to limit the crawler to at most
one page per second, while maintaining concurrency (in other words,
Crawl() must be called concurrently)

## Hint

This exercise can be solved in 3 lines only. If you can't do
it, have a look at this:
https://go.dev/wiki/RateLimiting

## Test your solution

Use `go test` to verify if your solution is correct.

Correct solution:
```
PASS
ok      github.com/loong/go-concurrency-exercises/00-limit-crawler  13.009s
```

Incorrect solution:
```
--- FAIL: TestMain (7.80s)
        main_test.go:18: There exists a two crawls who were executed less than 1 sec apart.
	        main_test.go:19: Solution is incorrect.
		FAIL
		exit status 1
		FAIL    github.com/loong/go-concurrency-exercises/00-limit-crawler  7.808s
```
