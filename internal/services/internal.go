// Package services provides business logic for internal operations
package services

import (
	"encoding/json"

	"go.uber.org/zap"

	"github.com/Mikimiya/remnawave-node/pkg/hashedset"
)

// InternalService manages internal node state.
//
// Thread safety: InternalService does NOT have its own mutex.
// All methods must be called while the global RWMutex is already held by the caller
// (XrayService, HandlerService, etc.).
//
// Design note: this implementation intentionally does NOT store user credentials
// ("storedUsers"). The original Go rewrite had such a cache to re-add users after
// an embedded-xray restart, but it introduced uncontrollable drift (users removed
// by the Panel could be silently re-added). The Node.js version never had this cache:
// after any xray restart the Panel's periodic sync re-pushes all active users, which
// is the correct and reliable recovery path.
type InternalService struct {
	logger           *zap.Logger
	hashedSet        *hashedset.HashedSet
	xrayConfig       json.RawMessage
	disableHashCheck bool

	// User-Inbound tracking: user (hash UUID) -> set of inbound tags
	userInboundMap map[string]map[string]struct{}
	// Per-inbound hash sets for fine-grained change detection.
	// Uses InboundHashedSet which replicates @remnawave/hashed-set (DJB2 dual XOR).
	// Named to match Node.js: this.inboundsHashMap
	inboundsHashMap map[string]*hashedset.InboundHashedSet
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
		inboundsHashMap:    make(map[string]*hashedset.InboundHashedSet),
		xtlsConfigInbounds: make(map[string]struct{}),
	}
}

// GetXtlsConfigInbounds returns all known inbound tags
func (s *InternalService) GetXtlsConfigInbounds() []string {
	result := make([]string, 0, len(s.xtlsConfigInbounds))
	for tag := range s.xtlsConfigInbounds {
		result = append(result, tag)
	}
	return result
}

// AddXtlsConfigInbound adds an inbound tag to the known set
// Matches Node.js: addXtlsConfigInbound(inboundTag)
func (s *InternalService) AddXtlsConfigInbound(inboundTag string) {
	s.xtlsConfigInbounds[inboundTag] = struct{}{}
}

// Cleanup clears internal state (called when Xray stops)
// Matches Node.js: cleanup()
func (s *InternalService) Cleanup() {
	s.logger.Info("Cleaning up internal service.")

	s.inboundsHashMap = make(map[string]*hashedset.InboundHashedSet)
	s.xtlsConfigInbounds = make(map[string]struct{})
	s.xrayConfig = nil
	s.emptyConfigHash = ""
	s.userInboundMap = make(map[string]map[string]struct{})
}

// GetUserInbounds returns all inbound tags that a user belongs to
func (s *InternalService) GetUserInbounds(email string) []string {
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

// AddUserToInbound records that a user belongs to an inbound and updates the DJB2 hash.
// Matches Node.js: addUserToInbound(inboundTag, user)
func (s *InternalService) AddUserToInbound(inboundTag, user string) {
	if s.userInboundMap[user] == nil {
		s.userInboundMap[user] = make(map[string]struct{})
	}
	s.userInboundMap[user][inboundTag] = struct{}{}

	// Update the InboundHashedSet for this inbound
	hs, exists := s.inboundsHashMap[inboundTag]
	if !exists {
		s.logger.Warn("Inbound not found in inboundsHashMap, creating new one",
			zap.String("tag", inboundTag))
		hs = hashedset.NewInboundHashedSet()
		s.inboundsHashMap[inboundTag] = hs
	}
	hs.Add(user)

	// Re-register the inbound: RemoveUserFromInbound deletes it when the last user
	// leaves, so AddUserToInbound must restore it for subsequent operations.
	s.xtlsConfigInbounds[inboundTag] = struct{}{}
}

// RemoveUserFromInbound removes a user from inbound tracking and updates the DJB2 hash.
// When the inbound has no remaining users, removes it from both maps (matches Node.js).
// Matches Node.js: removeUserFromInbound(inboundTag, user)
func (s *InternalService) RemoveUserFromInbound(inboundTag, user string) {
	if tags, exists := s.userInboundMap[user]; exists {
		delete(tags, inboundTag)
		if len(tags) == 0 {
			delete(s.userInboundMap, user)
		}
	}

	hs, exists := s.inboundsHashMap[inboundTag]
	if !exists {
		// Matches Node.js: if (!usersSet) return;
		return
	}
	hs.Delete(user)

	// Matches Node.js: if (usersSet.size === 0) { this.xtlsConfigInbounds.delete(...); ... }
	if hs.Size() == 0 {
		delete(s.inboundsHashMap, inboundTag)
		delete(s.xtlsConfigInbounds, inboundTag)
		s.logger.Debug("Inbound has no users, cleared hash tracking and inbound registry.",
			zap.String("tag", inboundTag))
	}
}

// GetUsersInInbound returns all user hash-UUIDs in a specific inbound
func (s *InternalService) GetUsersInInbound(tag string) []string {
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
			ID    string `json:"id"`    // UUID — used by Node.js HashedSet for tracking
			Email string `json:"email"` // email/username — used by Xray-core for user operations
		} `json:"clients"`
	} `json:"settings"`
}

// XrayConfigParsed represents parsed Xray config for user extraction
type XrayConfigParsed struct {
	Inbounds []XrayInbound `json:"inbounds"`
}

