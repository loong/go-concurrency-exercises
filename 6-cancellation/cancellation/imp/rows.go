package imp

import (
	"errors"
	"log"
	"sync"
)

// simulatedRows represents a simplified result set
type simulatedRows struct {
	data []string
	idx  int
	mu   sync.Mutex // Protects access to data/idx
}

func (sr *simulatedRows) Next() bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.idx < len(sr.data) {
		sr.idx++
		return true
	}
	return false
}

func (sr *simulatedRows) Scan(dest ...interface{}) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.idx-1 < len(sr.data) {
		if len(dest) != 1 {
			return errors.New("simulatedRows.Scan expects 1 destination")
		}
		if s, ok := dest[0].(*string); ok {
			*s = sr.data[sr.idx-1]
			return nil
		}
		return errors.New("simulatedRows.Scan: unsupported destination type")
	}
	return errors.New("simulatedRows.Scan: no more rows")
}

func (sr *simulatedRows) Close() error {
	log.Println("SimulatedRows: Closed.")
	return nil
}
