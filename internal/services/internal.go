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
// (XrayService, HandlerService, etc.). This is safe because all calls come from
// service methods that already hold the appropriate lock.
type InternalService struct {
	logger           *zap.Logger
	hashedSet        *hashedset.HashedSet
	xrayConfig       json.RawMessage
	disableHashCheck bool

	// User-Inbound tracking: user -> set of inbound tags
	userInboundMap map[string]map[string]struct{}
	// Per-inbound hash sets for fine-grained change detection.
	// Uses InboundHashedSet which replicates @remnawave/hashed-set (DJB2 dual XOR).
	// The hash dynamically updates when users are added/removed via
	// AddUserToInbound / RemoveUserFromInbound, matching Node.js behavior.
	// Named to match Node.js: this.inboundsHashMap
	inboundsHashMap map[string]*hashedset.InboundHashedSet
	// Empty config hash (config without users)
	emptyConfigHash string
	// All known inbound tags (used for removing users from all inbounds)
	xtlsConfigInbounds map[string]struct{}

	// Stored user credentials for re-adding after Xray restart.
	// Key: username (email), Value: StoredUser with full credentials
	// This is needed because Panel sends empty clients[] in config and adds users dynamically.
	// When Xray restarts, we need to re-add all these dynamic users.
	storedUsers map[string]*StoredUser
}

// StoredUser holds user credentials for re-adding after Xray restart
type StoredUser struct {
	Username       string                  // email/userId used as unique identifier
	HashUUID       string                  // vlessUuid used for hash tracking
	VlessUUID      string                  // actual VLESS UUID
	TrojanPassword string                  // Trojan password
	SSPassword     string                  // Shadowsocks password
	Inbounds       []StoredUserInboundData // inbound configurations
}

