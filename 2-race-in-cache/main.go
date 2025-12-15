package main

import (
	"container/list"
	"fmt"
	"sync"
	"testing"
)

const CacheSize = 100

type KeyStoreCacheLoader interface {
	// Load implements a function where the cache should gets it's content from
	Load(string) string
}

// page represents an item in our cache.
type page struct {
	Key   string
	Value string
}

// Future represents a pending or completed result for a key.
// It allows multiple goroutines to wait for the result of a single load operation.
type Future struct {
	wg     sync.WaitGroup         // Used to wait for the load to complete
	result *list.Element          // Pointer to the list element when done
	err    error                  // Any error during loading
	once   sync.Once              // Ensures load is called only once
	loader func() (string, error) // The function to perform the actual load
}

func newFuture(loader func() (string, error)) *Future {
	f := &Future{
		loader: loader,
	}
	f.wg.Add(1) // Initialize wait group for 1 completion
	return f
}

// Do performs the actual loading operation exactly once.
func (f *Future) Do() {
	f.once.Do(func() {
		// Simulate a time-consuming load operation
		val, err := f.loader()
		if err != nil {
			f.err = err
		} else {
			f.result = &list.Element{Value: &page{"", val}}
		}
		f.wg.Done() // Signal that loading is complete
	})
}

// Wait blocks until the future's operation is complete and returns the result.
func (f *Future) Wait() (*list.Element, error) {
	f.wg.Wait()
	return f.result, f.err
}

// SetResult sets the list.Element once the loading is done and added to the list.
func (f *Future) SetResult(e *list.Element) {
	f.result = e
}

// KeyStoreCache implements a concurrent LRU cache.
type KeyStoreCache struct {
	mu    sync.RWMutex            // Guards access to cache and pages
	cache map[string]*Future      // Maps key to its Future (pending or completed)
	pages *list.List              // Doubly linked list for LRU eviction
	load  func(key string) string // The actual resource loading function
}

// NewKeyStoreCache creates a new concurrent LRU cache.
func New(load KeyStoreCacheLoader) *KeyStoreCache {
	return &KeyStoreCache{
		cache: make(map[string]*Future),
		pages: list.New(),
		load:  load.Load,
	}
}

// Get retrieves a value from the cache, loading it if necessary.
func (k *KeyStoreCache) Get(key string) string {
	// --- Phase 1: Check for existing entry (read-locked) ---
	k.mu.RLock() // Acquire a read lock
	f, ok := k.cache[key]
	k.mu.RUnlock() // Release read lock quickly

	if ok {
		elem, err := f.Wait() // This blocks if the future is not yet done
		if err != nil {
			// Handle load error here if you want to propagate it
			fmt.Printf("Error loading key '%s': %v\n", key, err)
			return "" // Or re-attempt load, or return a specific error
		}

		k.mu.Lock()
		k.pages.MoveToFront(elem)
		k.mu.Unlock()

		return elem.Value.(*page).Value
	}

	k.mu.Lock()
	f, ok = k.cache[key]
	if ok {
		// Another goroutine beat us to it. Release lock and wait for its result.
		k.mu.Unlock()
		elem, err := f.Wait()
		if err != nil {
			fmt.Printf("Error loading key '%s': %v\n", key, err)
			return ""
		}
		k.mu.Lock() // Re-acquire lock to move to front
		k.pages.MoveToFront(elem)
		k.mu.Unlock()
		return elem.Value.(*page).Value
	}

	// It's genuinely not in the cache. Create a new future.
	newF := newFuture(func() (string, error) {
		// The actual load operation that will be called by Do()
		val := k.load(key)
		return val, nil // Assuming k.load doesn't return an error, adjust if it does
	})
	k.cache[key] = newF
	k.mu.Unlock() // Release the write lock *before* calling Do()

	newF.Do() // This will call the loader function for this key exactly once.

	// Now that loading is complete, acquire write lock again to update LRU and set result.
	k.mu.Lock()
	defer k.mu.Unlock() // Ensure lock is released

	// Check for eviction before adding the new item
	if k.pages.Len() >= CacheSize {
		oldest := k.pages.Back()
		if oldest != nil {
			pToDelete := oldest.Value.(*page)
			delete(k.cache, pToDelete.Key) // Remove from map
			k.pages.Remove(oldest)         // Remove from list
			fmt.Printf("Evicting key: %s\n", pToDelete.Key)
		}
	}

	// Get the loaded result from the future
	loadedElem, err := newF.Wait() // This should return immediately now as Do() just completed.
	if err != nil {
		// Handle the error (e.g., remove from cache if load failed permanently)
		delete(k.cache, key)
		fmt.Printf("Final error after load for key '%s': %v\n", key, err)
		return ""
	}

	// Add the new page to the front of the list and set its result in the future.
	p := &page{key, loadedElem.Value.(*page).Value} // Re-create page to get its value
	elem := k.pages.PushFront(p)
	newF.SetResult(elem) // Set the actual list.Element in the future for future lookups

	return p.Value
}

// Loader implements KeyStoreLoader
type Loader struct {
	DB *MockDB
}

// Load gets the data from the database
func (l *Loader) Load(key string) string {
	val, err := l.DB.Get(key)
	if err != nil {
		panic(err)
	}

	return val
}

func run(t *testing.T) (*KeyStoreCache, *MockDB) {
	loader := Loader{
		DB: GetMockDB(),
	}
	cache := New(&loader)

	RunMockServer(cache, t)

	return cache, loader.DB
}

func main() {
	run(nil)
}
