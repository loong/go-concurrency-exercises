# Sharded Concurrent Cache: Reducing Lock Contention

Given is a key-value `Cache` that is already correct - a single mutex
protects a single map, so there is no data race no matter how many
goroutines call it concurrently. But it has a real performance
problem: the cache's one mutex is held for the entire duration of
every call to `Do`, including however long the caller's `work`
function takes to run. That means an operation on key `"a"` that
happens to be slow completely blocks an operation on a totally
unrelated key `"b"` for just as long, even though `"a"` and `"b"`
don't logically conflict at all - there is no reason for that
serialization other than this cache's overly coarse locking.

Your task is to reimplement `Cache` internally as N independent shards
(e.g. `const numShards = 16`), each with its own mutex and its own
sub-map, so operations on keys that land in different shards can
proceed with true parallelism, while operations on the SAME key remain
correctly serialized through that key's shard lock. Use a simple,
deterministic sharding function to pick a key's shard - for
concreteness (and so this exercise's tests can rely on it), shard by
the key's first byte:

```go
shardIndex := int(key[0]) % numShards
```

The signature must stay the same, so `Cache` remains a drop-in
replacement:

```go
func NewCache() *Cache
func (c *Cache) Do(key string, work func(cur int) int) int
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
