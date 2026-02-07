// Package services provides business logic for user management
package services

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/Mikimiya/remnawave-node/pkg/xraycore"
)

// HandlerService manages user operations for Xray
type HandlerService struct {
	logger   *zap.Logger
	xrayCore *xraycore.Instance
	internal *InternalService

	// Per-inbound mutex for fine-grained locking
	inboundMu    sync.RWMutex
	inboundLocks map[string]*sync.Mutex
}

// NewHandlerService creates a new HandlerService
func NewHandlerService(xrayCore *xraycore.Instance, internal *InternalService, logger *zap.Logger) *HandlerService {
	return &HandlerService{
		logger:       logger,
		xrayCore:     xrayCore,
		internal:     internal,
		inboundLocks: make(map[string]*sync.Mutex),
	}
}

// getInboundLock returns a mutex for a specific inbound tag
func (s *HandlerService) getInboundLock(tag string) *sync.Mutex {
	s.inboundMu.RLock()
	lock, exists := s.inboundLocks[tag]
	s.inboundMu.RUnlock()

	if exists {
		return lock
	}

	s.inboundMu.Lock()
	defer s.inboundMu.Unlock()

	// Double check after acquiring write lock
	if lock, exists = s.inboundLocks[tag]; exists {
		return lock
	}

	lock = &sync.Mutex{}
	s.inboundLocks[tag] = lock
	return lock
}

// CipherType represents Shadowsocks cipher types (matches Node.js CipherType enum)
type CipherType int

const (
	CipherTypeUnknown           CipherType = 0
	CipherTypeUnrecognized      CipherType = -1
	CipherTypeAES128GCM         CipherType = 5
	CipherTypeAES256GCM         CipherType = 6
	CipherTypeCHACHA20POLY1305  CipherType = 7
	CipherTypeXCHACHA20POLY1305 CipherType = 8
	CipherTypeNone              CipherType = 9
)

// UserData represents user data in the new format (Node.js discriminated union)
type UserData struct {
	Type       string     `json:"type"` // "trojan", "vless", "shadowsocks"
	Tag        string     `json:"tag"`
	Username   string     `json:"username"`
	UUID       string     `json:"uuid,omitempty"`       // For vless
	Password   string     `json:"password,omitempty"`   // For trojan/shadowsocks
	Flow       string     `json:"flow,omitempty"`       // For vless: "xtls-rprx-vision" or ""
	CipherType CipherType `json:"cipherType,omitempty"` // For shadowsocks
	IvCheck    bool       `json:"ivCheck,omitempty"`    // For shadowsocks
}

// HashData represents hash data for tracking (Node.js format)
type HashData struct {
	VlessUUID     string `json:"vlessUuid"`
	PrevVlessUUID string `json:"prevVlessUuid,omitempty"`
}

// AddUserRequest represents a request to add a single user (Node.js format)
// Format: { data: [UserData], hashData: {vlessUuid, prevVlessUuid?} }
type AddUserRequest struct {
	Data     []UserData `json:"data"`
	HashData HashData   `json:"hashData"`
}

// AddUserResponse represents the response from adding a user
// Matches Node.js AddUserResponseModel: { success: boolean, error: null | string }
type AddUserResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
}

// Legacy UserInfo for internal use
type UserInfo struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Email    string `json:"email"`
	Level    uint32 `json:"level,omitempty"`
	Flow     string `json:"flow,omitempty"`
	Method   string `json:"method,omitempty"`
	PrevUUID string `json:"prevUuid,omitempty"`
}

// removeUserFromInbound removes a user from a specific inbound (internal, no lock)
func (s *HandlerService) removeUserFromInbound(ctx context.Context, tag, email string) error {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		return fmt.Errorf("Xray not running")
	}

	if err := s.xrayCore.RemoveUser(ctx, tag, email); err != nil {
		s.logger.Debug("Failed to remove user from inbound (may not exist)",
			zap.String("email", email),
			zap.String("tag", tag),
			zap.Error(err))
		return err
	}

	s.logger.Debug("Removed user from inbound",
		zap.String("email", email),
		zap.String("tag", tag))
	return nil
}

