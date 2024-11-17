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

import "time"

// User defines the UserModel. Use this to check whether a User is a
// Premium user or not
type User struct {
	ID        int
	IsPremium bool
	TimeUsed  int64 // in seconds
}

// HandleRequest runs the processes requested by users. Returns false
// if process had to be killed
func HandleRequest(process func(), u *User) bool {
	var (
		startReqProc, endReqProc time.Time
	)

	done := make(chan struct{})

	go func() {
		startReqProc = time.Now()
		process()
		endReqProc = time.Now()
		done <- struct{}{}
	}()

	if u.IsPremium {
		<-done
		return true
	}

	select {
	case <-time.After(10 * time.Second):
		return false
	case <-done:
		diff := endReqProc.Sub(startReqProc).Seconds()
		u.TimeUsed += int64(diff)
		if u.TimeUsed <= 10 {
			return true
		}
		return false
	}

}

func main() {
	RunMockServer()
}
