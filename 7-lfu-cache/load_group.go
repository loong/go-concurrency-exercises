package main

import (
	"container/list"
	"sync"
)

type LoadGroup struct {
	calls map[string]*call
	mutex sync.Mutex
	cache *Cache
}

type call struct {
	mu     sync.RWMutex
	result *string
	err    *error
}

// Get ensures one loading task is run even if multiple threads are waiting on the same key
// /*
func (l *LoadGroup) Get(key string, loaderFunc LoaderFunc) (string, error) {
	l.mutex.Lock()
	cache := *(l.cache)
	vc, err := cache.GetWithoutLoad(key)
	if err != nil {
		//
	}

	if len(vc) != 0 {
		l.mutex.Unlock()
		return vc, nil
	}

	if call, ok := l.calls[key]; ok {
		l.mutex.Unlock()

		call.mu.RLock()
		result := call.result
		call.mu.RUnlock()
		return *result, nil
	}

	call := &call{
		result: new(string),
	}

	l.calls[key] = call
	call.mu.Lock()
	l.mutex.Unlock()

	// TODO: handling panic
	v, err := loaderFunc(key)
	if err != nil {

	}
	call.result = &v
	call.mu.Unlock()

	// Remove call and update cache
	l.mutex.Lock()
	err = cache.Set(key, v)
	if err != nil {
		// TODO: handling error
		l.mutex.Unlock()
		return "", err
	}

	delete(l.calls, key)
	l.mutex.Unlock()
}
