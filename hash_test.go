package goby

import (
	"testing"
)

func TestNewHash(t *testing.T) {
	h := NewHash()
	if h == nil {
		t.Error("NewHash returned nil")
	}
	if h.Size() != 0 {
		t.Errorf("Expected empty hash, got size %d", h.Size())
	}
}

func TestHashSetAndGet(t *testing.T) {
	h := NewHash()
	key := "test"
	value := "value"

	h.Set(key, value)
	if val, ok := h.Get(key); !ok {
		t.Error("Key not found")
	} else if val != value {
		t.Errorf("Expected %v, got %v", value, val)
	}
}

func TestHashDelete(t *testing.T) {
	h := NewHash()
	key := "test"
	value := "value"

	h.Set(key, value)
	deleted := h.Delete(key)
	if deleted != value {
		t.Errorf("Expected deleted value %v, got %v", value, deleted)
	}
	if _, ok := h.Get(key); ok {
		t.Error("Key still exists after deletion")
	}
}

func TestHashSize(t *testing.T) {
	h := NewHash()
	if h.Size() != 0 {
		t.Errorf("Expected size 0, got %d", h.Size())
	}

	h.Set("key1", "value1")
	h.Set("key2", "value2")
	if h.Size() != 2 {
		t.Errorf("Expected size 2, got %d", h.Size())
	}
}

func TestHashKeys(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	keys := h.Keys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}

	keyMap := make(map[interface{}]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	if !keyMap["key1"] || !keyMap["key2"] {
		t.Error("Expected keys not found")
	}
}

func TestHashValues(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	values := h.Values()
	if len(values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(values))
	}

	valueMap := make(map[interface{}]bool)
	for _, v := range values {
		valueMap[v] = true
	}

	if !valueMap["value1"] || !valueMap["value2"] {
		t.Error("Expected values not found")
	}
}

func TestHashClear(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	h.Clear()
	if h.Size() != 0 {
		t.Errorf("Expected empty hash after clear, got size %d", h.Size())
	}
}

func TestHashHasKey(t *testing.T) {
	h := NewHash()
	key := "test"
	value := "value"

	h.Set(key, value)
	if !h.HasKey(key) {
		t.Error("HasKey returned false for existing key")
	}
	if h.HasKey("nonexistent") {
		t.Error("HasKey returned true for nonexistent key")
	}
}

func TestHashHasValue(t *testing.T) {
	h := NewHash()
	key := "test"
	value := "value"

	h.Set(key, value)
	if !h.HasValue(value) {
		t.Error("HasValue returned false for existing value")
	}
	if h.HasValue("nonexistent") {
		t.Error("HasValue returned true for nonexistent value")
	}
}

func TestHashMerge(t *testing.T) {
	h1 := NewHash()
	h1.Set("key1", "value1")

	h2 := NewHash()
	h2.Set("key2", "value2")

	merged := h1.Merge(h2)
	if merged.Size() != 2 {
		t.Errorf("Expected merged size 2, got %d", merged.Size())
	}

	if val, _ := merged.Get("key1"); val != "value1" {
		t.Error("First hash values not preserved in merge")
	}
	if val, _ := merged.Get("key2"); val != "value2" {
		t.Error("Second hash values not included in merge")
	}
}

func TestHashMergeBang(t *testing.T) {
	h1 := NewHash()
	h1.Set("key1", "value1")

	h2 := NewHash()
	h2.Set("key2", "value2")

	h1.MergeBang(h2)
	if h1.Size() != 2 {
		t.Errorf("Expected size 2 after merge, got %d", h1.Size())
	}

	if val, _ := h1.Get("key1"); val != "value1" {
		t.Error("Original values not preserved in merge")
	}
	if val, _ := h1.Get("key2"); val != "value2" {
		t.Error("Second hash values not included in merge")
	}
}

func TestHashToString(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	str := h.ToString()
	if str == "" {
		t.Error("ToString returned empty string")
	}
}

func TestHashInspect(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	str := h.Inspect()
	if str == "" {
		t.Error("Inspect returned empty string")
	}
}

func TestHashEach(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	count := 0
	h.Each(func(key, value interface{}) {
		count++
	})

	if count != 2 {
		t.Errorf("Expected 2 iterations, got %d", count)
	}
}

func TestHashSelect(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")
	h.Set("key3", "value3")

	selected := h.Select(func(key, value interface{}) bool {
		return key.(string) == "key1" || key.(string) == "key2"
	})

	if selected.Size() != 2 {
		t.Errorf("Expected 2 selected items, got %d", selected.Size())
	}
}

func TestHashReject(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")
	h.Set("key3", "value3")

	rejected := h.Reject(func(key, value interface{}) bool {
		return key.(string) == "key1"
	})

	if rejected.Size() != 2 {
		t.Errorf("Expected 2 rejected items, got %d", rejected.Size())
	}
}

