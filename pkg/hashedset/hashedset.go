// Package hashedset provides a concurrent-safe hash set for tracking configuration changes
package hashedset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// HashedSet stores hashes of configuration objects for change detection
type HashedSet struct {
	mu     sync.RWMutex
	hashes map[string]string // key -> hash
}

// New creates a new HashedSet
func New() *HashedSet {
	return &HashedSet{
		hashes: make(map[string]string),
	}
}

// SetHash sets the hash for a key
func (s *HashedSet) SetHash(key string, data any) error {
	hash, err := computeHash(data)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes[key] = hash
	return nil
}

// HasChanged checks if the data has changed from the stored hash
// Returns true if changed or if key doesn't exist
func (s *HashedSet) HasChanged(key string, data any) (bool, error) {
	hash, err := computeHash(data)
	if err != nil {
		return false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	storedHash, exists := s.hashes[key]
	if !exists {
		return true, nil
	}

	return storedHash != hash, nil
}

// UpdateIfChanged updates the hash if the data has changed
// Returns true if the hash was updated (data changed)
func (s *HashedSet) UpdateIfChanged(key string, data any) (bool, error) {
	hash, err := computeHash(data)
	if err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	storedHash, exists := s.hashes[key]
	if !exists || storedHash != hash {
		s.hashes[key] = hash
		return true, nil
	}

	return false, nil
}

// Delete removes a key from the set
func (s *HashedSet) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.hashes, key)
}

// Clear removes all entries
func (s *HashedSet) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes = make(map[string]string)
}

// GetHash returns the stored hash for a key
func (s *HashedSet) GetHash(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hash, exists := s.hashes[key]
	return hash, exists
}

// SetHashValue directly sets a hash value for a key (without computing)
func (s *HashedSet) SetHashValue(key, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes[key] = hash
}

// Keys returns all keys in the set
func (s *HashedSet) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.hashes))
	for k := range s.hashes {
		keys = append(keys, k)
	}
	return keys
}

// Size returns the number of entries
func (s *HashedSet) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.hashes)
}

// computeHash computes SHA256 hash of JSON-serialized data
func computeHash(data any) (string, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(hash[:]), nil
}

// ComputeHashString computes hash of a raw string
func ComputeHashString(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}

// ComputeHashBytes computes hash of raw bytes
func ComputeHashBytes(b []byte) string {
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}
