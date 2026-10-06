package main

import (
	"container/list"
	"errors"
	"sync"
)

var EMPTY_ERROR = errors.New("EMPTY ERROR")

type Cache interface {
	Get(key string) (string, error)
	GetWithoutLoad(key string) (string, error)
	Set(key, value string) error
}

type (
	LoaderFunc func(key string) (string, error)
)

type baseCache struct {
	mu         sync.Mutex
	size       int
	loaderFunc LoaderFunc
	loadGroup  LoadGroup
}

type LFUCache struct {
	baseCache
	cache map[string]*lfuItem
	list  *list.List // list of lruEntry
}

type lruEntry struct {
	freq  int
	items map[*lfuItem]struct{}
}

type lfuItem struct {
	value string
	key   string
	el    *list.Element // Reference to lruEntry
}

func NewLFUCache(size int, loaderFunc LoaderFunc) (*LFUCache, error) {
	if size <= 0 {
		return nil, errors.New("size must be greater than zero")
	}

	cache := &LFUCache{
		cache: make(map[string]*lfuItem),
		list:  list.New(),
	}

	cache.baseCache.size = size
	cache.baseCache.loaderFunc = loaderFunc
	cache.baseCache.loadGroup = LoadGroup{}

	return cache, nil
}

// For testing
func (cache *LFUCache) GetBuckets() *list.List {
	return cache.list
}

func (cache *LFUCache) GetFreq(buckets *list.Element) int {
	return buckets.Value.(*lruEntry).freq
}

func (cache *LFUCache) Get(key string) (string, error) {
	cache.mu.Lock()
	if item, ok := cache.cache[key]; ok {
		// Move item to the higher bucket
		v := item.value
		err := cache.moveToHigherBucket(item)
		if err != nil {
			return "", err
		}
		cache.mu.Unlock()

		return v, nil
	}
	cache.mu.Unlock()

	// Miss, so load value
	value, err := cache.loaderFunc(key)
	if err != nil {
		return "", err
	}

	err = cache.Set(key, value)
	if err != nil {
		return "", err
	}

	return value, nil
}

func load(key string) (string, error) {

}

func (cache *LFUCache) GetKeys() []string {
	keys := make([]string, 0)
	for k, _ := range cache.cache {
		keys = append(keys, k)
	}

	return keys
}

func (cache *LFUCache) Set(key, value string) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if item, ok := cache.cache[key]; ok {
		item.value = value
		return nil
	}

	if len(cache.cache) >= cache.size {
		err := cache.evict()
		if err != nil && !errors.Is(err, EMPTY_ERROR) {
			return err
		}
	}

	cache.insert(key, value)
	return nil
}

// insert inserts key, value knowing that there is always slot for it
func (cache *LFUCache) insert(key, value string) {
	insertedItem := &lfuItem{
		value: value,
		key:   key,
	}

	cache.cache[key] = insertedItem

	var firstEntry *lruEntry
	var firstElement *list.Element
	if cache.list.Front() == nil || cache.list.Front().Value.(*lruEntry).freq != 0 {
		firstEntry = &lruEntry{
			freq:  0,
			items: make(map[*lfuItem]struct{}),
		}

		firstElement = cache.list.PushFront(firstEntry)
	} else {
		firstElement = cache.list.Front()
		firstEntry = firstElement.Value.(*lruEntry)
	}

	firstEntry.items[insertedItem] = struct{}{}
	insertedItem.el = firstElement
}

func getItemToEvict(mapp map[*lfuItem]struct{}) (*lfuItem, error) {
	for key, _ := range mapp {
		return key, nil
	}

	return nil, EMPTY_ERROR
}

func (cache *LFUCache) evict() error {
	zeroBucket := cache.list.Front()
	if zeroBucket == nil {
		return EMPTY_ERROR
	}

	items := zeroBucket.Value.(*lruEntry).items
	itemToRemove, err := getItemToEvict(items)
	if err != nil {
		return err
	}

	delete(items, itemToRemove)
	if len(items) == 0 {
		cache.list.Remove(zeroBucket)
	}

	delete(cache.cache, itemToRemove.key)

	return nil
}

func (cache *LFUCache) moveToHigherBucket(item *lfuItem) error {
	if item == nil {
		return errors.New("item is nil")
	}

	curBucket := item.el
	curBucketEntry := curBucket.Value.(*lruEntry)
	nextFreq := curBucketEntry.freq + 1
	delete(curBucketEntry.items, item)

	var nextBucket *list.Element
	if item.el.Next() == nil || item.el.Next().Value.(*lruEntry).freq > nextFreq {
		nextBucketEntry := &lruEntry{
			freq:  nextFreq,
			items: make(map[*lfuItem]struct{}),
		}

		nextBucketEntry.items[item] = struct{}{}
		nextBucket = cache.list.InsertAfter(nextBucketEntry, item.el)
	} else {
		nextBucket = item.el.Next()
		nextBucketEntry := nextBucket.Value.(*lruEntry)
		nextBucketEntry.items[item] = struct{}{}
	}

	item.el = nextBucket

	// Remove last bucket in case it is empty
	if len(curBucketEntry.items) == 0 {
		cache.list.Remove(curBucket)
	}

	return nil
}
