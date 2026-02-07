// Package services provides business logic for internal operations
package services

import (
	"encoding/json"
	"sync"

	"go.uber.org/zap"

	"github.com/Mikimiya/remnawave-node/pkg/hashedset"
)

// InternalService manages internal node state
type InternalService struct {
	mu               sync.RWMutex
	logger           *zap.Logger
	hashedSet        *hashedset.HashedSet
	config           json.RawMessage
	disableHashCheck bool

	// User-Inbound tracking: email -> set of inbound tags
	userInboundMap map[string]map[string]struct{}
	// Per-inbound hash sets for fine-grained change detection
	inboundHashSets map[string]*hashedset.HashedSet
	// Empty config hash (config without users)
	emptyConfigHash string
	// All known inbound tags (used for removing users from all inbounds)
	xtlsConfigInbounds map[string]struct{}
}

// InternalConfig holds Internal service configuration
type InternalConfig struct {
	DisableHashCheck bool
}

// NewInternalService creates a new InternalService
func NewInternalService(cfg *InternalConfig, logger *zap.Logger) *InternalService {
	return &InternalService{
		logger:             logger,
		hashedSet:          hashedset.New(),
		disableHashCheck:   cfg.DisableHashCheck,
		userInboundMap:     make(map[string]map[string]struct{}),
		inboundHashSets:    make(map[string]*hashedset.HashedSet),
		xtlsConfigInbounds: make(map[string]struct{}),
	}
}

// GetXtlsConfigInbounds returns all known inbound tags
func (s *InternalService) GetXtlsConfigInbounds() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.xtlsConfigInbounds))
	for tag := range s.xtlsConfigInbounds {
		result = append(result, tag)
	}
	return result
}

// AddXtlsConfigInbound adds an inbound tag to the known set
func (s *InternalService) AddXtlsConfigInbound(tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.xtlsConfigInbounds[tag] = struct{}{}
}

// Cleanup clears all internal state (called when Xray stops)
func (s *InternalService) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Cleaning up internal service state")

	s.userInboundMap = make(map[string]map[string]struct{})
	s.inboundHashSets = make(map[string]*hashedset.HashedSet)
	s.xtlsConfigInbounds = make(map[string]struct{})
	s.config = nil
	s.emptyConfigHash = ""
}

// GetUserInbounds returns all inbound tags that a user belongs to
func (s *InternalService) GetUserInbounds(email string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tags, exists := s.userInboundMap[email]
	if !exists {
		return nil
	}

	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	return result
}

// AddUserToInbound records that a user belongs to an inbound
func (s *InternalService) AddUserToInbound(email, tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.userInboundMap[email] == nil {
		s.userInboundMap[email] = make(map[string]struct{})
	}
	s.userInboundMap[email][tag] = struct{}{}
}

// RemoveUserFromInbound removes a user from an inbound tracking
func (s *InternalService) RemoveUserFromInbound(email, tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tags, exists := s.userInboundMap[email]; exists {
		delete(tags, tag)
		// Clean up if no more inbounds
		if len(tags) == 0 {
			delete(s.userInboundMap, email)
		}
	}
}

// RemoveUserFromAllInbounds removes a user from all inbound tracking
func (s *InternalService) RemoveUserFromAllInbounds(email string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	tags, exists := s.userInboundMap[email]
	if !exists {
		return nil
	}

	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	delete(s.userInboundMap, email)
	return result
}

// GetUsersInInbound returns all user emails in a specific inbound
func (s *InternalService) GetUsersInInbound(tag string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var users []string
	for email, tags := range s.userInboundMap {
		if _, exists := tags[tag]; exists {
			users = append(users, email)
		}
	}
	return users
}

// GetUsersCountInInbound returns the count of users in a specific inbound
func (s *InternalService) GetUsersCountInInbound(tag string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, tags := range s.userInboundMap {
		if _, exists := tags[tag]; exists {
			count++
		}
	}
	return count
}

// XrayInbound represents an inbound configuration
type XrayInbound struct {
	Tag      string `json:"tag"`
	Settings struct {
		Clients []struct {
			Email string `json:"email"`
		} `json:"clients"`
	} `json:"settings"`
}

// XrayConfigParsed represents parsed Xray config for user extraction
type XrayConfigParsed struct {
	Inbounds []XrayInbound `json:"inbounds"`
}

