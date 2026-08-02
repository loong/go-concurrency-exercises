//////////////////////////////////////////////////////////////////////
//
// Given is an Account that is supposed to be safe for concurrent
// Deposit/Withdraw/Balance calls from many goroutines at once. Right
// now it is not: balance is read and written directly, with no
// synchronization of any kind, so concurrent calls race on it (lost
// updates, torn reads) - and Withdraw doesn't even check for
// sufficient funds before letting the balance go negative.
//
// Your task is to reimplement Account the "share memory by
// communicating" way, using the actor pattern instead of a mutex:
// NewAccount should start a single long-lived goroutine that owns
// balance and processes exactly one request at a time from an
// unexported channel of request structs (their shape is up to you,
// as long as they carry what kind of operation it is, any amount
// involved, and a reply channel to send the result back on).
// Deposit, Withdraw, and Balance become thin methods that build a
// request, send it on that channel, and block on the reply channel
// for the result. Correctness must come purely from serializing all
// access to balance through that single actor goroutine - no
// sync.Mutex (or any other lock) may be used anywhere.
//
// Withdraw must return ErrInsufficientFunds - and leave balance
// unchanged - if amount is greater than balance at the time the
// actor goroutine processes the request. The function signatures
// must stay the same:
//
//     func NewAccount(initial int) *Account
//     func (a *Account) Deposit(amount int)
//     func (a *Account) Withdraw(amount int) error
//     func (a *Account) Balance() int
//
// so that Account remains a drop-in replacement for the naive
// version below.
//
// Unlike a mutex, which needs nothing to clean up, the actor
// goroutine started in NewAccount keeps running for as long as the
// process does unless something tells it to stop. So Account also
// needs a Close method - func (a *Account) Close() - that terminates
// that goroutine; after Close returns, the actor goroutine must have
// exited. Calling Deposit, Withdraw, or Balance on a closed Account
// is not exercised by the tests, so its behavior is up to you.
//

package main

import (
	"errors"
	"fmt"
	"sync"
)

// Account is supposed to be safe for concurrent Deposit/Withdraw/
// Balance calls from many goroutines at once. Right now it is not:
// balance is read and written directly, with no synchronization of
// any kind, so concurrent calls race on it (lost updates, torn
// reads) - and Withdraw doesn't even check for sufficient funds.
type Account struct {
	balance int
}

func NewAccount(initial int) *Account {
	return &Account{balance: initial}
}

func (a *Account) Deposit(amount int) {
	a.balance = a.balance + amount
}

// Withdraw is supposed to fail with ErrInsufficientFunds rather than
// letting balance go negative, but the naive version doesn't check at
// all.
func (a *Account) Withdraw(amount int) error {
	a.balance = a.balance - amount
	return nil
}

func (a *Account) Balance() int {
	return a.balance
}

// Close is a no-op here since the naive version above starts no
// goroutine. An actor-based Account must override this to actually
// stop its actor goroutine.
func (a *Account) Close() {}

var ErrInsufficientFunds = errors.New("insufficient funds")

func main() {
	account := NewAccount(1000)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			account.Deposit(1)
		}()
	}
	wg.Wait()

	fmt.Printf("final balance after 100 concurrent deposits of 1: %d (want 1100)\n", account.Balance())

	if err := account.Withdraw(2000); err != nil {
		fmt.Printf("withdraw correctly rejected: %v\n", err)
	} else {
		fmt.Printf("withdraw of 2000 from a smaller balance was allowed - balance is now %d\n", account.Balance())
	}
}
