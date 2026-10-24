package main

import (
	"errors"
	"testing"
	"time"
)

func TestSessionManagersCreationAndUpdate(t *testing.T) {
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
}

func TestSessionManagersCleaner(t *testing.T) {
	m := NewSessionManager()
	sID, err := m.CreateSession()
	if err != nil {
		t.Error("Error CreateSession:", err)
	}

	// Note that the cleaner is only running every 5s
	time.Sleep(7 * time.Second)
	_, err = m.GetSessionData(sID)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Error("Session still in memory after 7 seconds")
	}
}

func TestSessionManagersCleanerAfterUpdate(t *testing.T) {
	m := NewSessionManager()
	sID, err := m.CreateSession()
	if err != nil {
		t.Error("Error CreateSession:", err)
	}

	time.Sleep(3 * time.Second)

	err = m.UpdateSessionData(sID, make(map[string]any))
	if err != nil {
		t.Error("Error UpdateSessionData:", err)
	}

	time.Sleep(3 * time.Second)

	_, err = m.GetSessionData(sID)
	if errors.Is(err, ErrSessionNotFound) {
		t.Error("Session not found although has been updated 3 seconds earlier.")
	}

	time.Sleep(4 * time.Second)
	_, err = m.GetSessionData(sID)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Error("Session still in memory 7 seconds after update")
	}
}