// AddUser adds user(s) to Xray (Node.js compatible format)
// The request contains multiple UserData items (one per inbound) and hashData for tracking
func (s *HandlerService) AddUser(ctx context.Context, req *AddUserRequest) (*AddUserResponse, error) {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		errMsg := "Xray not running"
		return &AddUserResponse{Success: false, Error: &errMsg}, nil
	}

	if len(req.Data) == 0 {
		errMsg := "no user data provided"
		return &AddUserResponse{Success: false, Error: &errMsg}, nil
	}

	// Get username from first item (all items have same username)
	username := req.Data[0].Username

	// Step 1: Add all target inbounds to known inbounds
	for _, item := range req.Data {
		s.internal.AddXtlsConfigInbound(item.Tag)
	}

	// Step 2: Remove user from ALL known inbounds first (like Node.js does)
	allTags := s.internal.GetXtlsConfigInbounds()
	for _, tag := range allTags {
		lock := s.getInboundLock(tag)
		lock.Lock()

		s.logger.Debug("Removing user from inbound before adding",
			zap.String("username", username),
			zap.String("tag", tag))

		_ = s.removeUserFromInbound(ctx, tag, username)

		// Also remove prevVlessUuid user if exists
		if req.HashData.PrevVlessUUID != "" {
			s.internal.RemoveUserFromInbound(req.HashData.PrevVlessUUID, tag)
		} else {
			s.internal.RemoveUserFromInbound(req.HashData.VlessUUID, tag)
		}

		lock.Unlock()
	}

	// Step 3: Add user to each inbound based on type
	var lastError error
	successCount := 0

	for _, item := range req.Data {
		lock := s.getInboundLock(item.Tag)
		lock.Lock()

		var err error

		switch item.Type {
		case "trojan":
			user, createErr := xraycore.CreateTrojanUser(item.Username, item.Password, 0)
			if createErr != nil {
				err = createErr
			} else {
				err = s.xrayCore.AddUser(ctx, item.Tag, user)
			}
		case "vless":
			user, createErr := xraycore.CreateVlessUser(item.Username, item.UUID, item.Flow, 0)
			if createErr != nil {
				err = createErr
			} else {
				err = s.xrayCore.AddUser(ctx, item.Tag, user)
			}
		case "shadowsocks":
			cipherType := xraycore.CipherTypeFromInt(int(item.CipherType))
			user, createErr := xraycore.CreateShadowsocksUser(item.Username, item.Password, cipherType, 0)
			if createErr != nil {
				err = createErr
			} else {
				err = s.xrayCore.AddUser(ctx, item.Tag, user)
			}
		default:
			s.logger.Warn("Unknown user type", zap.String("type", item.Type))
			lock.Unlock()
			continue
		}

		if err != nil {
			s.logger.Error("Failed to add user",
				zap.String("username", item.Username),
				zap.String("tag", item.Tag),
				zap.String("type", item.Type),
				zap.Error(err))
			lastError = err
		} else {
			// Update tracking on success
			s.internal.AddUserToInbound(req.HashData.VlessUUID, item.Tag)
			successCount++

			s.logger.Info("Added user",
				zap.String("username", item.Username),
				zap.String("tag", item.Tag),
				zap.String("type", item.Type))
		}

		lock.Unlock()
	}

	// Return success if at least one user was added
	if successCount > 0 {
		return &AddUserResponse{Success: true, Error: nil}, nil
	}

	// All failed
	if lastError != nil {
		errMsg := lastError.Error()
		return &AddUserResponse{Success: false, Error: &errMsg}, nil
	}

	errMsg := "no users were added"
	return &AddUserResponse{Success: false, Error: &errMsg}, nil
}

// cipherTypeToMethod converts CipherType enum to method string
func cipherTypeToMethod(ct CipherType) string {
	switch ct {
	case CipherTypeAES128GCM:
		return "aes-128-gcm"
	case CipherTypeAES256GCM:
		return "aes-256-gcm"
	case CipherTypeCHACHA20POLY1305:
		return "chacha20-poly1305"
	case CipherTypeXCHACHA20POLY1305:
		return "xchacha20-poly1305"
	case CipherTypeNone:
		return "none"
	default:
		return "aes-256-gcm"
	}
}

// InboundData represents inbound configuration for a user (Node.js format)
type InboundData struct {
	Type string `json:"type"` // "trojan", "vless", "shadowsocks"
	Tag  string `json:"tag"`
	Flow string `json:"flow,omitempty"` // For vless
}

// UserDataForBatch represents user data in batch request (Node.js format)
type UserDataForBatch struct {
	UserId         string `json:"userId"`
	HashUuid       string `json:"hashUuid"`
	VlessUuid      string `json:"vlessUuid"`
	TrojanPassword string `json:"trojanPassword"`
	SsPassword     string `json:"ssPassword"`
}

// UserForBatch represents a user in batch add request (Node.js format)
type UserForBatch struct {
	InboundData []InboundData    `json:"inboundData"`
	UserData    UserDataForBatch `json:"userData"`
}

// AddUsersRequest represents a request to add multiple users (Node.js format)
type AddUsersRequest struct {
	AffectedInboundTags []string       `json:"affectedInboundTags"`
	Users               []UserForBatch `json:"users"`
}

// AddUsersResponse represents the response from adding multiple users
// Matches Node.js: { success: boolean, error: null | string }
type AddUsersResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
}

