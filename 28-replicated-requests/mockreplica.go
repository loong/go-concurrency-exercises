//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"sync"
	"time"
)

// MockReplica is a test/demo helper that builds a Replica with a
// configurable artificial latency, and exposes - after the fact -
// whether that particular call observed its done channel being closed
// and exited early because of it, versus running all the way to
// completion regardless of anyone still caring about the result.
type MockReplica struct {
	name    string
	value   string
	err     error
	latency time.Duration

	mu             sync.Mutex
	observedCancel bool
	completed      bool
}

// NewMockReplica creates a MockReplica named name that, once wired up
// via its Replica method, waits for latency to elapse (simulating the
// replica doing real, slow work) before returning name as its value
// with a nil error - unless its done is closed first.
func NewMockReplica(name string, latency time.Duration) *MockReplica {
	return &MockReplica{name: name, value: name, latency: latency}
}

// WithError makes the mock return err instead of a nil error once its
// artificial latency elapses.
func (m *MockReplica) WithError(err error) *MockReplica {
	m.err = err
	return m
}

// Replica is the Replica function backed by this mock - pass
// m.Replica straight into FetchFastest as one of its replicas. It
// blocks for m.latency (standing in for the replica actually doing
// work), unless done is closed first, in which case it returns
// immediately and records that it observed the cancellation.
func (m *MockReplica) Replica(done <-chan struct{}) (string, error) {
	timer := time.NewTimer(m.latency)
	defer timer.Stop()

	select {
	case <-timer.C:
		m.mu.Lock()
		m.completed = true
		m.mu.Unlock()
		return m.value, m.err

	case <-done:
		m.mu.Lock()
		m.observedCancel = true
		m.mu.Unlock()
		return "", nil
	}
}

// ObservedCancellation reports whether this replica's done channel
// was closed and it reacted by exiting early, rather than running to
// completion regardless.
func (m *MockReplica) ObservedCancellation() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.observedCancel
}

// RanToCompletion reports whether this replica's artificial latency
// fully elapsed and it returned its result normally, without ever
// having been cancelled.
func (m *MockReplica) RanToCompletion() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completed
}
