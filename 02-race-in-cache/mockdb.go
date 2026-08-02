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
type MockDB struct{
	Calls int32
}

// Get only returns the key, as this is only for demonstration purposes
func (db *MockDB) Get(key string) (string, error) {
	d, _ := time.ParseDuration("20ms")
	time.Sleep(d)
	atomic.AddInt32(&db.Calls, 1)
	return key, nil
}

// GetMockDB returns an instance of MockDB
func GetMockDB() *MockDB {
	return &MockDB{
		Calls: 0,
	}
}
