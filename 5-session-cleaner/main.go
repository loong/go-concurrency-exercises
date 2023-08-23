//////////////////////////////////////////////////////////////////////
//
// Given is a SessionManager that stores session information in
// memory. The SessionManager itself is working, however, since we
// keep on adding new sessions to the manager our program will
// eventually run out of memory.
//
// Your task is to implement a session cleaner routine that runs
// concurrently in the background and cleans every session that
// hasn't been updated for more than 5 seconds (of course usually
// session times are much longer).
//
// Note that we expect the session to be removed anytime between 5 and
// 7 seconds after the last update. Also, note that you have to be
// very careful in order to prevent race conditions.
//

package main

import (
	"errors"
	"log"
	"sync"
	"time"
)

const SessionTtl = 5

// SessionManager keeps track of all sessions from creation, updating
// to destroying.
type SessionManager struct {
	sessions     map[string]Session
	sessionsLock sync.Locker
}

// Session stores the session's data
type Session struct {
	Data       map[string]interface{}
	expiration time.Time
}

// NewSession creates a new session
func NewSession(data ...map[string]interface{}) Session {
	var newSessionData map[string]interface{}
	if len(data) == 0 || data[0] == nil {
		newSessionData = make(map[string]interface{})
	} else {
		newSessionData = data[0]
	}
	return Session{
		Data:       newSessionData,
		expiration: time.Now().Add(SessionTtl * time.Second),
	}
}

// isExpired returns true if the session has expired
func (s Session) isExpired() bool {
	return s.expiration.Before(time.Now())
}

// NewSessionManager creates a new sessionManager
func NewSessionManager() *SessionManager {
	m := &SessionManager{
		sessions:     make(map[string]Session),
		sessionsLock: new(sync.Mutex),
	}

	return m
}

// CreateSession creates a new session and returns the sessionID
func (m *SessionManager) CreateSession() (string, error) {
	sessionID, err := MakeSessionID()
	if err != nil {
		return "", err
	}

	m.sessions[sessionID] = NewSession()

	ticker := time.NewTicker(1 * time.Second)
	go func() {
		for {
			<-ticker.C
			m.purgeExpiredSessions()
		}
	}()

	return sessionID, nil
}

// ErrSessionNotFound returned when sessionID not listed in
// SessionManager
var ErrSessionNotFound = errors.New("SessionID does not exists")

// GetSessionData returns data related to session if sessionID is
// found, errors otherwise
func (m *SessionManager) GetSessionData(sessionID string) (map[string]interface{}, error) {
	m.sessionsLock.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	m.sessionsLock.Unlock()
	return session.Data, nil
}

// UpdateSessionData overwrites the old session data with the new one
func (m *SessionManager) UpdateSessionData(sessionID string, data map[string]interface{}) error {
	_, ok := m.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	// Hint: you should renew expiry of the session here
	m.sessionsLock.Lock()
	m.sessions[sessionID] = NewSession(data)
	m.sessionsLock.Unlock()
	return nil
}

// purgeExpiredSessions deletes all expired sessions
func (m *SessionManager) purgeExpiredSessions() {
	m.sessionsLock.Lock()
	for sessionID, session := range m.sessions {
		if session.isExpired() {
			go m.deleteSession(sessionID)
		}
	}
	m.sessionsLock.Unlock()
}

// deleteSession deletes a session
func (m *SessionManager) deleteSession(sessionID string) {
	m.sessionsLock.Lock()
	defer m.sessionsLock.Unlock()
	delete(m.sessions, sessionID)
}

func main() {
	// Create new sessionManager and new session
	m := NewSessionManager()
	sID, err := m.CreateSession()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Created new session with ID", sID)

	// Update session data
	data := make(map[string]interface{})
	data["website"] = "longhoang.de"

	err = m.UpdateSessionData(sID, data)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Update session data, set website to longhoang.de")

	// Retrieve data from manager again
	updatedData, err := m.GetSessionData(sID)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Get session data:", updatedData)
}