// ExtractUsersFromConfig parses config and builds user-inbound mapping
// Also stores the incoming hashes for later comparison
func (s *InternalService) ExtractUsersFromConfig(config json.RawMessage, hashes *InboundHashes) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var parsed XrayConfigParsed
	if err := json.Unmarshal(config, &parsed); err != nil {
		return err
	}

	// Clear existing mappings
	s.userInboundMap = make(map[string]map[string]struct{})
	s.inboundHashSets = make(map[string]*hashedset.HashedSet)
	s.xtlsConfigInbounds = make(map[string]struct{})

	// Build valid tags set from incoming hashes
	validTags := make(map[string]string) // tag -> hash
	if hashes != nil {
		s.emptyConfigHash = hashes.EmptyConfig
		for _, item := range hashes.Inbounds {
			validTags[item.Tag] = item.Hash
		}
	}

	for _, inbound := range parsed.Inbounds {
		if inbound.Tag == "" {
			continue
		}

		// Only process inbounds that are in the valid tags (from hashes)
		incomingHash, isValid := validTags[inbound.Tag]
		if hashes != nil && !isValid {
			continue
		}

		// Add to known inbounds set
		s.xtlsConfigInbounds[inbound.Tag] = struct{}{}

		// Create hash set for this inbound and store the incoming hash
		hs := hashedset.New()
		if incomingHash != "" {
			// Store the incoming hash directly (using "users" as the key)
			hs.SetHashValue("users", incomingHash)
		}
		s.inboundHashSets[inbound.Tag] = hs

		// Map users to this inbound
		for _, client := range inbound.Settings.Clients {
			if client.Email == "" {
				continue
			}
			if s.userInboundMap[client.Email] == nil {
				s.userInboundMap[client.Email] = make(map[string]struct{})
			}
			s.userInboundMap[client.Email][inbound.Tag] = struct{}{}
		}

		s.logger.Debug("Extracted inbound",
			zap.String("tag", inbound.Tag),
			zap.Int("users", len(inbound.Settings.Clients)),
			zap.String("hash", incomingHash))
	}

	s.logger.Info("Extracted users from config",
		zap.Int("inbounds", len(s.xtlsConfigInbounds)),
		zap.Int("users", len(s.userInboundMap)))

	return nil
}

// InboundHashItem represents a single inbound hash (Node.js array format)
type InboundHashItem struct {
	Tag        string `json:"tag"`
	Hash       string `json:"hash"`
	UsersCount int    `json:"usersCount,omitempty"`
}

// InboundHashes represents hash values for config comparison (Node.js format)
// Uses array format: inbounds: [{tag, hash, usersCount}]
type InboundHashes struct {
	EmptyConfig string            `json:"emptyConfig"`
	Inbounds    []InboundHashItem `json:"inbounds"`
}

// GetInboundHash returns the hash for a specific inbound tag
func (h *InboundHashes) GetInboundHash(tag string) (string, bool) {
	for _, item := range h.Inbounds {
		if item.Tag == tag {
			return item.Hash, true
		}
	}
	return "", false
}

// InboundsCount returns the number of inbounds
func (h *InboundHashes) InboundsCount() int {
	return len(h.Inbounds)
}

// IsNeedRestartCore checks if core restart is needed by comparing hashes
func (s *InternalService) IsNeedRestartCore(hashes *InboundHashes) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.disableHashCheck {
		return true
	}

	// If no stored hash, need restart
	if s.emptyConfigHash == "" {
		s.logger.Debug("No stored config hash, need restart")
		return true
	}

	// Compare empty config hash
	if s.emptyConfigHash != hashes.EmptyConfig {
		s.logger.Warn("Detected changes in Xray Core base configuration",
			zap.String("current", s.emptyConfigHash),
			zap.String("new", hashes.EmptyConfig))
		return true
	}

	// Compare number of inbounds
	if len(hashes.Inbounds) != len(s.inboundHashSets) {
		s.logger.Warn("Number of Xray Core inbounds has changed",
			zap.Int("current", len(s.inboundHashSets)),
			zap.Int("new", len(hashes.Inbounds)))
		return true
	}

	// Compare per-inbound hashes (using array format)
	for _, item := range hashes.Inbounds {
		hs, exists := s.inboundHashSets[item.Tag]
		if !exists {
			s.logger.Warn("New inbound detected", zap.String("tag", item.Tag))
			return true
		}
		currentHash, _ := hs.GetHash("users")
		if currentHash != item.Hash {
			s.logger.Warn("User configuration changed for inbound",
				zap.String("tag", item.Tag),
				zap.String("current", currentHash),
				zap.String("new", item.Hash))
			return true
		}
	}

	// Check if any existing inbounds were removed
	// Build a set of incoming tags for quick lookup
	incomingTags := make(map[string]struct{}, len(hashes.Inbounds))
	for _, item := range hashes.Inbounds {
		incomingTags[item.Tag] = struct{}{}
	}
	for tag := range s.inboundHashSets {
		if _, exists := incomingTags[tag]; !exists {
			s.logger.Warn("Inbound no longer exists", zap.String("tag", tag))
			return true
		}
	}

	s.logger.Info("Xray Core configuration is up-to-date - no restart required")
	return false
}

