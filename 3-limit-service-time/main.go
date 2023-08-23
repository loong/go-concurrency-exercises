//////////////////////////////////////////////////////////////////////
//
// Your video processing service has a freemium model. Everyone has 10
// sec of free processing time on your service. After that, the
// service will kill your process, unless you are a paid premium user.
//
// Beginner Level: 10s max per request
// Advanced Level: 10s max per user (accumulated)
//

package main

import (
	"sync/atomic"
	"time"
)

const FreemiumQuota int64 = 10

// User defines the UserModel. Use this to check whether a User is a
// Premium user or not
type User struct {
	ID        int
	IsPremium bool
	TimeUsed  int64 // in seconds
}

// addTime adds a second to the total time used
func (u *User) addSecond() {
	atomic.AddInt64(&u.TimeUsed, 1)
}

// mayContinueToView
func (u *User) exceededQuota() bool {
	return !u.IsPremium && u.TimeUsed >= FreemiumQuota
}

// HandleRequest runs the processes requested by users. Returns false
// if process had to be killed
func HandleRequest(process func(), u *User) bool {
	ticker := time.NewTicker(1 * time.Second)
	done := make(chan bool)
	go func() {
		process()
		done <- true
	}()
	for {
		select {
		case <-ticker.C:
			u.addSecond()
			if u.exceededQuota() {
				return false
			}
		case <-done:
			return true
		}
	}
}

func main() {
	RunMockServer()
}
