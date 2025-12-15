package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	cache, err := NewLFUCache(3, func(key string) (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = cache.Set("vu", "10")
	err = cache.Set("nghia", "20")
	err = cache.Set("luan", "5")

	value, err := cache.Get("vu")
	if value != "10" {
		t.Errorf("value should be 10, got %s", value)
	}

	value, err = cache.Get("nghia")
	if value != "20" {
		t.Errorf("value should be 20, got %s", value)
	}

	err = cache.Set("xanh", "30")

	keys := cache.GetKeys()
	if slices.Contains(keys, "luan") {
		t.Errorf("keys should not contain luan")
	}
}

func TestCache1(t *testing.T) {
	cache, err := NewLFUCache(3, func(key string) (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = cache.Set("vu", "10")
	err = cache.Set("nghia", "20")
	err = cache.Set("luan", "5")

	for i := 0; i < 10; i++ {
		cache.Get("vu")
	}

	for i := 0; i < 9; i++ {
		cache.Get("nghia")
	}

	for i := 0; i < 8; i++ {
		cache.Get("luan")
	}

	i := 8
	for e := cache.GetBuckets().Front(); e != nil; e = e.Next() {
		fmt.Printf("Value: %v (Type: %T)\n", e.Value, e.Value)
		bucketFreq := cache.GetFreq(e)
		if bucketFreq != i {
			t.Errorf("bucketFreq should be %d, got %d", i, bucketFreq)
		}
		i += 1
	}

	err = cache.Set("xanh", "30")
	keys := cache.GetKeys()
	if slices.Contains(keys, "luan") {
		t.Errorf("keys should not contain luan")
	}
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generaterandomstringMathrand(length int) string {
	if length <= 0 {
		return ""
	}

	// Use strings.Builder for efficient string concatenation.
	// It pre-allocates memory, avoiding multiple re-allocations.
	var sb strings.Builder
	sb.Grow(length) // Pre-allocate capacity for efficiency

	charsetLen := len(charset)
	for i := 0; i < length; i++ {
		// Pick a random index from the charset
		randomIndex := rand.Intn(charsetLen)
		// Append the character at that index
		sb.WriteByte(charset[randomIndex])
	}

	return sb.String()
}

// --- Test Main for Global Setup ---
func TestMain(m *testing.M) {
	// Seed the global random number generator once for all tests in this package.
	// This is CRUCIAL for reproducible random behavior across test runs.
	rand.New(rand.NewSource(time.Now().UnixNano()))

	// Run all tests
	code := m.Run()

	// Exit with the test result code
	os.Exit(code)
}

func TestCacheConcurrency(t *testing.T) {
	cache, _ := NewLFUCache(5, func(key string) (string, error) {
		return "", errors.New("Loader hasn't been implemented yet")
	})

	keyValueMap := []string{"vu", "nghia", "luan", "xanh", "orange", "thuong",
		"tien", "lemon", "durian", "rambutant", "pear", "mango", "apple"}

	var wg sync.WaitGroup
	maxSetOperations := 10000
	maxGetOperations := 5000
	// Setter
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < maxSetOperations; i++ {
				randomNumber := rand.Intn(len(keyValueMap)) + 0
				cache.Set(keyValueMap[randomNumber], generaterandomstringMathrand(5))
			}
		}()
	}

	// 5 getters
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < maxGetOperations; j++ {
				randomNumber := rand.Intn(len(keyValueMap)) + 0
				cache.Get(keyValueMap[randomNumber])
			}
		}()
	}

	wg.Wait()
}
