//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"sync/atomic"
	"time"
)

// MockDB used to simulate a database model
type MockDB struct {
	Calls int32
}

// Get only returns the key, as this is only for demonstration purposes
func (db *MockDB) Get(key string) (string, error) {
	time.Sleep(20 * time.Millisecond)
	atomic.AddInt32(&db.Calls, 1)
	return key, nil
}

// GetMockDB returns an instance of MockDB
func GetMockDB() *MockDB {
	return &MockDB{
		Calls: 0,
	}
}