// StoredUserInboundData holds inbound-specific data for a stored user
type StoredUserInboundData struct {
	Tag        string // inbound tag
	Type       string // "vless", "trojan", "shadowsocks"
	Flow       string // for VLESS
	CipherType int    // for Shadowsocks
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
		storedUsers:        make(map[string]*StoredUser),
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
// NOTE: storedUsers is NOT cleared here - it's needed to re-add users after restart.
// Call ClearStoredUsers() explicitly if you want to clear all users.
func (s *InternalService) Cleanup() {

	s.logger.Info("Cleaning up internal service.")

	s.inboundsHashMap = make(map[string]*hashedset.InboundHashedSet)
	s.xtlsConfigInbounds = make(map[string]struct{})
	s.xrayConfig = nil
	s.emptyConfigHash = ""

	// Go-specific: also clear userInboundMap
	s.userInboundMap = make(map[string]map[string]struct{})

	// NOTE: storedUsers is intentionally NOT cleared - needed for restart
}

// StoreUser saves user credentials for re-adding after Xray restart
func (s *InternalService) StoreUser(user *StoredUser) {
	if user == nil || user.Username == "" {
		return
	}
	s.storedUsers[user.Username] = user
	s.logger.Debug("Stored user credentials for restart recovery",
		zap.String("username", user.Username),
		zap.Int("inboundCount", len(user.Inbounds)))
}

// RemoveStoredUser removes a user from stored credentials
func (s *InternalService) RemoveStoredUser(username string) {
	if _, exists := s.storedUsers[username]; exists {
		delete(s.storedUsers, username)
		s.logger.Debug("Removed stored user credentials",
			zap.String("username", username))
	}
}

// GetAllStoredUsers returns all stored users for re-adding after restart
func (s *InternalService) GetAllStoredUsers() []*StoredUser {
	result := make([]*StoredUser, 0, len(s.storedUsers))
	for _, user := range s.storedUsers {
		result = append(result, user)
	}
	return result
}

// GetStoredUsersCount returns the count of stored users
func (s *InternalService) GetStoredUsersCount() int {
	return len(s.storedUsers)
}

// ClearStoredUsers clears all stored user credentials (use when doing full cleanup)
func (s *InternalService) ClearStoredUsers() {
	s.storedUsers = make(map[string]*StoredUser)
	s.logger.Info("Cleared all stored user credentials")
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

// AddUserToInbound records that a user belongs to an inbound.
// Also adds the user to the InboundHashedSet so the DJB2 hash stays in sync.
// Matches Node.js: addUserToInbound(inboundTag, user)
func (s *InternalService) AddUserToInbound(inboundTag, user string) {

	if s.userInboundMap[user] == nil {
		s.userInboundMap[user] = make(map[string]struct{})
	}
	s.userInboundMap[user][inboundTag] = struct{}{}

	// Update the InboundHashedSet for this inbound
	hs, exists := s.inboundsHashMap[inboundTag]
	if !exists {
		// Inbound not yet tracked — create a new InboundHashedSet
		s.logger.Warn("Inbound not found in inboundsHashMap, creating new one",
			zap.String("tag", inboundTag))
		hs = hashedset.NewInboundHashedSet()
		s.inboundsHashMap[inboundTag] = hs
	}
	hs.Add(user)

	// Ensure the inbound is also registered in xtlsConfigInbounds.
	// This is critical: RemoveUserFromInbound deletes the tag from xtlsConfigInbounds
	// when the last user is removed. If a new user is subsequently added to the same
	// inbound (e.g., in an AddUsers loop), the tag must be restored so that later
	// iterations of GetXtlsConfigInbounds() still see this inbound.
	s.xtlsConfigInbounds[inboundTag] = struct{}{}
}

// RemoveUserFromInbound removes a user from an inbound tracking.
// Also removes the user from the InboundHashedSet so the DJB2 hash stays in sync.
// When the inbound has no remaining users, also cleans up xtlsConfigInbounds and
// inboundsHashMap (matches Node.js behavior where both inboundsHashMap and
// xtlsConfigInbounds are cleared when usersSet.size === 0).
// Matches Node.js: removeUserFromInbound(inboundTag, user)
func (s *InternalService) RemoveUserFromInbound(inboundTag, user string) {

	// Update the InboundHashedSet — matches Node.js: usersSet.delete(user)
	hs, exists := s.inboundsHashMap[inboundTag]
	if !exists {
		// Matches Node.js: if (!usersSet) return;
		return
	}
	hs.Delete(user)

	// Also update userInboundMap for internal tracking
	if tags, exists := s.userInboundMap[user]; exists {
		delete(tags, inboundTag)
		if len(tags) == 0 {
			delete(s.userInboundMap, user)
		}
	}

	// Check if the inbound now has zero users using InboundHashedSet.Size()
	// This matches Node.js: if (usersSet.size === 0) { ... }
	if hs.Size() == 0 {
		// NOTE: We intentionally do NOT delete from xtlsConfigInbounds here.
		// Deleting the tag causes a critical bug: if Panel calls RemoveUser after
		// the last user is removed, allTags would be empty and the user would
		// remain in xray-core without being cleaned up.
		// We only clear inboundsHashMap (hash tracking), not the inbound registry.
		delete(s.inboundsHashMap, inboundTag)
		s.logger.Debug("Inbound has no users, cleared hash tracking (inbound kept in registry).",
			zap.String("tag", inboundTag))
	}
}

// GetUsersInInbound returns all user emails in a specific inbound
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

// ExtractUsersFromConfig parses config and builds user-inbound mapping
// Also stores the incoming hashes for later comparison
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

	// Store the config (matches Node.js: this.xrayConfig = newConfig)
	s.xrayConfig = newConfig

	// Build valid tags set from incoming hashes
	validTags := make(map[string]string) // tag -> hash
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

		// Matches Node.js: const usersSet = new HashedSet();
		usersSet := hashedset.NewInboundHashedSet()

		// Matches Node.js: if (inbound.settings?.clients) for (client of clients) usersSet.add(client.id)
		for _, client := range inbound.Settings.Clients {
			if client.ID != "" {
				usersSet.Add(client.ID)
			}
		}

		// Matches Node.js: this.inboundsHashMap.set(inboundTag, usersSet)
		s.inboundsHashMap[inboundTag] = usersSet

		// Also populate userInboundMap for internal tracking (Go-specific, needed for GetUsersInInbound)
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

	// Matches Node.js: for (const [inboundTag, usersSet] of this.inboundsHashMap) { xtlsConfigInbounds.add(inboundTag); }
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
// Matches Node.js: isNeedRestartCore(incomingHashes)
func (s *InternalService) IsNeedRestartCore(incomingHashes *InboundHashes) bool {

	if s.disableHashCheck {
		return true
	}

	// If no stored hash, need restart
	if s.emptyConfigHash == "" {
		return true
	}

	// Compare empty config hash
	if incomingHashes.EmptyConfig != s.emptyConfigHash {
		s.logger.Warn("Detected changes in Xray Core base configuration")
		return true
	}

	// Compare number of inbounds
	if len(incomingHashes.Inbounds) != len(s.inboundsHashMap) {
		s.logger.Warn("Number of Xray Core inbounds has changed")
		return true
	}

	// Compare per-inbound hashes — iterate stored inboundsHashMap, find in incoming
	// Matches Node.js: for (const [inboundTag, usersSet] of this.inboundsHashMap)
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

// UpdateInboundHash is a no-op for compatibility. With InboundHashedSet, hashes are
// dynamically maintained via Add/Delete. This method is kept for interface compatibility.
func (s *InternalService) UpdateInboundHash(tag string, data json.RawMessage) (bool, error) {

	_, exists := s.inboundsHashMap[tag]
	if !exists {
		s.inboundsHashMap[tag] = hashedset.NewInboundHashedSet()
	}
	// With InboundHashedSet, the hash is automatically computed from members.
	// There's nothing to "update" from external data — return false (no change).
	return false, nil
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
