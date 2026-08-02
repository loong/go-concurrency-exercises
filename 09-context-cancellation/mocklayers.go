package main

import (
	"context"
	"time"
)

// LayerLatency is how long each downstream layer takes to complete
// under normal conditions, when it is not interrupted by ctx being
// cancelled or timing out first.
const LayerLatency = 300 * time.Millisecond

// callLayer is the shared implementation behind CallLayerA/B/C: it
// simulates one hop of a downstream call chain. It is a well-behaved,
// context-aware dependency - it waits for LayerLatency to simulate
// real work, but returns early with ctx.Err() the moment ctx is
// cancelled or its deadline expires, instead of blindly sleeping the
// full duration regardless of the caller's wishes.
func callLayer(ctx context.Context, result string) (string, error) {
	timer := time.NewTimer(LayerLatency)
	defer timer.Stop()

	select {
	case <-timer.C:
		return result, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// CallLayerA simulates the first hop of the downstream call chain.
func CallLayerA(ctx context.Context) (string, error) {
	return callLayer(ctx, "A")
}

// CallLayerB simulates the second hop of the downstream call chain.
func CallLayerB(ctx context.Context) (string, error) {
	return callLayer(ctx, "B")
}

// CallLayerC simulates the third hop of the downstream call chain.
func CallLayerC(ctx context.Context) (string, error) {
	return callLayer(ctx, "C")
}
