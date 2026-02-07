package hashedset

import (
	"testing"
)

func TestHashedSet_SetAndGet(t *testing.T) {
	hs := New()

	data := map[string]string{"key": "value"}
	err := hs.SetHash("test", data)
	if err != nil {
		t.Fatalf("SetHash failed: %v", err)
	}

	hash, exists := hs.GetHash("test")
	if !exists {
		t.Error("Expected hash to exist")
	}
	if hash == "" {
		t.Error("Expected non-empty hash")
	}
}

func TestHashedSet_HasChanged(t *testing.T) {
	hs := New()

	data1 := map[string]string{"key": "value1"}
	data2 := map[string]string{"key": "value2"}

	// Initially should report as changed (key doesn't exist)
	changed, err := hs.HasChanged("test", data1)
	if err != nil {
		t.Fatalf("HasChanged failed: %v", err)
	}
	if !changed {
		t.Error("Expected changed=true for non-existent key")
	}

	// Set the hash
	err = hs.SetHash("test", data1)
	if err != nil {
		t.Fatalf("SetHash failed: %v", err)
	}

	// Same data should not be changed
	changed, err = hs.HasChanged("test", data1)
	if err != nil {
		t.Fatalf("HasChanged failed: %v", err)
	}
	if changed {
		t.Error("Expected changed=false for same data")
	}

	// Different data should be changed
	changed, err = hs.HasChanged("test", data2)
	if err != nil {
		t.Fatalf("HasChanged failed: %v", err)
	}
	if !changed {
		t.Error("Expected changed=true for different data")
	}
}

func TestHashedSet_UpdateIfChanged(t *testing.T) {
	hs := New()

	data1 := map[string]string{"key": "value1"}
	data2 := map[string]string{"key": "value2"}

	// First update should return true (new key)
	updated, err := hs.UpdateIfChanged("test", data1)
	if err != nil {
		t.Fatalf("UpdateIfChanged failed: %v", err)
	}
	if !updated {
		t.Error("Expected updated=true for new key")
	}

	// Same data should not update
	updated, err = hs.UpdateIfChanged("test", data1)
	if err != nil {
		t.Fatalf("UpdateIfChanged failed: %v", err)
	}
	if updated {
		t.Error("Expected updated=false for same data")
	}

	// Different data should update
	updated, err = hs.UpdateIfChanged("test", data2)
	if err != nil {
		t.Fatalf("UpdateIfChanged failed: %v", err)
	}
	if !updated {
		t.Error("Expected updated=true for different data")
	}
}

func TestHashedSet_Delete(t *testing.T) {
	hs := New()

	data := map[string]string{"key": "value"}
	hs.SetHash("test", data)

	hs.Delete("test")

	_, exists := hs.GetHash("test")
	if exists {
		t.Error("Expected hash to be deleted")
	}
}

func TestHashedSet_Clear(t *testing.T) {
	hs := New()

	hs.SetHash("test1", "data1")
	hs.SetHash("test2", "data2")

	if hs.Size() != 2 {
		t.Errorf("Expected size 2, got %d", hs.Size())
	}

	hs.Clear()

	if hs.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", hs.Size())
	}
}

func TestHashedSet_Keys(t *testing.T) {
	hs := New()

	hs.SetHash("key1", "data1")
	hs.SetHash("key2", "data2")
	hs.SetHash("key3", "data3")

	keys := hs.Keys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	// Check all keys exist
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	for _, expected := range []string{"key1", "key2", "key3"} {
		if !keyMap[expected] {
			t.Errorf("Expected key %s to exist", expected)
		}
	}
}

func TestComputeHashString(t *testing.T) {
	hash1 := ComputeHashString("hello")
	hash2 := ComputeHashString("hello")
	hash3 := ComputeHashString("world")

	if hash1 != hash2 {
		t.Error("Same input should produce same hash")
	}

	if hash1 == hash3 {
		t.Error("Different input should produce different hash")
	}

	if len(hash1) != 64 {
		t.Errorf("Expected SHA256 hash length 64, got %d", len(hash1))
	}
}
