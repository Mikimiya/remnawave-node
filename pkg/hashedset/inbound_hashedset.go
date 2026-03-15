// Package hashedset provides hash-based set implementations for change detection.
//
// InboundHashedSet replicates the @remnawave/hashed-set algorithm (DJB2 dual hash with XOR),
// used by the panel and Node.js node to track per-inbound user sets.
// The hash is order-independent due to XOR and supports efficient add/delete.
package hashedset

import (
	"fmt"
	"sync"
)

// InboundHashedSet is a set of strings that maintains a rolling DJB2 dual hash.
// This matches the @remnawave/hashed-set TypeScript implementation:
//   - Uses djb2Dual(str) → {high, low} with two variants (seed 5381 and 5387)
//   - add(str): XOR the element's hash into the running hash
//   - delete(str): XOR the element's hash out (XOR is self-inverse)
//   - hash64String: 16-char hex = highHex(8) + lowHex(8)
type InboundHashedSet struct {
	mu       sync.RWMutex
	members  map[string]struct{}
	hashHigh uint32
	hashLow  uint32
}

// NewInboundHashedSet creates a new empty InboundHashedSet
func NewInboundHashedSet() *InboundHashedSet {
	return &InboundHashedSet{
		members: make(map[string]struct{}),
	}
}

// NewInboundHashedSetFrom creates a new InboundHashedSet from initial members
func NewInboundHashedSetFrom(items []string) *InboundHashedSet {
	s := NewInboundHashedSet()
	for _, item := range items {
		s.Add(item)
	}
	return s
}

// djb2Dual computes the dual DJB2 hash of a string.
// Matches TypeScript:
//
//	let high = 5381, low = 5387;
//	for (let i = 0; i < str.length; i++) {
//	    const char = str.charCodeAt(i);
//	    high = ((high << 5) + high + char) | 0;
//	    low  = ((low << 6) + low + char * 37) | 0;
//	}
//	return { high: high >>> 0, low: low >>> 0 };
func djb2Dual(str string) (high, low uint32) {
	high = 5381
	low = 5387

	for i := 0; i < len(str); i++ {
		ch := uint32(str[i])
		// high = ((high << 5) + high + char) | 0  →  truncated to 32-bit signed then unsigned
		high = (high << 5) + high + ch
		// low = ((low << 6) + low + char * 37) | 0
		low = (low << 6) + low + ch*37
	}

	return high, low
}

// Add adds a string to the set and updates the hash.
// If the string is already present, this is a no-op.
func (s *InboundHashedSet) Add(str string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.members[str]; exists {
		return
	}

	s.members[str] = struct{}{}
	h, l := djb2Dual(str)
	s.hashHigh ^= h
	s.hashLow ^= l
}

// Delete removes a string from the set and updates the hash.
// Returns true if the element was present and removed.
func (s *InboundHashedSet) Delete(str string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.members[str]; !exists {
		return false
	}

	delete(s.members, str)
	h, l := djb2Dual(str)
	s.hashHigh ^= h
	s.hashLow ^= l
	return true
}

// Clear removes all members and resets the hash to zero.
func (s *InboundHashedSet) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.members = make(map[string]struct{})
	s.hashHigh = 0
	s.hashLow = 0
}

// Hash64String returns the 16-character hex hash string.
// Format: high (8 hex chars, zero-padded) + low (8 hex chars, zero-padded)
// Matches TypeScript: this.hashHigh.toString(16).padStart(8, '0') + this.hashLow.toString(16).padStart(8, '0')
func (s *InboundHashedSet) Hash64String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return fmt.Sprintf("%08x%08x", s.hashHigh, s.hashLow)
}

// Size returns the number of members in the set.
func (s *InboundHashedSet) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.members)
}

// Has checks if a string is in the set.
func (s *InboundHashedSet) Has(str string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.members[str]
	return exists
}
