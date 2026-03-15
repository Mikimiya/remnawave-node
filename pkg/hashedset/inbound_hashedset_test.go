package hashedset

import (
	"testing"
)

// TestDjb2Dual verifies the Go djb2Dual function matches TypeScript behavior.
// The TypeScript version uses:
//
//	high = ((high << 5) + high + char) | 0;   → signed 32-bit truncation
//	low  = ((low << 6) + low + char * 37) | 0; → signed 32-bit truncation
//
// then: high >>> 0, low >>> 0  → unsigned interpretation
// In Go, uint32 naturally gives unsigned 32-bit truncation, which is equivalent.
func TestDjb2Dual(t *testing.T) {
	// Test with a simple UUID-like string
	h, l := djb2Dual("test")
	if h == 0 && l == 0 {
		t.Error("Expected non-zero hash for 'test'")
	}

	// Verify determinism
	h2, l2 := djb2Dual("test")
	if h != h2 || l != l2 {
		t.Error("djb2Dual is not deterministic")
	}

	// Different strings should produce different hashes (with high probability)
	h3, l3 := djb2Dual("test2")
	if h == h3 && l == l3 {
		t.Error("Expected different hashes for different strings")
	}
}

// TestInboundHashedSetAddDelete tests basic add/delete with XOR property.
// XOR is self-inverse: adding then deleting the same element should restore the original hash.
func TestInboundHashedSetAddDelete(t *testing.T) {
	s := NewInboundHashedSet()

	// Empty set should have zero hash
	if s.Hash64String() != "0000000000000000" {
		t.Errorf("Empty set hash should be all zeros, got %s", s.Hash64String())
	}

	// Add an element
	s.Add("user1")
	hash1 := s.Hash64String()
	if hash1 == "0000000000000000" {
		t.Error("Hash should change after adding an element")
	}

	// Add same element again (no-op)
	s.Add("user1")
	if s.Hash64String() != hash1 {
		t.Error("Adding duplicate should not change hash")
	}
	if s.Size() != 1 {
		t.Errorf("Size should be 1 after duplicate add, got %d", s.Size())
	}

	// Delete the element (should restore zero hash)
	s.Delete("user1")
	if s.Hash64String() != "0000000000000000" {
		t.Errorf("Hash should be zero after removing the only element, got %s", s.Hash64String())
	}
	if s.Size() != 0 {
		t.Errorf("Size should be 0 after delete, got %d", s.Size())
	}
}

// TestInboundHashedSetXORCommutative tests that hash is order-independent.
// {a, b} should produce the same hash regardless of insertion order.
func TestInboundHashedSetXORCommutative(t *testing.T) {
	s1 := NewInboundHashedSet()
	s1.Add("alpha")
	s1.Add("beta")

	s2 := NewInboundHashedSet()
	s2.Add("beta")
	s2.Add("alpha")

	if s1.Hash64String() != s2.Hash64String() {
		t.Errorf("Hash should be order-independent: %s != %s", s1.Hash64String(), s2.Hash64String())
	}
}

// TestInboundHashedSetMultipleUsers tests adding multiple users and comparing hashes.
func TestInboundHashedSetMultipleUsers(t *testing.T) {
	s := NewInboundHashedSet()

	uuids := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"f47ac10b-58cc-4372-a567-0e02b2c3d479",
	}

	for _, uuid := range uuids {
		s.Add(uuid)
	}

	if s.Size() != 3 {
		t.Errorf("Expected size 3, got %d", s.Size())
	}

	hashBefore := s.Hash64String()

	// Remove middle element and re-add it
	s.Delete("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	s.Add("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	if s.Hash64String() != hashBefore {
		t.Errorf("Hash should be restored after remove+re-add: %s != %s",
			hashBefore, s.Hash64String())
	}
}

// TestInboundHashedSetFrom tests creating from initial members.
func TestInboundHashedSetFrom(t *testing.T) {
	items := []string{"a", "b", "c"}

	s1 := NewInboundHashedSetFrom(items)
	s2 := NewInboundHashedSet()
	for _, item := range items {
		s2.Add(item)
	}

	if s1.Hash64String() != s2.Hash64String() {
		t.Errorf("NewInboundHashedSetFrom should produce same hash as manual adds: %s != %s",
			s1.Hash64String(), s2.Hash64String())
	}
}

// TestInboundHashedSetClear tests clearing the set.
func TestInboundHashedSetClear(t *testing.T) {
	s := NewInboundHashedSet()
	s.Add("user1")
	s.Add("user2")
	s.Clear()

	if s.Hash64String() != "0000000000000000" {
		t.Errorf("Hash should be zero after clear, got %s", s.Hash64String())
	}
	if s.Size() != 0 {
		t.Errorf("Size should be 0 after clear, got %d", s.Size())
	}
}

