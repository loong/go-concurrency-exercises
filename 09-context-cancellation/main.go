//////////////////////////////////////////////////////////////////////
//
// HandleRequest is supposed to call layers A, B and C in sequence
// (see mocklayers.go), honoring the caller's context so that if ctx
// is cancelled or its deadline expires, the whole chain gives up
// promptly instead of grinding through all three 300ms layers
// regardless. Each layer is itself well-behaved and context-aware -
// it returns early with ctx.Err() the instant the context it is
// given is done - but that only helps if HandleRequest actually
// hands it the real ctx in the first place.
//
// The naive implementation below ignores ctx entirely: it calls every
// layer with context.Background(), so a caller's timeout or
// cancellation is never propagated anywhere, and each layer runs its
// full 300ms no matter what, even if the caller has already given up.
//
// Your task is to fix HandleRequest so that it passes ctx (not
// context.Background()) into each layer call, so the caller's
// deadline/cancellation actually reaches them and a layer can return
// early via ctx.Err() instead of always running its full 300ms.
//
// Keep the function signature identical:
//
//     func HandleRequest(ctx context.Context) (string, error)
//

package main

import (
	"context"
	"fmt"
	"time"
)

// HandleRequest calls layers A, B and C in sequence and concatenates
// their results.
//
// NAIVE / BROKEN: it never passes ctx to any layer - each call uses
// context.Background() instead - so a caller's timeout or
// cancellation is never propagated to any of them.
func HandleRequest(ctx context.Context) (string, error) {
	a, err := CallLayerA(context.Background())
	if err != nil {
		return "", err
	}

	b, err := CallLayerB(context.Background())
	if err != nil {
		return "", err
	}

	c, err := CallLayerC(context.Background())
	if err != nil {
		return "", err
	}

	return a + b + c, nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := HandleRequest(ctx)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("HandleRequest failed after %s: %v\n", elapsed, err)
		return
	}

	fmt.Printf("HandleRequest succeeded after %s: %q\n", elapsed, result)
}