// UpdateInboundHash updates the hash for a specific inbound
func (s *InternalService) UpdateInboundHash(tag string, data json.RawMessage) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hs, exists := s.inboundHashSets[tag]
	if !exists {
		hs = hashedset.New()
		s.inboundHashSets[tag] = hs
	}

	return hs.UpdateIfChanged("users", data)
}

// SetEmptyConfigHash sets the hash for empty config (without users)
func (s *InternalService) SetEmptyConfigHash(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emptyConfigHash = hash
}

// GetEmptyConfigHash returns the current empty config hash
func (s *InternalService) GetEmptyConfigHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.emptyConfigHash
}

// GetInboundHashes returns all current hashes
func (s *InternalService) GetInboundHashes() *InboundHashes {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inbounds := make([]InboundHashItem, 0, len(s.inboundHashSets))
	for tag, hs := range s.inboundHashSets {
		hash, _ := hs.GetHash("users")
		inbounds = append(inbounds, InboundHashItem{
			Tag:  tag,
			Hash: hash,
		})
	}

	return &InboundHashes{
		EmptyConfig: s.emptyConfigHash,
		Inbounds:    inbounds,
	}
}

// GetUserCount returns the total number of tracked users
func (s *InternalService) GetUserCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.userInboundMap)
}

// GetConfigResponse represents the current stored configuration
type GetConfigResponse struct {
	Config     json.RawMessage `json:"config"`
	ConfigHash string          `json:"configHash,omitempty"`
}

// GetConfig returns the current stored configuration
func (s *InternalService) GetConfig() *GetConfigResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash, _ := s.hashedSet.GetHash("config")
	return &GetConfigResponse{
		Config:     s.config,
		ConfigHash: hash,
	}
}

// SetConfigRequest represents a request to store configuration
type SetConfigRequest struct {
	Config json.RawMessage `json:"config"`
}

// SetConfigResponse represents the response from setting config
type SetConfigResponse struct {
	Success bool   `json:"success"`
	Changed bool   `json:"changed"`
	Hash    string `json:"hash,omitempty"`
}

// SetConfig stores a configuration and checks for changes
func (s *InternalService) SetConfig(req *SetConfigRequest) *SetConfigResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if config has changed
	changed := true
	if !s.disableHashCheck {
		var err error
		changed, err = s.hashedSet.UpdateIfChanged("config", req.Config)
		if err != nil {
			s.logger.Warn("Failed to compute config hash", zap.Error(err))
		}
	}

	if changed || s.disableHashCheck {
		s.config = req.Config
		s.logger.Debug("Config updated", zap.Bool("changed", changed))
	}

	hash, _ := s.hashedSet.GetHash("config")
	return &SetConfigResponse{
		Success: true,
		Changed: changed,
		Hash:    hash,
	}
}

// CheckHashRequest represents a request to check if data has changed
type CheckHashRequest struct {
	Key  string          `json:"key"`
	Data json.RawMessage `json:"data"`
}

// CheckHashResponse represents whether data has changed
type CheckHashResponse struct {
	Changed bool   `json:"changed"`
	Hash    string `json:"hash,omitempty"`
}

// CheckHash checks if data has changed from stored hash
func (s *InternalService) CheckHash(req *CheckHashRequest) (*CheckHashResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.disableHashCheck {
		return &CheckHashResponse{Changed: true}, nil
	}

	changed, err := s.hashedSet.HasChanged(req.Key, req.Data)
	if err != nil {
		return nil, err
	}

	hash, _ := s.hashedSet.GetHash(req.Key)
	return &CheckHashResponse{
		Changed: changed,
		Hash:    hash,
	}, nil
}

// UpdateHashRequest represents a request to update hash
type UpdateHashRequest struct {
	Key  string          `json:"key"`
	Data json.RawMessage `json:"data"`
}

// UpdateHashResponse represents the result of updating hash
type UpdateHashResponse struct {
	Updated bool   `json:"updated"`
	Hash    string `json:"hash,omitempty"`
}

// UpdateHash updates the hash for a key if data changed
func (s *InternalService) UpdateHash(req *UpdateHashRequest) (*UpdateHashResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	updated, err := s.hashedSet.UpdateIfChanged(req.Key, req.Data)
	if err != nil {
		return nil, err
	}

	hash, _ := s.hashedSet.GetHash(req.Key)
	return &UpdateHashResponse{
		Updated: updated,
		Hash:    hash,
	}, nil
}

// ClearHashSet clears all stored hashes
func (s *InternalService) ClearHashSet() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashedSet.Clear()
	s.logger.Info("Cleared hash set")
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}
