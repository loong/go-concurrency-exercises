package imp

import (
	"context"
	"log"
)

// EmulatedDriver is the interface that your 'db' instance would use to interact with
// the underlying database driver.
type EmulatedDriver interface {
	// PrepareQuery initiates a query and returns a handle to the ongoing operation.
	// It does NOT block until the query completes.
	PrepareQuery(ctx context.Context, query string, args ...interface{}) (QueryOperation, error)
}

// -----------------------------------------------------------------------------
// Mock Implementation of the EmulatedDriver and QueryOperation
// -----------------------------------------------------------------------------

// mockEmulatedDriver is a concrete implementation of EmulatedDriver for testing.
type mockEmulatedDriver struct {
	// You might add a connection pool or other driver-level state here
}

// NewMockEmulatedDriver creates a new instance of the mock driver.
func NewMockEmulatedDriver() EmulatedDriver {
	return &mockEmulatedDriver{}
}

// PrepareQuery simulates preparing and starting a database query.
func (m *mockEmulatedDriver) PrepareQuery(ctx context.Context, query string, args ...interface{}) (QueryOperation, error) {
	log.Printf("Mock Driver: Preparing and starting query: '%s'", query)
	op := &mockQueryOperation{
		query:        query,
		finished:     make(chan struct{}),
		cancelSignal: make(chan struct{}, 1), // Buffered channel for non-blocking sends
	}

	go op.run(ctx) // Start the "query" in a goroutine
	return op, nil
}