// AddUsers adds multiple users to Xray (Node.js compatible format)
func (s *HandlerService) AddUsers(ctx context.Context, req *AddUsersRequest) (*AddUsersResponse, error) {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		errMsg := "Xray not running"
		return &AddUsersResponse{Success: false, Error: &errMsg}, nil
	}

	// Add affected inbound tags to known inbounds
	for _, tag := range req.AffectedInboundTags {
		s.internal.AddXtlsConfigInbound(tag)
	}

	s.logger.Info("Adding users to inbounds",
		zap.Int("users", len(req.Users)),
		zap.Strings("inbounds", req.AffectedInboundTags))

	for _, user := range req.Users {
		// Step 1: Remove user from ALL known inbounds first
		allTags := s.internal.GetXtlsConfigInbounds()
		for _, tag := range allTags {
			lock := s.getInboundLock(tag)
			lock.Lock()

			_ = s.removeUserFromInbound(ctx, tag, user.UserData.UserId)
			s.internal.RemoveUserFromInbound(user.UserData.HashUuid, tag)

			lock.Unlock()
		}

		// Step 2: Add user to each inbound based on type
		for _, item := range user.InboundData {
			lock := s.getInboundLock(item.Tag)
			lock.Lock()

			var err error

			switch item.Type {
			case "trojan":
				u, createErr := xraycore.CreateTrojanUser(user.UserData.UserId, user.UserData.TrojanPassword, 0)
				if createErr != nil {
					err = createErr
				} else {
					err = s.xrayCore.AddUser(ctx, item.Tag, u)
				}
			case "vless":
				u, createErr := xraycore.CreateVlessUser(user.UserData.UserId, user.UserData.VlessUuid, item.Flow, 0)
				if createErr != nil {
					err = createErr
				} else {
					err = s.xrayCore.AddUser(ctx, item.Tag, u)
				}
			case "shadowsocks":
				cipherType := xraycore.CipherTypeFromInt(7) // chacha20-poly1305 default
				u, createErr := xraycore.CreateShadowsocksUser(user.UserData.UserId, user.UserData.SsPassword, cipherType, 0)
				if createErr != nil {
					err = createErr
				} else {
					err = s.xrayCore.AddUser(ctx, item.Tag, u)
				}
			default:
				s.logger.Warn("Unknown user type", zap.String("type", item.Type))
				lock.Unlock()
				continue
			}

			if err != nil {
				s.logger.Warn("Failed to add user",
					zap.String("userId", user.UserData.UserId),
					zap.String("tag", item.Tag),
					zap.Error(err))
			} else {
				s.internal.AddUserToInbound(user.UserData.VlessUuid, item.Tag)
				s.logger.Debug("Added user",
					zap.String("userId", user.UserData.UserId),
					zap.String("tag", item.Tag))
			}

			lock.Unlock()
		}
	}

	s.logger.Info("Batch add users completed", zap.Int("users", len(req.Users)))

	return &AddUsersResponse{Success: true, Error: nil}, nil
}

// RemoveUserHashData represents hash data in remove request (Node.js format)
type RemoveUserHashData struct {
	VlessUUID string `json:"vlessUuid"`
}

// RemoveUserRequest represents a request to remove a user (Node.js format)
type RemoveUserRequest struct {
	Username string             `json:"username"`
	HashData RemoveUserHashData `json:"hashData"`
}

// RemoveUserResponse represents the response from removing a user
// Matches Node.js RemoveUserResponseModel: { success: boolean, error: null | string }
type RemoveUserResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
}

// RemoveUser removes a user from ALL known inbounds (Node.js compatible)
func (s *HandlerService) RemoveUser(ctx context.Context, req *RemoveUserRequest) (*RemoveUserResponse, error) {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		errMsg := "Xray not running"
		return &RemoveUserResponse{Success: false, Error: &errMsg}, nil
	}

	// Get all known inbounds
	allTags := s.internal.GetXtlsConfigInbounds()
	if len(allTags) == 0 {
		return &RemoveUserResponse{Success: true, Error: nil}, nil
	}

	successCount := 0
	failCount := 0
	var lastError error

	// Remove from all inbounds
	for _, tag := range allTags {
		lock := s.getInboundLock(tag)
		lock.Lock()

		s.logger.Debug("Removing user from inbound",
			zap.String("username", req.Username),
			zap.String("tag", tag))

		if err := s.xrayCore.RemoveUser(ctx, tag, req.Username); err != nil {
			failCount++
			lastError = err
		} else {
			successCount++
		}
		s.internal.RemoveUserFromInbound(req.HashData.VlessUUID, tag)

		lock.Unlock()
	}

	s.logger.Info("Removed user from all inbounds",
		zap.String("username", req.Username),
		zap.Int("success", successCount),
		zap.Int("failed", failCount))

	// If ALL operations failed, return error (matches Node.js behavior)
	if successCount == 0 && failCount > 0 {
		errMsg := "all remove operations failed"
		if lastError != nil {
			errMsg = lastError.Error()
		}
		return &RemoveUserResponse{Success: false, Error: &errMsg}, nil
	}

	return &RemoveUserResponse{Success: true, Error: nil}, nil
}

