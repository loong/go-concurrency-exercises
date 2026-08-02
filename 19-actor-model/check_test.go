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
)

// TestAccountBasicOperations exercises Deposit, Withdraw, and Balance
// from a single goroutine, so it passes against both the naive and
// the fixed implementation - except for the insufficient-funds check,
// which only the fixed implementation performs.
func TestAccountBasicOperations(t *testing.T) {
	account := NewAccount(100)

	if got := account.Balance(); got != 100 {
		t.Fatalf("initial balance = %d, want 100", got)
	}

	account.Deposit(50)
	if got := account.Balance(); got != 150 {
		t.Fatalf("balance after deposit = %d, want 150", got)
	}

	if err := account.Withdraw(30); err != nil {
		t.Fatalf("Withdraw(30) returned unexpected error: %v", err)
	}
	if got := account.Balance(); got != 120 {
		t.Fatalf("balance after withdraw = %d, want 120", got)
	}

	err := account.Withdraw(1000)
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("Withdraw(1000) from balance 120 = %v, want ErrInsufficientFunds", err)
	}
	if got := account.Balance(); got != 120 {
		t.Errorf("balance after rejected withdraw = %d, want unchanged 120", got)
	}
}

// TestAccountConcurrentSafety is the key `-race` test. Many goroutines
// deposit concurrently; the naive implementation reads and writes
// balance with no synchronization at all, so the race detector must
// flag a data race on every run, and the final balance is likely to
// be wrong on top of that. The actor implementation serializes all
// access through a single goroutine, so this passes cleanly under
// -race with the exact expected balance.
func TestAccountConcurrentSafety(t *testing.T) {
	const initial = 10000
	const goroutines = 100

	account := NewAccount(initial)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			account.Deposit(1)
		}()
	}
	wg.Wait()

	if got, want := account.Balance(), initial+goroutines; got != want {
		t.Errorf("final balance = %d, want %d (exactly initial + %d concurrent deposits of 1)", got, want, goroutines)
	}
}

// TestAccountWithdrawRejectsInsufficientFunds fires more concurrent
// withdrawals than the account can afford and checks that exactly as
// many succeed as the balance allows, with no lost or duplicated
// withdrawals and no negative balance - which requires the actor to
// serialize the check-then-debit sequence atomically per request.
func TestAccountWithdrawRejectsInsufficientFunds(t *testing.T) {
	const initial = 100
	const amount = 30
	const attempts = 5

	account := NewAccount(initial)

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := account.Withdraw(amount); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !errors.Is(err, ErrInsufficientFunds) {
				t.Errorf("Withdraw(%d) returned unexpected error: %v", amount, err)
			}
		}()
	}
	wg.Wait()

	finalBalance := account.Balance()

	if finalBalance < 0 {
		t.Fatalf("final balance went negative: %d", finalBalance)
	}

	if successes > initial/amount {
		t.Errorf("%d withdrawals of %d succeeded, but only %d can fit in a balance of %d",
			successes, amount, initial/amount, initial)
	}

	if want := initial - successes*amount; finalBalance != want {
		t.Errorf("final balance = %d, want %d (initial %d minus %d successful withdrawals of %d)",
			finalBalance, want, initial, successes, amount)
	}
}

// TestAccountCloseStopsActorGoroutine checks that Close actually
// terminates the actor goroutine an actor-based Account starts,
// rather than just existing as a no-op to satisfy the API. It never
// inspects Account's internals - it relies entirely on
// synctest.Test's own rule that every goroutine spawned inside the
// bubble (transitively) must have exited by the time the function
// passed to it returns. NewAccount's actor goroutine, if any, is
// spawned inside this bubble; if Close doesn't make it exit, it's
// still durably blocked (e.g. on <-requests) when this test function
// returns, and synctest.Test panics with a deadlock message instead
// of this test merely failing an assertion.
//
// This passes trivially against the naive, lock-free implementation
// above, which starts no goroutine at all - it only starts pulling
// its weight once Deposit/Withdraw/Balance are backed by an actor.
func TestAccountCloseStopsActorGoroutine(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		account := NewAccount(100)
		defer account.Close()

		account.Deposit(50)
		if got := account.Balance(); got != 150 {
			t.Fatalf("balance = %d, want 150", got)
		}
	})
}
