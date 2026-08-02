# Channel of Channels (Bridge Pattern)

Given is a `LogHub` (see `mockloghub.go`) that simulates log-collecting
infrastructure where new "shard" sources connect over time. Instead of
handing you a single channel of log lines, `StartLogHub` returns a
channel of channels: every value it emits is itself a channel carrying
the log lines from one shard, which closes once that shard is done.
`StartLogHub` closes the outer channel once all of its shards have
been started - but by then, the shards themselves may still be
producing lines.

The naive `Bridge` function in `main.go` is supposed to flatten this
channel-of-channels into a single stream of log lines that `main` can
simply range over. It doesn't: it only ever looks at the very first
inner channel it receives, forwards its lines, and then stops -
silently dropping every log line produced by every other shard.

Your task is to fix `Bridge` so that it flattens **every** inner
channel it ever receives from `chanStream` into the single output
stream - a fan-in over a dynamically arriving, unbounded set of
channels. The output channel must close once `chanStream` itself has
been closed **and** every inner channel it ever produced has been
fully drained. `Bridge` must also stop promptly - closing its output
and abandoning any further reads - as soon as `done` is closed. The
function signature must stay the same:

```go
func Bridge(chanStream <-chan (<-chan string), done <-chan struct{}) <-chan string
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