// RemoveUserItem represents a user to remove in batch (Node.js format)
type RemoveUserItem struct {
	UserId   string `json:"userId"`
	HashUuid string `json:"hashUuid"`
}

// RemoveUsersRequest represents a request to remove multiple users (Node.js format)
type RemoveUsersRequest struct {
	Users []RemoveUserItem `json:"users"`
}

// RemoveUsersResponse represents the response from removing multiple users
// Matches Node.js: { success: boolean, error: null | string }
type RemoveUsersResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
}

// RemoveUsers removes multiple users from ALL known inbounds (Node.js compatible)
func (s *HandlerService) RemoveUsers(ctx context.Context, req *RemoveUsersRequest) (*RemoveUsersResponse, error) {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		errMsg := "Xray not running"
		return &RemoveUsersResponse{Success: false, Error: &errMsg}, nil
	}

	// Get all known inbounds
	allTags := s.internal.GetXtlsConfigInbounds()
	if len(allTags) == 0 {
		return &RemoveUsersResponse{Success: true, Error: nil}, nil
	}

	s.logger.Info("Removing users from all inbounds",
		zap.Int("users", len(req.Users)),
		zap.Strings("inbounds", allTags))

	successCount := 0
	failCount := 0
	var lastError error

	for _, user := range req.Users {
		// Remove from all known inbounds
		for _, tag := range allTags {
			lock := s.getInboundLock(tag)
			lock.Lock()

			s.logger.Debug("Removing user from inbound",
				zap.String("userId", user.UserId),
				zap.String("tag", tag))

			if err := s.xrayCore.RemoveUser(ctx, tag, user.UserId); err != nil {
				failCount++
				lastError = err
			} else {
				successCount++
			}
			s.internal.RemoveUserFromInbound(user.HashUuid, tag)

			lock.Unlock()
		}
	}

	s.logger.Info("Batch remove users completed",
		zap.Int("users", len(req.Users)),
		zap.Int("success", successCount),
		zap.Int("failed", failCount))

	// If ALL operations failed, return error (matches Node.js behavior)
	if successCount == 0 && failCount > 0 {
		errMsg := "all remove operations failed"
		if lastError != nil {
			errMsg = lastError.Error()
		}
		return &RemoveUsersResponse{Success: false, Error: &errMsg}, nil
	}

	return &RemoveUsersResponse{Success: true, Error: nil}, nil
}

// InboundUserInfo represents a user in an inbound (matches Node.js IInboundUser)
// InboundUserInfo represents a user in an inbound (matches Node.js format)
type InboundUserInfo struct {
	Username string  `json:"username"`
	Email    *string `json:"email,omitempty"`
	Level    *uint32 `json:"level,omitempty"`
}

// GetInboundUsersResponse represents the response for getting inbound users
type GetInboundUsersResponse struct {
	Users []InboundUserInfo `json:"users"`
}

// GetInboundUsers returns the list of users in the specified inbound
// Note: With embedded Xray-core, we rely on internal tracking
func (s *HandlerService) GetInboundUsers(ctx context.Context, tag string) (*GetInboundUsersResponse, error) {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		return &GetInboundUsersResponse{
			Users: []InboundUserInfo{},
		}, fmt.Errorf("Xray not running")
	}

	// Use internal service to get tracked users for this inbound
	trackedUsers := s.internal.GetUsersInInbound(tag)
	users := make([]InboundUserInfo, len(trackedUsers))
	for i, username := range trackedUsers {
		users[i] = InboundUserInfo{
			Username: username,
		}
	}

	return &GetInboundUsersResponse{
		Users: users,
	}, nil
}

// GetInboundUsersCountResponse represents the response for getting inbound users count
// Matches Node.js GetInboundUsersCountResponseModel: { count: number }
type GetInboundUsersCountResponse struct {
	Count int64 `json:"count"`
}

// GetInboundUsersCount returns the count of users in the specified inbound
// Note: With embedded Xray-core, we rely on internal tracking
func (s *HandlerService) GetInboundUsersCount(ctx context.Context, tag string) (*GetInboundUsersCountResponse, error) {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		return &GetInboundUsersCountResponse{
			Count: 0,
		}, fmt.Errorf("Xray not running")
	}

	// Use internal service to get count of tracked users
	count := s.internal.GetUsersCountInInbound(tag)

	return &GetInboundUsersCountResponse{
		Count: int64(count),
	}, nil
}
