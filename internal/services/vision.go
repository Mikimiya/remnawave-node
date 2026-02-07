// Package services provides business logic for IP blocking (Vision)
package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"sync"

	"go.uber.org/zap"

	"github.com/Mikimiya/remnawave-node/pkg/xraycore"
)

// VisionService manages IP blocking via Xray router rules
type VisionService struct {
	mu         sync.RWMutex
	logger     *zap.Logger
	xrayCore   *xraycore.Instance
	blockedIPs map[string]string // IP -> ruleTag (MD5 hash)
	blockTag   string
}

// VisionConfig holds Vision service configuration
type VisionConfig struct {
	BlockTag string // The outbound tag for blocked traffic (e.g., "block" or "BLOCK")
}

// NewVisionService creates a new VisionService
func NewVisionService(cfg *VisionConfig, xrayCore *xraycore.Instance, logger *zap.Logger) *VisionService {
	blockTag := cfg.BlockTag
	if blockTag == "" {
		blockTag = "BLOCK"
	}
	return &VisionService{
		logger:     logger,
		xrayCore:   xrayCore,
		blockedIPs: make(map[string]string),
		blockTag:   blockTag,
	}
}

// getIPHash returns MD5 hash of an IP address (like Node.js object-hash)
func (s *VisionService) getIPHash(ip string) string {
	hash := md5.Sum([]byte(ip))
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

// BlockIP blocks an IP address
func (s *VisionService) BlockIP(ctx context.Context, req *BlockIPRequest) (*BlockIPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already blocked
	if _, exists := s.blockedIPs[req.IP]; exists {
		return &BlockIPResponse{
			Success: true,
			Error:   nil,
		}, nil
	}

	// Generate rule tag from IP hash
	ruleTag := s.getIPHash(req.IP)

	// Add rule via embedded Xray router
	if s.xrayCore != nil && s.xrayCore.IsRunning() {
		if err := s.xrayCore.AddRoutingRule(ctx, ruleTag, req.IP, s.blockTag); err != nil {
			s.logger.Error("Failed to add block rule",
				zap.String("ip", req.IP),
				zap.String("ruleTag", ruleTag),
				zap.Error(err))
			errMsg := err.Error()
			return &BlockIPResponse{Success: false, Error: &errMsg}, nil
		}
	}

	s.blockedIPs[req.IP] = ruleTag
	s.logger.Info("Blocked IP",
		zap.String("ip", req.IP),
		zap.String("ruleTag", ruleTag))

	return &BlockIPResponse{Success: true, Error: nil}, nil
}

// UnblockIPRequest represents a request to unblock an IP
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

// UnblockIP unblocks an IP address
func (s *VisionService) UnblockIP(ctx context.Context, req *UnblockIPRequest) (*UnblockIPResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if not blocked
	ruleTag, exists := s.blockedIPs[req.IP]
	if !exists {
		return &UnblockIPResponse{
			Success: true,
			Error:   nil,
		}, nil
	}

	// Remove rule via embedded Xray router
	if s.xrayCore != nil && s.xrayCore.IsRunning() {
		if err := s.xrayCore.RemoveRoutingRule(ctx, ruleTag); err != nil {
			s.logger.Error("Failed to remove block rule",
				zap.String("ip", req.IP),
				zap.String("ruleTag", ruleTag),
				zap.Error(err))
			errMsg := err.Error()
			return &UnblockIPResponse{Success: false, Error: &errMsg}, nil
		}
	}

	delete(s.blockedIPs, req.IP)
	s.logger.Info("Unblocked IP",
		zap.String("ip", req.IP),
		zap.String("ruleTag", ruleTag))

	return &UnblockIPResponse{Success: true, Error: nil}, nil
}

// GetBlockedIPsResponse represents the list of blocked IPs
type GetBlockedIPsResponse struct {
	IPs []string `json:"ips"`
}

// GetBlockedIPs returns all blocked IPs
func (s *VisionService) GetBlockedIPs() *GetBlockedIPsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ips := make([]string, 0, len(s.blockedIPs))
	for ip := range s.blockedIPs {
		ips = append(ips, ip)
	}

	return &GetBlockedIPsResponse{IPs: ips}
}

// ClearBlockedIPs clears all blocked IPs
func (s *VisionService) ClearBlockedIPs(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for ip, ruleTag := range s.blockedIPs {
		if s.xrayCore != nil && s.xrayCore.IsRunning() {
			if err := s.xrayCore.RemoveRoutingRule(ctx, ruleTag); err != nil {
				s.logger.Warn("Failed to remove block rule during clear",
					zap.String("ip", ip),
					zap.String("ruleTag", ruleTag),
					zap.Error(err))
			}
		}
	}

	s.blockedIPs = make(map[string]string)
	s.logger.Info("Cleared all blocked IPs")

	return nil
}
