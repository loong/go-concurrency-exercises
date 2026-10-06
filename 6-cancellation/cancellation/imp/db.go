package imp

import (
	"context"
	"log"
)

// -----------------------------------------------------------------------------
// Your db instance (that uses the EmulatedDriver)
// -----------------------------------------------------------------------------

// YourDB is your custom database instance that would use the EmulatedDriver.
type YourDB struct {
	driver EmulatedDriver
}

// NewYourDB creates a new instance of YourDB with the provided driver.
func NewYourDB(driver EmulatedDriver) *YourDB {
	return &YourDB{driver: driver}
}

// QueryContext is your implementation of the database query method that
// supports context cancellation.
func (db *YourDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*simulatedRows, error) {
	queryOperation, err := db.driver.PrepareQuery(ctx, query, args)
	if err != nil {
		return nil, err
	}

	resChannel := make(chan *simulatedRows)
	errChannel := make(chan error)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		res, err := queryOperation.Wait()
		select {
		case <-ctx.Done():
			log.Printf("Sub-goroutine for '%s': Context canceled. Not sending result/error.", query)
			return // Exit the goroutine cleanly
		default:
			if err != nil {
				errChannel <- err
			} else {
				resChannel <- res
			}
		}
	}()

	var res *simulatedRows
	select {
	case <-ctx.Done():
		{
			close(resChannel)
			close(errChannel)
			err := queryOperation.Cancel()
			if err != nil {
				return nil, err
			}

			<-finished
			return nil, ctx.Err()
		}
	case res = <-resChannel:
		{
			return res, nil
		}
	case err := <-errChannel:
		{
			return nil, err
		}
	}

	return nil, nil
}
