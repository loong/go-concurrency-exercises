//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"errors"
	"sync"
)

// ErrGatewayDown is returned by Charge whenever the gateway has been
// set into a failing state.
var ErrGatewayDown = errors.New("payment gateway: connection down")

// PaymentGateway simulates a flaky downstream payment gateway. Tests
// (and main) can flip it between failing and healthy via SetFailing.
// All methods are safe for concurrent use.
type PaymentGateway struct {
	mu      sync.Mutex
	failing bool
	calls   int
}

// NewPaymentGateway creates a new, initially healthy, PaymentGateway.
func NewPaymentGateway() *PaymentGateway {
	return &PaymentGateway{}
}

// SetFailing controls whether subsequent calls to Charge succeed or
// fail. Used by tests to simulate the gateway going down and
// recovering.
func (g *PaymentGateway) SetFailing(failing bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failing = failing
}

// Calls returns the number of times Charge actually reached the
// gateway.
func (g *PaymentGateway) Calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

// Charge simulates charging amountCents to the gateway. It always
// counts as a call regardless of outcome. If the gateway is currently
// set to failing, it returns ErrGatewayDown; otherwise it succeeds.
func (g *PaymentGateway) Charge(amountCents int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.calls++

	if g.failing {
		return ErrGatewayDown
	}
	return nil
}
