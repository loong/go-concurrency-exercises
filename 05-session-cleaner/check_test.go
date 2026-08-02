package main

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestSessionManagersCreationAndUpdate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create manager and new session
		m := NewSessionManager()
		sID, err := m.CreateSession()
		if err != nil {
			t.Error("Error CreateSession:", err)
		}

		data, err := m.GetSessionData(sID)
		if err != nil {
			t.Error("Error GetSessionData:", err)
		}

		// Modify and update data
		data["website"] = "longhoang.de"
		err = m.UpdateSessionData(sID, data)
		if err != nil {
			t.Error("Error UpdateSessionData:", err)
		}

		// Retrieve data from manager again
		data, err = m.GetSessionData(sID)
		if err != nil {
			t.Error("Error GetSessionData:", err)
		}

		if data["website"] != "longhoang.de" {
			t.Error("Expected website to be longhoang.de")
		}
	})
}

func TestSessionManagersCleaner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := NewSessionManager()
		sID, err := m.CreateSession()
		if err != nil {
			t.Error("Error CreateSession:", err)
		}

		// Note that the cleaner is only running every 5s
		time.Sleep(7 * time.Second)
		_, err = m.GetSessionData(sID)
		if err != ErrSessionNotFound {
			t.Error("Session still in memory after 7 seconds")
		}
	})
}

func TestSessionManagersCleanerAfterUpdate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := NewSessionManager()
		sID, err := m.CreateSession()
		if err != nil {
			t.Error("Error CreateSession:", err)
		}

		time.Sleep(3 * time.Second)

		err = m.UpdateSessionData(sID, make(map[string]interface{}))
		if err != nil {
			t.Error("Error UpdateSessionData:", err)
		}

		time.Sleep(3 * time.Second)

		_, err = m.GetSessionData(sID)
		if err == ErrSessionNotFound {
			t.Error("Session not found although has been updated 3 seconds earlier.")
		}

		time.Sleep(4 * time.Second)
		_, err = m.GetSessionData(sID)
		if err != ErrSessionNotFound {
			t.Error("Session still in memory 7 seconds after update")
		}
	})
}

// TestSessionManagerConcurrentAccess hammers CreateSession,
// UpdateSessionData and GetSessionData from many goroutines at once to
// catch unsynchronized access to the sessions map (run with `go test
// -race`). It stays on the real clock rather than synctest: the goroutines
// below spin on plain CPU work with no blocking call in the loop body, so
// they never become "durably blocked" - inside a bubble the fake clock
// would simply never advance and the test would spin forever instead of
// finishing. A fixed iteration count (rather than a wall-clock deadline)
// gets the same determinism win synctest would have provided, without
// needing a bubble.
func TestSessionManagerConcurrentAccess(t *testing.T) {
	m := NewSessionManager()

	const numSessions = 20
	const iterationsPerGoroutine = 2000
	sessionIDs := make([]string, numSessions)
	for i := range sessionIDs {
		sID, err := m.CreateSession()
		if err != nil {
			t.Fatal("Error CreateSession:", err)
		}
		sessionIDs[i] = sID
	}

	var wg sync.WaitGroup

	for _, sID := range sessionIDs {
		sID := sID

		wg.Add(3)

		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				_, _ = m.GetSessionData(sID)
			}
		}()

		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				_ = m.UpdateSessionData(sID, make(map[string]interface{}))
			}
		}()

		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				_, _ = m.CreateSession()
			}
		}()
	}

	wg.Wait()
}
