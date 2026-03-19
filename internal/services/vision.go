// Package services provides business logic for IP blocking (Vision)
package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/Mikimiya/remnawave-node/pkg/xraycore"
)

// VisionService manages IP blocking via Xray router rules.
//
// Concurrency: uses the same global RWMutex shared across all services.
// BlockIP / UnblockIP acquire a WRITE lock (mutations).
//
// Design note: No local IP tracking map. After any xray restart routing rules
// are wiped, so a local cache would become stale. Instead, every call goes
// directly through to xray — matching Node.js which always calls gRPC directly.
// "already exists" / "not found" errors from xray are treated as success.
type VisionService struct {
	logger   *zap.Logger
	xrayCore *xraycore.Instance
	blockTag string

	// Global RWMutex shared across all services.
	mu *sync.RWMutex
}

// VisionConfig holds Vision service configuration
type VisionConfig struct {
	BlockTag string // The outbound tag for blocked traffic (e.g., "block" or "BLOCK")
}

// NewVisionService creates a new VisionService
func NewVisionService(cfg *VisionConfig, xrayCore *xraycore.Instance, mu *sync.RWMutex, logger *zap.Logger) *VisionService {
	blockTag := cfg.BlockTag
	if blockTag == "" {
		blockTag = "BLOCK"
	}
	return &VisionService{
		logger:   logger,
		xrayCore: xrayCore,
		blockTag: blockTag,
		mu:       mu,
	}
}

// getIPHash returns MD5 hash of an IP address, compatible with Node.js object-hash.
//
// Node.js object-hash serializes strings as "string:<length>:<value>" before hashing.
// See: https://github.com/puleos/object-hash/blob/main/index.js (_string method)
//
//	objectHash("192.168.1.1", {algorithm:'md5', encoding:'hex'})
//	  => MD5("string:11:192.168.1.1")
func (s *VisionService) getIPHash(ip string) string {
	serialized := fmt.Sprintf("string:%d:%s", len(ip), ip)
	hash := md5.Sum([]byte(serialized))
	return hex.EncodeToString(hash[:])
}

// BlockIPRequest represents a request to block an IP (Node.js format)
type BlockIPRequest struct {
	IP       string `json:"ip"`
	Username string `json:"username"` // For logging/tracking, not used in blocking logic
}

// BlockIPResponse represents the response from blocking an IP
// Matches Node.js BlockIpResponseModel: { success: boolean, error: null | string }
type BlockIPResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
}

// BlockIP blocks an IP address by adding a routing rule to xray.
// Always calls through to xray directly — no local cache.
// If xray is not running, returns success (rule will be absent, but so is xray).
func (s *VisionService) BlockIP(ctx context.Context, req *BlockIPRequest) (*BlockIPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		s.logger.Warn("BlockIP: xray not running, skipping",
			zap.String("ip", req.IP))
		return &BlockIPResponse{Success: true, Error: nil}, nil
	}

	ruleTag := s.getIPHash(req.IP)

	if err := s.xrayCore.AddRoutingRule(ctx, ruleTag, req.IP, s.blockTag); err != nil {
		// "already exists" means rule is already in xray — treat as success
		if strings.Contains(err.Error(), "already exists") {
			s.logger.Debug("BlockIP: rule already exists in xray",
				zap.String("ip", req.IP))
			return &BlockIPResponse{Success: true, Error: nil}, nil
		}
		s.logger.Error("Failed to add block rule",
			zap.String("ip", req.IP),
			zap.String("ruleTag", ruleTag),
			zap.Error(err))
		errMsg := err.Error()
		return &BlockIPResponse{Success: false, Error: &errMsg}, nil
	}

	s.logger.Info("Blocked IP",
		zap.String("ip", req.IP),
		zap.String("ruleTag", ruleTag))

	return &BlockIPResponse{Success: true, Error: nil}, nil
}

// UnblockIPRequest represents a request to unblock an IP (Node.js format)
type UnblockIPRequest struct {
	IP       string `json:"ip"`
	Username string `json:"username"` // For logging/tracking, not used in unblocking logic
}

// UnblockIPResponse represents the response from unblocking an IP
// Matches Node.js UnblockIpResponseModel: { success: boolean, error: null | string }
type UnblockIPResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
}

// UnblockIP unblocks an IP address by removing its routing rule from xray.
// Always calls through to xray directly — no local cache.
// If xray is not running, returns success.
func (s *VisionService) UnblockIP(ctx context.Context, req *UnblockIPRequest) (*UnblockIPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		s.logger.Warn("UnblockIP: xray not running, skipping",
			zap.String("ip", req.IP))
		return &UnblockIPResponse{Success: true, Error: nil}, nil
	}

	ruleTag := s.getIPHash(req.IP)

	if err := s.xrayCore.RemoveRoutingRule(ctx, ruleTag); err != nil {
		// "not found" means rule is already absent in xray — treat as success
		if strings.Contains(err.Error(), "not found") {
			s.logger.Debug("UnblockIP: rule not found in xray (already removed)",
				zap.String("ip", req.IP))
			return &UnblockIPResponse{Success: true, Error: nil}, nil
		}
		s.logger.Error("Failed to remove block rule",
			zap.String("ip", req.IP),
			zap.String("ruleTag", ruleTag),
			zap.Error(err))
		errMsg := err.Error()
		return &UnblockIPResponse{Success: false, Error: &errMsg}, nil
	}

	s.logger.Info("Unblocked IP",
		zap.String("ip", req.IP),
		zap.String("ruleTag", ruleTag))

	return &UnblockIPResponse{Success: true, Error: nil}, nil
}