func TestHashTransformKeys(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	transformed := h.TransformKeys(func(key interface{}) interface{} {
		return key.(string) + "_transformed"
	})

	if transformed.Size() != 2 {
		t.Errorf("Expected 2 transformed items, got %d", transformed.Size())
	}

	if val, _ := transformed.Get("key1_transformed"); val != "value1" {
		t.Error("Values not preserved in key transformation")
	}
}

func TestHashTransformValues(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	transformed := h.TransformValues(func(value interface{}) interface{} {
		return value.(string) + "_transformed"
	})

	if transformed.Size() != 2 {
		t.Errorf("Expected 2 transformed items, got %d", transformed.Size())
	}

	if val, _ := transformed.Get("key1"); val != "value1_transformed" {
		t.Error("Values not properly transformed")
	}
}

func TestHashFetch(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")

	// Test existing key
	if val, err := h.Fetch("key1"); err != nil {
		t.Error("Fetch returned error for existing key")
	} else if val != "value1" {
		t.Errorf("Expected value1, got %v", val)
	}

	// Test nonexistent key with default
	if val, err := h.Fetch("nonexistent", "default"); err != nil {
		t.Error("Fetch returned error with default value")
	} else if val != "default" {
		t.Errorf("Expected default, got %v", val)
	}

	// Test nonexistent key without default
	if _, err := h.Fetch("nonexistent"); err == nil {
		t.Error("Fetch did not return error for nonexistent key")
	}
}

func TestHashToA(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	arr := h.ToA()
	if arr.Length() != 2 {
		t.Errorf("Expected array length 2, got %d", arr.Length())
	}
}

func TestHashToH(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	newHash := h.ToH()
	if newHash.Size() != 2 {
		t.Errorf("Expected hash size 2, got %d", newHash.Size())
	}

	if val, _ := newHash.Get("key1"); val != "value1" {
		t.Error("Values not preserved in hash conversion")
	}
}

func TestHashToS(t *testing.T) {
	h := NewHash()
	h.Set("key1", "value1")
	h.Set("key2", "value2")

	str := h.ToS()
	if str.ToString() == "" {
		t.Error("ToS returned empty string")
	}
}

func TestHashEqual(t *testing.T) {
	h1 := NewHash()
	h1.Set("key1", "value1")
	h1.Set("key2", "value2")

	h2 := NewHash()
	h2.Set("key1", "value1")
	h2.Set("key2", "value2")

	if !h1.Equal(h2) {
		t.Error("Equal hashes not considered equal")
	}

	h2.Set("key3", "value3")
	if h1.Equal(h2) {
		t.Error("Different hashes considered equal")
	}
}

func TestRHash_ToJSON(t *testing.T) {
	hash := NewHash()
	hash.Set("name", "John")
	hash.Set("age", 30)

	expected := `{"age":30,"name":"John"}`
	result := hash.ToJSON().ToString()
	if result != expected {
		t.Errorf("ToJSON() = %v, want %v", result, expected)
	}
}

func TestRHash_ToYAML(t *testing.T) {
	hash := NewHash()
	hash.Set("name", "John")
	hash.Set("age", 30)

	expected := "age: 30\nname: John\n"
	result := hash.ToYAML().ToString()
	if result != expected {
		t.Errorf("ToYAML() = %v, want %v", result, expected)
	}
}

func TestRHash_ToXML(t *testing.T) {
	hash := NewHash()
	hash.Set("name", "John")
	hash.Set("age", 30)

	expected := `<hash>
  <entry>
    <key>age</key>
    <value>30</value>
  </entry>
  <entry>
    <key>name</key>
    <value>John</value>
  </entry>
</hash>`
	result := hash.ToXML().ToString()
	if result != expected {
		t.Errorf("ToXML() = %v, want %v", result, expected)
	}
}

func TestRHash_ToHTML(t *testing.T) {
	hash := NewHash()
	hash.Set("name", "John")
	hash.Set("age", 30)

	expected := `<div class="hash">
  <div class="entry">
    <span class="key">age</span>
    <span class="value">30</span>
  </div>
  <div class="entry">
    <span class="key">name</span>
    <span class="value">John</span>
  </div>
</div>`
	result := hash.ToHTML().ToString()
	if result != expected {
		t.Errorf("ToHTML() = %v, want %v", result, expected)
	}
}

func TestRHash_ToCSV(t *testing.T) {
	hash := NewHash()
	hash.Set("name", "John")
	hash.Set("age", 30)

	expected := "key,value\nage,30\nname,John"
	result := hash.ToCSV().ToString()
	if result != expected {
		t.Errorf("ToCSV() = %v, want %v", result, expected)
	}
}

func TestRHash_ToTSV(t *testing.T) {
	hash := NewHash()
	hash.Set("name", "John")
	hash.Set("age", 30)

	expected := "key\tvalue\nage\t30\nname\tJohn"
	result := hash.ToTSV().ToString()
	if result != expected {
		t.Errorf("ToTSV() = %v, want %v", result, expected)
	}
}
