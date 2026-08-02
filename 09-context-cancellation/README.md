# Context Cancellation & Propagation: A Request Chain That Ignores Its Deadline

Given is `HandleRequest`, which is supposed to call three independent
downstream layers - `CallLayerA`, `CallLayerB`, and `CallLayerC` (see
`mocklayers.go`) - in sequence and concatenate their results. Each
layer is a well-behaved, context-aware dependency: it takes 300ms to
complete under normal conditions, but returns early with `ctx.Err()`
the moment the context it's given is cancelled or its deadline
expires.

The current implementation ignores the context it receives entirely:
it calls every layer with `context.Background()` instead of `ctx`, so
a caller's timeout or cancellation is never actually propagated
anywhere - each layer always runs its full 300ms regardless, even if
the caller has already given up.

Your task is to fix `HandleRequest` so that it passes `ctx` (not
`context.Background()`) into each layer call, so the caller's
deadline/cancellation actually reaches them and a layer can return
early via `ctx.Err()` instead of always running its full 300ms.

The function signature must stay exactly the same:

```go
func HandleRequest(ctx context.Context) (string, error)
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
