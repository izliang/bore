package bmap

import "testing"

func TestSyncMap(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*SyncMap[string, int])
		verify   func(*testing.T, *SyncMap[string, int])
	}{
		{
			name: "Store and Load",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
				sm.Store("b", 2)
			},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if v, ok := sm.Load("a"); !ok || v != 1 {
					t.Errorf("Load('a') = %v, %v; want 1, true", v, ok)
				}
				if v, ok := sm.Load("b"); !ok || v != 2 {
					t.Errorf("Load('b') = %v, %v; want 2, true", v, ok)
				}
			},
		},
		{
			name: "Load non-existent key",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
			},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if v, ok := sm.Load("nonexistent"); ok || v != 0 {
					t.Errorf("Load('nonexistent') = %v, %v; want 0, false", v, ok)
				}
			},
		},
		{
			name: "Update existing key",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
				sm.Store("a", 100)
			},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if v, ok := sm.Load("a"); !ok || v != 100 {
					t.Errorf("Load('a') after update = %v, %v; want 100, true", v, ok)
				}
			},
		},
		{
			name: "Delete existing key",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
				sm.Delete("a")
			},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if v, ok := sm.Load("a"); ok || v != 0 {
					t.Errorf("Load('a') after delete = %v, %v; want 0, false", v, ok)
				}
			},
		},
		{
			name: "Delete non-existent key",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
				sm.Delete("nonexistent")
			},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if v, ok := sm.Load("a"); !ok || v != 1 {
					t.Errorf("Load('a') = %v, %v; want 1, true", v, ok)
				}
			},
		},
		{
			name: "Len on empty map",
			setup: func(sm *SyncMap[string, int]) {},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if l := sm.Len(); l != 0 {
					t.Errorf("Len() = %d; want 0", l)
				}
			},
		},
		{
			name: "Len after storing",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
				sm.Store("b", 2)
				sm.Store("c", 3)
			},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if l := sm.Len(); l != 3 {
					t.Errorf("Len() = %d; want 3", l)
				}
			},
		},
		{
			name: "Len after delete",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
				sm.Store("b", 2)
				sm.Delete("a")
			},
			verify: func(t *testing.T, sm *SyncMap[string, int]) {
				if l := sm.Len(); l != 1 {
					t.Errorf("Len() = %d; want 1", l)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewSyncMap[string, int]()
			tt.setup(sm)
			tt.verify(t, sm)
		})
	}
}

func TestSyncMapRange(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*SyncMap[string, int])
		wantKeys []string
	}{
		{
			name: "Range over all elements",
			setup: func(sm *SyncMap[string, int]) {
				sm.Store("a", 1)
				sm.Store("b", 2)
				sm.Store("c", 3)
			},
			wantKeys: []string{"a", "b", "c"},
		},
		{
			name:     "Range over empty map",
			setup:    func(sm *SyncMap[string, int]) {},
			wantKeys: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewSyncMap[string, int]()
			tt.setup(sm)

			gotKeys := make([]string, 0)
			sm.Range(func(k string, v int) bool {
				gotKeys = append(gotKeys, k)
				return true
			})

			// Since sync.Map order is not guaranteed, compare as sets
			if len(gotKeys) != len(tt.wantKeys) {
				t.Errorf("Range() got %d keys; want %d", len(gotKeys), len(tt.wantKeys))
			}

			wantSet := make(map[string]bool)
			for _, k := range tt.wantKeys {
				wantSet[k] = true
			}
			for _, k := range gotKeys {
				if !wantSet[k] {
					t.Errorf("Range() got unexpected key %q", k)
				}
			}
		})
	}
}

func TestSyncMapRangeStopEarly(t *testing.T) {
	sm := NewSyncMap[string, int]()
	sm.Store("a", 1)
	sm.Store("b", 2)
	sm.Store("c", 3)

	count := 0
	sm.Range(func(k string, v int) bool {
		count++
		return count < 2 // Stop after 2 elements
	})

	if count != 2 {
		t.Errorf("Range stopped after %d iterations; want 2", count)
	}
}

func TestSyncMapDifferentTypes(t *testing.T) {
	type testStruct struct {
		Value string
	}

	tests := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "Int key and struct value",
			setup: func(t *testing.T) {
				sm := NewSyncMap[int, testStruct]()
				sm.Store(1, testStruct{Value: "test"})
				v, ok := sm.Load(1)
				if !ok || v.Value != "test" {
					t.Errorf("Unexpected result: %v, %v", v, ok)
				}
			},
		},
		{
			name: "Pointer value",
			setup: func(t *testing.T) {
				sm := NewSyncMap[string, *testStruct]()
				s := &testStruct{Value: "ptr"}
				sm.Store("key", s)
				v, ok := sm.Load("key")
				if !ok || v != s || v.Value != "ptr" {
					t.Errorf("Unexpected result: %v, %v", v, ok)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.setup)
	}
}
