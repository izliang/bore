package bmap

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func TestOrderMapBasic(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*OrderMap[string, string])
		verify func(*testing.T, *OrderMap[string, string])
	}{
		{
			name: "Set and Get",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Set("2", "b")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				if v, ok := om.Get("1"); !ok || v != "a" {
					t.Errorf("Get('1') = %q, %v; want 'a', true", v, ok)
				}
				if v, ok := om.Get("2"); !ok || v != "b" {
					t.Errorf("Get('2') = %q, %v; want 'b', true", v, ok)
				}
			},
		},
		{
			name: "Get non-existent key",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				if v, ok := om.Get("nonexistent"); ok || v != "" {
					t.Errorf("Get('nonexistent') = %q, %v; want '', false", v, ok)
				}
			},
		},
		{
			name: "Len after Set",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Set("2", "b")
				om.Set("3", "c")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				if l := om.Len(); l != 3 {
					t.Errorf("Len() = %d; want 3", l)
				}
			},
		},
		{
			name:  "Len on empty map",
			setup: func(om *OrderMap[string, string]) {},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				if l := om.Len(); l != 0 {
					t.Errorf("Len() = %d; want 0", l)
				}
			},
		},
		{
			name: "Keys order preserved",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Set("2", "b")
				om.Set("3", "c")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				want := []string{"1", "2", "3"}
				if got := om.Keys(); !reflect.DeepEqual(got, want) {
					t.Errorf("Keys() = %v; want %v", got, want)
				}
			},
		},
		{
			name: "Values order preserved",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Set("2", "b")
				om.Set("3", "c")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				want := []string{"a", "b", "c"}
				if got := om.Values(); !reflect.DeepEqual(got, want) {
					t.Errorf("Values() = %v; want %v", got, want)
				}
			},
		},
		{
			name: "Update existing key moves to end",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Set("2", "b")
				om.Set("1", "updated")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				wantKeys := []string{"2", "1"}
				wantValues := []string{"b", "updated"}
				if got := om.Keys(); !reflect.DeepEqual(got, wantKeys) {
					t.Errorf("Keys() = %v; want %v", got, wantKeys)
				}
				if got := om.Values(); !reflect.DeepEqual(got, wantValues) {
					t.Errorf("Values() = %v; want %v", got, wantValues)
				}
				if v, _ := om.Get("1"); v != "updated" {
					t.Errorf("Get('1') = %q; want 'updated'", v)
				}
			},
		},
		{
			name: "Delete existing key",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Set("2", "b")
				om.Delete("1")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				if v, ok := om.Get("1"); ok || v != "" {
					t.Errorf("Get('1') after delete = %q, %v; want '', false", v, ok)
				}
				if l := om.Len(); l != 1 {
					t.Errorf("Len() = %d; want 1", l)
				}
				want := []string{"2"}
				if got := om.Keys(); !reflect.DeepEqual(got, want) {
					t.Errorf("Keys() = %v; want %v", got, want)
				}
			},
		},
		{
			name: "Delete non-existent key",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Delete("nonexistent")
			},
			verify: func(t *testing.T, om *OrderMap[string, string]) {
				if l := om.Len(); l != 1 {
					t.Errorf("Len() = %d; want 1", l)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			om := NewOrderMap[string, string]()
			tt.setup(om)
			tt.verify(t, om)
		})
	}
}

func TestOrderMapIterateFunc(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*OrderMap[string, int])
		wantErr bool
	}{
		{
			name: "Iterate all without error",
			setup: func(om *OrderMap[string, int]) {
				om.Set("a", 1)
				om.Set("b", 2)
				om.Set("c", 3)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			om := NewOrderMap[string, int]()
			tt.setup(om)

			sum := 0
			err := om.IterateFunc(func(k string, v int) error {
				sum += v
				return nil
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("IterateFunc() error = %v, wantErr %v", err, tt.wantErr)
			}
			if sum != 6 {
				t.Errorf("IterateFunc sum = %d; want 6", sum)
			}
		})
	}
}