// TestInboundHashedSetHas tests membership check.
func TestInboundHashedSetHas(t *testing.T) {
	s := NewInboundHashedSet()
	s.Add("exists")

	if !s.Has("exists") {
		t.Error("Has should return true for existing element")
	}
	if s.Has("not-exists") {
		t.Error("Has should return false for non-existing element")
	}
}

// TestInboundHashedSetDeleteNonExistent tests deleting a non-existent element.
func TestInboundHashedSetDeleteNonExistent(t *testing.T) {
	s := NewInboundHashedSet()
	s.Add("user1")
	hashBefore := s.Hash64String()

	deleted := s.Delete("nonexistent")
	if deleted {
		t.Error("Delete should return false for non-existent element")
	}
	if s.Hash64String() != hashBefore {
		t.Error("Hash should not change when deleting non-existent element")
	}
}

// TestDjb2DualKnownValues tests specific known values to verify algorithm correctness.
// These values can be verified by running the TypeScript implementation.
func TestDjb2DualKnownValues(t *testing.T) {
	// Manually verify: djb2 for empty string
	h, l := djb2Dual("")
	if h != 5381 || l != 5387 {
		t.Errorf("djb2Dual('') should be (5381, 5387), got (%d, %d)", h, l)
	}

	// For "a" (char code 97):
	// high = ((5381 << 5) + 5381 + 97) = (172192 + 5381 + 97) = 177670
	// low  = ((5387 << 6) + 5387 + 97*37) = (344768 + 5387 + 3589) = 353744
	h, l = djb2Dual("a")
	expectedH := uint32(177670)
	expectedL := uint32(353744)
	if h != expectedH || l != expectedL {
		t.Errorf("djb2Dual('a') should be (%d, %d), got (%d, %d)",
			expectedH, expectedL, h, l)
	}
}

// TestCrossLanguageHashCompatibility verifies that Go's djb2Dual produces identical
// results to the TypeScript @remnawave/hashed-set implementation.
// These reference values were generated by running the exact TypeScript algorithm.
// If these tests pass, the Go node will compute the same hashes as the panel.
func TestCrossLanguageHashCompatibility(t *testing.T) {
	// Reference values from TypeScript @remnawave/hashed-set djb2Dual
	djb2Tests := []struct {
		input      string
		expectHigh uint32
		expectLow  uint32
	}{
		{"", 5381, 5387},
		{"a", 177670, 353744},
		{"abc", 193485963, 1494807753},
		{"hello", 261238937, 2321330031},
		{"550e8400-e29b-41d4-a716-446655440000", 2510727352, 1920271882},
		{"6ba7b810-9dad-11d1-80b4-00c04fd430c8", 3867652190, 3888860392},
		{"f47ac10b-58cc-4372-a567-0e02b2c3d479", 694905728, 1795696914},
	}

	for _, tc := range djb2Tests {
		h, l := djb2Dual(tc.input)
		if h != tc.expectHigh || l != tc.expectLow {
			t.Errorf("djb2Dual(%q): Go=(%d,%d) TypeScript=(%d,%d)",
				tc.input, h, l, tc.expectHigh, tc.expectLow)
		}
	}
}

// TestCrossLanguageHash64String verifies that Go's Hash64String output matches
// the TypeScript HashedSet.hash64String for the same set of UUIDs.
// This is the critical test: if these pass, IsNeedRestartCore will correctly
// compare hashes with the panel and avoid unnecessary Xray restarts.
func TestCrossLanguageHash64String(t *testing.T) {
	uuid1 := "550e8400-e29b-41d4-a716-446655440000"
	uuid2 := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	uuid3 := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	tests := []struct {
		name   string
		items  []string
		expect string
	}{
		{
			name:   "single UUID",
			items:  []string{uuid1},
			expect: "95a6a8b87275060a",
		},
		{
			name:   "two UUIDs",
			items:  []string{uuid1, uuid2},
			expect: "732118e695be4ae2",
		},
		{
			name:   "three UUIDs",
			items:  []string{uuid1, uuid2, uuid3},
			expect: "5a4a7366feb663f0",
		},
		{
			name:   "two UUIDs reversed order (commutative)",
			items:  []string{uuid2, uuid1},
			expect: "732118e695be4ae2",
		},
		{
			name:   "three UUIDs shuffled order (commutative)",
			items:  []string{uuid3, uuid1, uuid2},
			expect: "5a4a7366feb663f0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewInboundHashedSet()
			for _, item := range tc.items {
				s.Add(item)
			}
			got := s.Hash64String()
			if got != tc.expect {
				t.Errorf("Hash64String = %s, want %s (TypeScript reference)", got, tc.expect)
			}
		})
	}
}