// ExtractUsersFromConfig parses config and builds user-inbound mapping.
// Also stores the incoming hashes for later comparison.
// Matches Node.js: extractUsersFromConfig(hashes, newConfig)
func (s *InternalService) ExtractUsersFromConfig(hashes *InboundHashes, newConfig json.RawMessage) error {
	var parsed XrayConfigParsed
	if err := json.Unmarshal(newConfig, &parsed); err != nil {
		return err
	}

	// Clear existing mappings (matches Node.js: this.cleanup())
	s.userInboundMap = make(map[string]map[string]struct{})
	s.inboundsHashMap = make(map[string]*hashedset.InboundHashedSet)
	s.xtlsConfigInbounds = make(map[string]struct{})

	// Store the config
	s.xrayConfig = newConfig

	// Build valid tags set from incoming hashes
	validTags := make(map[string]string)
	if hashes != nil {
		s.emptyConfigHash = hashes.EmptyConfig
		for _, item := range hashes.Inbounds {
			validTags[item.Tag] = item.Hash
		}
	}

	for _, inbound := range parsed.Inbounds {
		inboundTag := inbound.Tag
		if inboundTag == "" {
			continue
		}

		// Only process inbounds that are in the valid tags (from hashes)
		// Matches Node.js: if (!inboundTag || !validTags.has(inboundTag)) return;
		_, isValid := validTags[inboundTag]
		if hashes != nil && !isValid {
			continue
		}

		usersSet := hashedset.NewInboundHashedSet()
		for _, client := range inbound.Settings.Clients {
			if client.ID != "" {
				usersSet.Add(client.ID)
			}
		}

		s.inboundsHashMap[inboundTag] = usersSet

		for _, client := range inbound.Settings.Clients {
			trackingKey := client.ID
			if trackingKey == "" {
				trackingKey = client.Email
			}
			if trackingKey == "" {
				continue
			}
			if s.userInboundMap[trackingKey] == nil {
				s.userInboundMap[trackingKey] = make(map[string]struct{})
			}
			s.userInboundMap[trackingKey][inboundTag] = struct{}{}
		}
	}

	for inboundTag, usersSet := range s.inboundsHashMap {
		s.xtlsConfigInbounds[inboundTag] = struct{}{}
		s.logger.Info("Inbound loaded",
			zap.String("tag", inboundTag),
			zap.Int("users", usersSet.Size()))
	}

	return nil
}

// InboundHashItem represents a single inbound hash (Node.js array format)
type InboundHashItem struct {
	Tag        string `json:"tag"`
	Hash       string `json:"hash"`
	UsersCount int    `json:"usersCount,omitempty"`
}

// InboundHashes represents hash values for config comparison (Node.js format)
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

// IsNeedRestartCore checks if core restart is needed by comparing hashes.
// Matches Node.js: isNeedRestartCore(incomingHashes)
func (s *InternalService) IsNeedRestartCore(incomingHashes *InboundHashes) bool {
	if s.disableHashCheck {
		return true
	}

	if s.emptyConfigHash == "" {
		return true
	}

	if incomingHashes.EmptyConfig != s.emptyConfigHash {
		s.logger.Warn("Detected changes in Xray Core base configuration")
		return true
	}

	if len(incomingHashes.Inbounds) != len(s.inboundsHashMap) {
		s.logger.Warn("Number of Xray Core inbounds has changed")
		return true
	}

	for inboundTag, usersSet := range s.inboundsHashMap {
		var incomingInbound *InboundHashItem
		for i := range incomingHashes.Inbounds {
			if incomingHashes.Inbounds[i].Tag == inboundTag {
				incomingInbound = &incomingHashes.Inbounds[i]
				break
			}
		}

		if incomingInbound == nil {
			s.logger.Warn("Inbound no longer exists in Xray Core configuration",
				zap.String("tag", inboundTag))
			return true
		}

		if usersSet.Hash64String() != incomingInbound.Hash {
			s.logger.Warn("User configuration changed for inbound",
				zap.String("tag", inboundTag),
				zap.String("current", usersSet.Hash64String()),
				zap.String("new", incomingInbound.Hash))
			return true
		}
	}

	s.logger.Info("Xray Core configuration is up-to-date - no restart required")
	return false
}

// SetEmptyConfigHash sets the hash for empty config (without users)
func (s *InternalService) SetEmptyConfigHash(hash string) {
	s.emptyConfigHash = hash
}

// GetEmptyConfigHash returns the current empty config hash
func (s *InternalService) GetEmptyConfigHash() string {
	return s.emptyConfigHash
}

// GetInboundHashes returns all current hashes
func (s *InternalService) GetInboundHashes() *InboundHashes {
	inbounds := make([]InboundHashItem, 0, len(s.inboundsHashMap))
	for tag, hs := range s.inboundsHashMap {
		inbounds = append(inbounds, InboundHashItem{
			Tag:  tag,
			Hash: hs.Hash64String(),
		})
	}
	return &InboundHashes{
		EmptyConfig: s.emptyConfigHash,
		Inbounds:    inbounds,
	}
}

// GetUserCount returns the total number of tracked users
func (s *InternalService) GetUserCount() int {
	return len(s.userInboundMap)
}

// GetConfigResponse represents the current stored configuration
type GetConfigResponse struct {
	Config     json.RawMessage `json:"config"`
	ConfigHash string          `json:"configHash,omitempty"`
}

// GetConfig returns the current stored configuration
func (s *InternalService) GetConfig() *GetConfigResponse {
	hash, _ := s.hashedSet.GetHash("config")
	return &GetConfigResponse{
		Config:     s.xrayConfig,
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
	changed := true
	if !s.disableHashCheck {
		var err error
		changed, err = s.hashedSet.UpdateIfChanged("config", req.Config)
		if err != nil {
			s.logger.Warn("Failed to compute config hash", zap.Error(err))
		}
	}

	if changed || s.disableHashCheck {
		s.xrayConfig = req.Config
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
	s.hashedSet.Clear()
	s.logger.Info("Cleared hash set")
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}
