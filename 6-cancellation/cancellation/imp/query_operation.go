package imp

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// QueryOperation represents an ongoing database query.
// It allows for waiting on the query's completion and explicitly canceling it.
type QueryOperation interface {
	// Wait blocks until the query completes successfully or with an error.
	// It returns the results (e.g., *simulatedRows) and any error.
	Wait() (*simulatedRows, error)

	// Cancel attempts to interrupt the ongoing query.
	// This method should be safe to call multiple times or if the query has already finished.
	Cancel() error
}

// mockQueryOperation is a concrete implementation of QueryOperation for testing.
type mockQueryOperation struct {
	query        string
	result       *simulatedRows
	opErr        error
	finished     chan struct{} // Closed when the operation completes (successfully or with error)
	cancelSignal chan struct{} // Used to signal cancellation to the running operation goroutine
	mu           sync.Mutex    // Protects access to result and opErr
	canceled     bool
}

// run simulates the actual database query execution.
func (op *mockQueryOperation) run(ctx context.Context) {
	defer close(op.finished) // Ensure 'finished' is always closed

	// Simulate query execution time
	queryDuration := 3 * time.Second // Default query duration
	if op.query == "FAST QUERY" {
		queryDuration = 500 * time.Millisecond // Iteration faster query
	}

	log.Printf("Mock QueryOperation: Starting execution for '%s' (will take %v)", op.query, queryDuration)

	select {
	case <-time.After(queryDuration):
		// Query completed successfully
		op.mu.Lock()
		op.result = &simulatedRows{data: []string{"data_for_" + op.query}}
		op.opErr = nil
		op.mu.Unlock()
		log.Printf("Mock QueryOperation: Query '%s' completed successfully.", op.query)
	case <-op.cancelSignal:
		// Cancellation requested by the caller
		op.mu.Lock()
		op.opErr = context.Canceled // Or a custom driver-specific cancellation error
		op.canceled = true
		op.mu.Unlock()
		log.Printf("Mock QueryOperation: Query '%s' was explicitly canceled by the caller.", op.query)
	case <-ctx.Done():
		// Context itself was canceled (e.g., timeout, parent context cancel)
		op.mu.Lock()
		op.opErr = ctx.Err() // This will be context.Canceled or context.DeadlineExceeded
		op.canceled = true
		op.mu.Unlock()
		log.Printf("Mock QueryOperation: Query '%s' interrupted due to context cancellation: %v", op.query, ctx.Err())
	}
}

// Wait blocks until the query completes.
func (op *mockQueryOperation) Wait() (*simulatedRows, error) {
	<-op.finished // Wait for the operation to complete
	op.mu.Lock()
	defer op.mu.Unlock()
	return op.result, op.opErr
}

// Cancel attempts to interrupt the ongoing query by sending a signal.
func (op *mockQueryOperation) Cancel() error {
	op.mu.Lock()
	if op.canceled { // Already canceled or finished by context
		op.mu.Unlock()
		log.Printf("Mock QueryOperation: Attempted to cancel '%s' but it was already cancelled/finished.", op.query)
		return nil // Or return a specific error if you want to differentiate
	}
	op.mu.Unlock()

	select {
	case op.cancelSignal <- struct{}{}: // Send a cancellation signal
		log.Printf("Mock QueryOperation: Sent explicit cancel signal for '%s'.", op.query)
		return nil
	case <-op.finished:
		// Operation already finished before we could send the cancel signal
		log.Printf("Mock QueryOperation: Attempted to cancel '%s' but it already finished.", op.query)
		return nil
	default:
		// Should not happen if the buffer is 1 and handled correctly
		log.Printf("Mock QueryOperation: Failed to send cancel signal for '%s'. Channel blocked or already sent.", op.query)
		return errors.New("failed to send cancel signal")
	}
}
