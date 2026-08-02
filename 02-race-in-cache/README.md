# Race condition in caching scenario

Given is some code to cache key-value pairs from a mock database into
the main memory (to reduce access time). The code is buggy and
contains a race condition. Change the code to make this thread safe.

Also, try to get your solution down to less than 30 seconds to run tests.  *Hint*: fetching from the database takes the longest.

*Note*: Map access is unsafe only when updates are occurring. As long as all goroutines are only reading and not changing the map, it is safe to access the map concurrently without synchronization. (See [https://golang.org/doc/faq#atomic_maps](https://golang.org/doc/faq#atomic_maps))

If possible, get your solution down to less than 5 seconds for all tests.

## Background Reading

* [https://tour.golang.org/concurrency/9](https://tour.golang.org/concurrency/9)
* [https://golang.org/ref/mem](https://golang.org/ref/mem)

# Test your solution

Use the following command to test for race conditions and correct functionality:
```
go test -race
```

Correct solution:
No output = solution correct:
```
$ go test -race
$
```

Incorrect solution:
```
==================
WARNING: DATA RACE
Write by goroutine 7:
...
==================
Found 3 data race(s)
```

## Additional Reading

* [https://golang.org/pkg/sync/](https://golang.org/pkg/sync/)
* [https://gobyexample.com/mutexes](https://gobyexample.com/mutexes)
* [https://golangdocs.com/mutex-in-golang](https://golangdocs.com/mutex-in-golang)

### High Performance Caches in Production

* [https://www.mailgun.com/blog/golangs-superior-cache-solution-memcached-redis/](https://www.mailgun.com/blog/golangs-superior-cache-solution-memcached-redis/)
* [https://allegro.tech/2016/03/writing-fast-cache-service-in-go.html](https://allegro.tech/2016/03/writing-fast-cache-service-in-go.html)