func TestOrderMapJSON(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*OrderMap[string, string])
		want    string
		wantErr bool
	}{
		{
			name: "MarshalJSON order preserved",
			setup: func(om *OrderMap[string, string]) {
				om.Set("0", "a")
				om.Set("1", "b")
				om.Set("2", "c")
			},
			want:    `{"0":"a","1":"b","2":"c"}`,
			wantErr: false,
		},
		{
			name:    "MarshalJSON empty map",
			setup:   func(om *OrderMap[string, string]) {},
			want:    `{}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			om := NewOrderMap[string, string]()
			tt.setup(om)

			got, err := json.Marshal(om)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.want {
				t.Errorf("MarshalJSON() = %q; want %q", string(got), tt.want)
			}
		})
	}
}

func TestOrderMapUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantKeys   []string
		wantValues []string
		wantErr    bool
	}{
		{
			name:       "Unmarshal JSON order preserved",
			input:      `{"0":"a","1":"b","2":"c"}`,
			wantKeys:   []string{"0", "1", "2"},
			wantValues: []string{"a", "b", "c"},
			wantErr:    false,
		},
		{
			name:       "Unmarshal empty object",
			input:      `{}`,
			wantKeys:   []string{},
			wantValues: []string{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			om := NewOrderMap[string, string]()
			err := json.Unmarshal([]byte(tt.input), &om)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(om.Keys(), tt.wantKeys) {
				t.Errorf("Keys() = %v; want %v", om.Keys(), tt.wantKeys)
			}
			if !reflect.DeepEqual(om.Values(), tt.wantValues) {
				t.Errorf("Values() = %v; want %v", om.Values(), tt.wantValues)
			}
		})
	}
}

func TestOrderMapRoundTripJSON(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*OrderMap[string, string])
	}{
		{
			name: "Marshal and Unmarshal round trip",
			setup: func(om *OrderMap[string, string]) {
				om.Set("1", "a")
				om.Set("2", "b")
				om.Set("3", "c")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := NewOrderMap[string, string]()
			tt.setup(original)

			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var restored *OrderMap[string, string]
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if !reflect.DeepEqual(original.Keys(), restored.Keys()) {
				t.Errorf("Keys mismatch: original %v, restored %v", original.Keys(), restored.Keys())
			}
			if !reflect.DeepEqual(original.Values(), restored.Values()) {
				t.Errorf("Values mismatch: original %v, restored %v", original.Values(), restored.Values())
			}
		})
	}
}

func TestOrderMapConcurrentSameKey(t *testing.T) {
	om := NewOrderMap[string, int]()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			om.Set("same", v)
			om.Get("same")
		}(i)
	}
	wg.Wait()

	if l := om.Len(); l != 1 {
		t.Fatalf("Len() = %d; want 1", l)
	}
	if keys := om.Keys(); !reflect.DeepEqual(keys, []string{"same"}) {
		t.Fatalf("Keys() = %v; want [same]", keys)
	}
	if _, ok := om.Get("same"); !ok {
		t.Fatalf("Get('same') missing after concurrent writes")
	}
}

func TestOrderMapConcurrentDifferentKeys(t *testing.T) {
	om := NewOrderMap[int, int]()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			om.Set(v, v)
			om.Get(v)
		}(i)
	}
	wg.Wait()

	if l := om.Len(); l != 100 {
		t.Fatalf("Len() = %d; want 100", l)
	}
	for i := 0; i < 100; i++ {
		if v, ok := om.Get(i); !ok || v != i {
			t.Fatalf("Get(%d) = %d, %v; want %d, true", i, v, ok, i)
		}
	}
}

func TestOrderMapKeysReturnsCopy(t *testing.T) {
	om := NewOrderMap[string, string]()
	om.Set("1", "a")
	om.Set("2", "b")

	keys := om.Keys()
	keys[0] = "changed"

	if got := om.Keys(); !reflect.DeepEqual(got, []string{"1", "2"}) {
		t.Fatalf("Keys() after external mutation = %v; want [1 2]", got)
	}
}

func TestOrderMapDifferentTypes(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "Int key with string value",
			setup: func(t *testing.T) {
				om := NewOrderMap[int, string]()
				om.Set(1, "one")
				om.Set(2, "two")
				if v, ok := om.Get(1); !ok || v != "one" {
					t.Errorf("Get(1) = %q, %v; want 'one', true", v, ok)
				}
			},
		},
		{
			name: "Struct value",
			setup: func(t *testing.T) {
				type Person struct {
					Name string
					Age  int
				}
				om := NewOrderMap[string, Person]()
				om.Set("alice", Person{Name: "Alice", Age: 30})
				v, ok := om.Get("alice")
				if !ok || v.Name != "Alice" || v.Age != 30 {
					t.Errorf("Get('alice') = %v, %v", v, ok)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.setup)
	}
}
