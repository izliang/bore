package bmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

// OrderMap is a generic ordered map that preserves insertion order.
// When updating an existing key, the key is moved to the end of the order.
type OrderMap[K comparable, V any] struct {
	orderMu sync.RWMutex
	arr     []K
	value   sync.Map
	locks   sync.Map
}

// NewOrderMap creates and returns a new empty OrderMap.
func NewOrderMap[K comparable, V any]() *OrderMap[K, V] {
	return &OrderMap[K, V]{
		arr: []K{},
	}
}

func (o *OrderMap[K, V]) keyLock(k K) *sync.RWMutex {
	lock, _ := o.locks.LoadOrStore(k, &sync.RWMutex{})
	return lock.(*sync.RWMutex)
}

func (o *OrderMap[K, V]) load(k K) (V, bool) {
	val, ok := o.value.Load(k)
	if !ok {
		var zero V
		return zero, false
	}
	return val.(V), true
}

// Len returns the number of elements in the map.
func (o *OrderMap[K, V]) Len() int {
	o.orderMu.RLock()
	defer o.orderMu.RUnlock()

	return len(o.arr)
}

// Keys returns all keys in insertion order.
func (o *OrderMap[K, V]) Keys() []K {
	o.orderMu.RLock()
	defer o.orderMu.RUnlock()

	result := make([]K, len(o.arr))
	copy(result, o.arr)
	return result
}

// Values returns all values in the order corresponding to their keys' insertion order.
func (o *OrderMap[K, V]) Values() []V {
	o.orderMu.RLock()
	defer o.orderMu.RUnlock()

	result := make([]V, 0, len(o.arr))
	for _, k := range o.arr {
		if v, ok := o.load(k); ok {
			result = append(result, v)
		}
	}
	return result
}

// Get returns the value for a key and whether the key exists.
func (o *OrderMap[K, V]) Get(k K) (v V, ok bool) {
	lock := o.keyLock(k)
	lock.RLock()
	defer lock.RUnlock()

	return o.load(k)
}

// Set sets the value for a key. If the key already exists,
// it is moved to the end of the insertion order.
func (o *OrderMap[K, V]) Set(k K, v V) {
	lock := o.keyLock(k)
	lock.Lock()
	defer lock.Unlock()

	o.orderMu.Lock()
	defer o.orderMu.Unlock()

	if _, exist := o.load(k); exist {
		// Remove the key from its current position
		for i, v := range o.arr {
			if v == k {
				o.arr = append(o.arr[:i], o.arr[i+1:]...)
				break
			}
		}
	}
	o.value.Store(k, v)
	o.arr = append(o.arr, k)
}

// Delete removes a key from the map.
func (o *OrderMap[K, V]) Delete(k K) {
	lock := o.keyLock(k)
	lock.Lock()
	defer lock.Unlock()

	o.orderMu.Lock()
	defer o.orderMu.Unlock()

	if _, exist := o.load(k); !exist {
		return
	}
	// Remove the key from the ordered slice
	for i, v := range o.arr {
		if v == k {
			o.arr = append(o.arr[:i], o.arr[i+1:]...)
			break
		}
	}
	o.value.Delete(k)
}

// IterateFunc iterates over the map in insertion order and calls fn for each entry.
// If fn returns an error, the iteration stops and the error is returned.
func (o *OrderMap[K, V]) IterateFunc(fn func(k K, v V) error) error {
	type entry struct {
		key   K
		value V
	}

	o.orderMu.RLock()
	entries := make([]entry, 0, len(o.arr))
	for _, k := range o.arr {
		v, ok := o.load(k)
		if !ok {
			o.orderMu.RUnlock()
			return fmt.Errorf("no value for key %v", k)
		}
		entries = append(entries, entry{key: k, value: v})
	}
	o.orderMu.RUnlock()

	for _, entry := range entries {
		if err := fn(entry.key, entry.value); err != nil {
			return fmt.Errorf("iterate ordermap error. key: %v, err: %v", entry.key, err)
		}
	}
	return nil
}

// MarshalJSON implements ordered JSON serialization preserving insertion order.
func (o *OrderMap[K, V]) MarshalJSON() ([]byte, error) {
	type entry struct {
		key   K
		value V
	}

	o.orderMu.RLock()
	entries := make([]entry, 0, len(o.arr))
	for _, key := range o.arr {
		if v, ok := o.load(key); ok {
			entries = append(entries, entry{key: key, value: v})
		}
	}
	o.orderMu.RUnlock()

	var buf bytes.Buffer
	buf.WriteByte('{')

	for i, entry := range entries {
		if i > 0 {
			buf.WriteByte(',')
		}

		keyJS, err := json.Marshal(entry.key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJS)
		buf.WriteByte(':')

		valJS, err := json.Marshal(entry.value)
		if err != nil {
			return nil, err
		}
		buf.Write(valJS)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON implements ordered JSON deserialization preserving JSON key order.
func (o *OrderMap[K, V]) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))

	// Verify opening '{'
	t, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected '{', got %v", t)
	}

	for decoder.More() {
		// Parse key
		t, err := decoder.Token()
		if err != nil {
			return err
		}

		// Direct key conversion using unsafe to avoid Marshal/Unmarshal overhead
		var key K
		switch v := t.(type) {
		case string:
			key = *(*K)(unsafe.Pointer(&v))
		default:
			return fmt.Errorf("unexpected key type: %T", t)
		}

		// Parse value (supports nested structures)
		var val V
		if err := decoder.Decode(&val); err != nil {
			return err
		}

		// Store and maintain order
		o.Set(key, val)
	}

	return nil
}
