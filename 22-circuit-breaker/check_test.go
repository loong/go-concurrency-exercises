//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// TestCircuitOpensAfterThreshold checks that the breaker passes
// failures straight through to the gateway while closed, but trips to
// Open and starts fail-fasting (without touching the gateway at all)
// once 5 consecutive failures have been observed.
func TestCircuitOpensAfterThreshold(t *testing.T) {
	gateway := NewPaymentGateway()
	cb := NewCircuitBreaker(gateway)

	gateway.SetFailing(true)

	for i := 1; i <= 5; i++ {
		err := cb.Execute(100)
		if !errors.Is(err, ErrGatewayDown) {
			t.Fatalf("call %d: expected ErrGatewayDown, got %v", i, err)
		}
	}

	if got := gateway.Calls(); got != 5 {
		t.Fatalf("expected gateway to have been called 5 times, got %d", got)
	}

	// The 6th call should now be failed fast by the open breaker,
	// without ever reaching the gateway.
	err := cb.Execute(100)
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen on 6th call, got %v", err)
	}

	if got := gateway.Calls(); got != 5 {
		t.Fatalf("expected gateway to still have been called only 5 times (fail-fast), got %d", got)
	}
}

// TestCircuitHalfOpenRecovery checks that after the cooldown elapses
// the breaker allows a trial call through (Half-Open), and that a
// success there fully recovers the breaker back to Closed rather than
// leaving it stuck.
func TestCircuitHalfOpenRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gateway := NewPaymentGateway()
		cb := NewCircuitBreaker(gateway)

		gateway.SetFailing(true)

		for i := 1; i <= 5; i++ {
			_ = cb.Execute(100)
		}

		// Confirm it's actually open now.
		if err := cb.Execute(100); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("expected breaker to be open, got %v", err)
		}

		// Gateway recovers, and we wait past the 2s cooldown.
		gateway.SetFailing(false)
		time.Sleep(2*time.Second + 10*time.Millisecond)

		// The trial (half-open) call should now go through and
		// succeed, closing the breaker again.
		if err := cb.Execute(100); err != nil {
			t.Fatalf("expected half-open trial call to succeed, got %v", err)
		}

		// Breaker should be fully closed now, not stuck half-open:
		// further calls should succeed normally too.
		for i := 0; i < 3; i++ {
			if err := cb.Execute(100); err != nil {
				t.Fatalf("expected breaker to be closed after recovery, got %v", err)
			}
		}
	})
}

// TestCircuitConcurrentSafety hammers Execute from many goroutines at
// once while the gateway is failing, to catch data races on the
// breaker's internal state (run with `go test -race`).
func TestCircuitConcurrentSafety(t *testing.T) {
	gateway := NewPaymentGateway()
	cb := NewCircuitBreaker(gateway)

	gateway.SetFailing(true)

	const numGoroutines = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = cb.Execute(100)
		}()
	}

	wg.Wait()

	// Loose, non-flaky sanity bound: the gateway can never have been
	// hit more often than the number of callers.
	if got := gateway.Calls(); got > numGoroutines {
		t.Fatalf("gateway called more times (%d) than goroutines launched (%d)", got, numGoroutines)
	}
}
