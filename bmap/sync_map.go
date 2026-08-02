package bmap

import "sync"

// SyncMap is a generic wrapper around sync.Map that provides type safety.
type SyncMap[K comparable, V any] struct {
	m sync.Map
}

// NewSyncMap creates and returns a new SyncMap instance.
func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: sync.Map{},
	}
}

// Store sets the value for a key.
func (sm *SyncMap[K, V]) Store(k K, v V) {
	sm.m.Store(k, v)
}

// Load returns the value stored in the map for a key, or zero if no value is present.
// The ok result indicates whether value was found in the map.
func (sm *SyncMap[K, V]) Load(k K) (V, bool) {
	val, ok := sm.m.Load(k)
	if !ok {
		var zero V
		return zero, false
	}
	return val.(V), true
}

// Delete deletes the value for a key.
func (sm *SyncMap[K, V]) Delete(k K) {
	sm.m.Delete(k)
}

// Len returns the number of entries in the map.
// Note: This operation iterates the entire map and has O(n) complexity.
func (sm *SyncMap[K, V]) Len() int {
	count := 0
	sm.m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Range calls fn sequentially for each key and value present in the map.
// If fn returns false, range stops the iteration.
func (sm *SyncMap[K, V]) Range(fn func(k K, v V) bool) {
	sm.m.Range(func(key, value any) bool {
		return fn(key.(K), value.(V))
	})
}
