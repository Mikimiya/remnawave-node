// Package services provides business logic services
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/Mikimiya/remnawave-node/pkg/xraycore"
)

// ErrXrayAlreadyProcessing indicates Xray is already being started/restarted
var ErrXrayAlreadyProcessing = errors.New("Xray is already being processed")

// XrayService manages Xray-core process and configuration
type XrayService struct {
	mu           sync.RWMutex
	logger       *zap.Logger
	xrayCore     *xraycore.Instance
	internal     *InternalService
	configDir    string
	isConfigured bool

	// Concurrency protection
	isStartProcessing atomic.Bool

	// Online status tracking
	isXrayOnline bool

	// Disable hash check (skip restart optimization)
	disableHashedSetCheck bool

	// Port mapping for NAT machines
	portMap map[int]int
}

// XrayConfig holds Xray service configuration
type XrayConfig struct {
	ConfigDir             string
	DisableHashedSetCheck bool        // If true, skip hash-based restart optimization
	PortMap               map[int]int // Port mapping for NAT machines
}

// NewXrayService creates a new XrayService
func NewXrayService(cfg *XrayConfig, xrayCore *xraycore.Instance, internal *InternalService, logger *zap.Logger) *XrayService {
	return &XrayService{
		logger:                logger,
		xrayCore:              xrayCore,
		internal:              internal,
		configDir:             cfg.ConfigDir,
		isXrayOnline:          false,
		disableHashedSetCheck: cfg.DisableHashedSetCheck,
		portMap:               cfg.PortMap,
	}
}

// GetXrayCore returns the underlying Xray-core instance
func (s *XrayService) GetXrayCore() *xraycore.Instance {
	return s.xrayCore
}

// checkXrayHealth checks if Xray is responding
func (s *XrayService) checkXrayHealth(ctx context.Context) bool {
	if s.xrayCore == nil || !s.xrayCore.IsRunning() {
		return false
	}

	// Try to get system stats to verify it's working
	_, err := s.xrayCore.GetSystemStats(ctx)
	return err == nil
}

// XrayConfigData represents the Xray configuration file structure
type XrayConfigData struct {
	Log       interface{}   `json:"log,omitempty"`
	API       interface{}   `json:"api,omitempty"`
	Inbounds  []interface{} `json:"inbounds,omitempty"`
	Outbounds []interface{} `json:"outbounds,omitempty"`
	Routing   interface{}   `json:"routing,omitempty"`
	Stats     interface{}   `json:"stats,omitempty"`
	Policy    interface{}   `json:"policy,omitempty"`
}

// Default policy configuration (matches Node.js XRAY_DEFAULT_POLICY_MODEL)
var defaultPolicyConfig = map[string]interface{}{
	"levels": map[string]interface{}{
		"0": map[string]interface{}{
			"statsUserUplink":   true,
			"statsUserDownlink": true,
			"statsUserOnline":   true,
		},
	},
	"system": map[string]interface{}{
		"statsInboundDownlink":  true,
		"statsInboundUplink":    true,
		"statsOutboundDownlink": true,
		"statsOutboundUplink":   true,
	},
}

// generateApiConfig adds Stats and Policy configurations to the Xray config
// Note: We don't need API/gRPC config since we're using embedded Xray-core
func generateApiConfig(config map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all existing config
	for k, v := range config {
		result[k] = v
	}

	// Add stats configuration (empty object)
	result["stats"] = map[string]interface{}{}

	// Build and add policy configuration (required for user stats)
	result["policy"] = defaultPolicyConfig

	// Only enable debug logging if NODE_ENV is development
	logLevel := "warning"
	if os.Getenv("NODE_ENV") == "development" {
		logLevel = "debug"
	}

	result["log"] = map[string]interface{}{
		"loglevel": logLevel,
		"access":   "",
		"error":    "",
	}

	return result
}

// StartRequestInternals represents the internals part of start request (Node.js format)
type StartRequestInternals struct {
	ForceRestart bool           `json:"forceRestart"`
	Hashes       *InboundHashes `json:"hashes"`
}

// StartRequest represents a request to start Xray (Node.js compatible format)
// Format: { internals: { forceRestart, hashes }, xrayConfig: {...} }
type StartRequest struct {
	Internals  StartRequestInternals  `json:"internals"`
	XrayConfig map[string]interface{} `json:"xrayConfig"`
}

// SystemInformation represents system info in response
type SystemInformation struct {
	CPUCores    int    `json:"cpuCores"`
	CPUModel    string `json:"cpuModel"`
	MemoryTotal string `json:"memoryTotal"`
}

// NodeInformation represents node version info
type NodeInformation struct {
	Version string `json:"version"`
}

// StartResponseData represents the response data for start request (Node.js format)
type StartResponseData struct {
	IsStarted         bool               `json:"isStarted"`
	Version           *string            `json:"version"`
	Error             *string            `json:"error"`
	SystemInformation *SystemInformation `json:"systemInformation"`
	NodeInformation   NodeInformation    `json:"nodeInformation"`
}

// StartResponse represents a response to start request (Node.js compatible format)
type StartResponse struct {
	Response StartResponseData `json:"response"`
}

// NodeHealthCheckResponseData represents the response data for health check (Node.js format)
type NodeHealthCheckResponseData struct {
	IsAlive                  bool    `json:"isAlive"`
	XrayInternalStatusCached bool    `json:"xrayInternalStatusCached"`
	XrayVersion              *string `json:"xrayVersion"`
	NodeVersion              string  `json:"nodeVersion"`
}

// NodeHealthCheckResponse represents a response to health check request
type NodeHealthCheckResponse struct {
	Response NodeHealthCheckResponseData `json:"response"`
}

// nodeVersion is the current node version
var nodeVersion = "1.0.0"

// SetNodeVersion sets the node version (called during initialization)
func SetNodeVersion(version string) {
	nodeVersion = version
}

// Start starts the Xray process with the given configuration
func (s *XrayService) Start(ctx context.Context, req *StartRequest) (*StartResponse, error) {
	startTime := time.Now()

	// Helper to create error response
	errorResponse := func(errMsg string) *StartResponse {
		return &StartResponse{
			Response: StartResponseData{
				IsStarted:         false,
				Version:           nil,
				Error:             &errMsg,
				SystemInformation: nil,
				NodeInformation:   NodeInformation{Version: nodeVersion},
			},
		}
	}

	// Helper to create success response
	successResponse := func(version string) *StartResponse {
		return &StartResponse{
			Response: StartResponseData{
				IsStarted:         true,
				Version:           &version,
				Error:             nil,
				SystemInformation: s.getSystemInformation(),
				NodeInformation:   NodeInformation{Version: nodeVersion},
			},
		}
	}

	// Check for concurrent processing
	if !s.isStartProcessing.CompareAndSwap(false, true) {
		errMsg := "Request already in progress"
		return errorResponse(errMsg), nil
	}
	defer s.isStartProcessing.Store(false)

	s.mu.Lock()
	defer s.mu.Unlock()

	// If Xray is online, hashed set check is enabled, and not force restart, check if restart is needed
	if s.isXrayOnline && !s.disableHashedSetCheck && !req.Internals.ForceRestart && req.Internals.Hashes != nil && s.internal != nil {
		// First verify Xray is actually healthy
		if s.checkXrayHealth(ctx) {
			// Check if config changed
			needRestart := s.internal.IsNeedRestartCore(req.Internals.Hashes)
			if !needRestart {
				s.logger.Info("No changes detected, skipping restart",
					zap.Duration("checkTime", time.Since(startTime)))
				version := s.GetVersion()
				return &StartResponse{
					Response: StartResponseData{
						IsStarted:         true,
						Version:           &version,
						Error:             nil,
						SystemInformation: s.getSystemInformation(),
						NodeInformation:   NodeInformation{Version: nodeVersion},
					},
				}, nil
			}
		} else {
			// Health check failed, need to restart
			s.isXrayOnline = false
			s.logger.Warn("Xray Core health check failed, restarting...")
		}
	}

	// Force restart requested
	if req.Internals.ForceRestart {
		s.logger.Warn("Force restart requested")
	}

	// Check if restart is needed (hash comparison) - for first start
	if !req.Internals.ForceRestart && !s.isXrayOnline && req.Internals.Hashes != nil && s.internal != nil {
		needRestart := s.internal.IsNeedRestartCore(req.Internals.Hashes)
		if !needRestart {
			s.logger.Info("No changes detected, skipping restart",
				zap.Duration("checkTime", time.Since(startTime)))
			version := s.GetVersion()
			return &StartResponse{
				Response: StartResponseData{
					IsStarted:         true,
					Version:           &version,
					Error:             nil,
					SystemInformation: s.getSystemInformation(),
					NodeInformation:   NodeInformation{Version: nodeVersion},
				},
			}, nil
		}
	}

	// Generate full config with Stats and Policy
	fullConfig := generateApiConfig(req.XrayConfig)

	// Apply port mapping for NAT machines
	if len(s.portMap) > 0 {
		s.logger.Info("Applying port mapping to Xray config", zap.Int("mappings", len(s.portMap)))
		fullConfig = ApplyPortMap(fullConfig, s.portMap, s.logger)
	}

	// Convert fullConfig to JSON bytes
	configBytes, err := json.Marshal(fullConfig)
	if err != nil {
		return errorResponse(fmt.Sprintf("failed to marshal config: %v", err)), nil
	}

	// Write config to file for reference
	configPath := filepath.Join(s.configDir, "config.json")
	if err := os.MkdirAll(s.configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	s.logger.Info("Written Xray config", zap.String("path", configPath))

	// Extract users from config for tracking (pass hashes to store them)
	if s.internal != nil {
		if err := s.internal.ExtractUsersFromConfig(configBytes, req.Internals.Hashes); err != nil {
			s.logger.Warn("Failed to extract users from config", zap.Error(err))
		}
	}

	// Start the embedded Xray-core
	if err := s.xrayCore.Start(ctx, configBytes); err != nil {
		s.isXrayOnline = false
		s.logger.Error("Failed to start Xray",
			zap.Error(err),
			zap.Duration("elapsed", time.Since(startTime)))
		return errorResponse(err.Error()), nil
	}

	// Verify Xray is actually responding
	isStarted := s.checkXrayHealth(ctx)
	if !isStarted {
		s.isXrayOnline = false
		s.logger.Error("Xray failed to start - health check failed",
			zap.Duration("elapsed", time.Since(startTime)))
		return errorResponse("Xray started but health check failed"), nil
	}

	// Get version after start
	version := s.GetVersion()

	s.isConfigured = true
	s.isXrayOnline = true
	s.logger.Info("Xray started successfully",
		zap.String("version", version),
		zap.Duration("elapsed", time.Since(startTime)))

	return successResponse(version), nil
}

// getSystemInformation returns system information for the response
func (s *XrayService) getSystemInformation() *SystemInformation {
	return &SystemInformation{
		CPUCores:    getCPUCores(),
		CPUModel:    getCPUModel(),
		MemoryTotal: getMemoryTotal(),
	}
}

// StopResponse represents a response to stop request (Node.js compatible)
type StopResponse struct {
	IsStopped bool `json:"isStopped"`
}

// Stop stops the Xray process
func (s *XrayService) Stop(ctx context.Context) (*StopResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.xrayCore.Stop(); err != nil {
		s.logger.Error("Failed to stop Xray", zap.Error(err))
		return &StopResponse{IsStopped: false}, nil
	}

	s.isConfigured = false
	s.isXrayOnline = false

	// Cleanup internal state
	if s.internal != nil {
		s.internal.Cleanup()
	}

	return &StopResponse{IsStopped: true}, nil
}

// RestartRequest represents a request to restart Xray
type RestartRequest struct {
	Config       json.RawMessage `json:"config,omitempty"`
	Hashes       *InboundHashes  `json:"hashes,omitempty"`
	ForceRestart bool            `json:"forceRestart,omitempty"`
}

// RestartResponse represents a response to restart request
type RestartResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
}

// Restart restarts the Xray process, optionally with new config
func (s *XrayService) Restart(ctx context.Context, req *RestartRequest) (*RestartResponse, error) {
	startTime := time.Now()

	// Check for concurrent processing
	if !s.isStartProcessing.CompareAndSwap(false, true) {
		return &RestartResponse{
			Success: false,
			Message: "Request already in progress",
			Version: s.GetVersion(),
		}, nil
	}
	defer s.isStartProcessing.Store(false)

	s.mu.Lock()
	defer s.mu.Unlock()

	// If Xray is online and not force restart, check if restart is needed
	if s.isXrayOnline && !req.ForceRestart && req.Hashes != nil && s.internal != nil {
		if s.checkXrayHealth(ctx) {
			needRestart := s.internal.IsNeedRestartCore(req.Hashes)
			if !needRestart {
				s.logger.Info("No changes detected, skipping restart",
					zap.Duration("checkTime", time.Since(startTime)))
				return &RestartResponse{
					Success: true,
					Message: "No changes detected, restart skipped",
					Skipped: true,
					Version: s.GetVersion(),
				}, nil
			}
		} else {
			s.isXrayOnline = false
			s.logger.Warn("Xray Core health check failed, restarting...")
		}
	}

	if req.ForceRestart {
		s.logger.Warn("Force restart requested")
	}

	// If new config provided, write it and use it
	configBytes := req.Config
	if len(configBytes) > 0 {
		// Apply port mapping for NAT machines
		if len(s.portMap) > 0 {
			s.logger.Info("Applying port mapping to Xray config (restart)", zap.Int("mappings", len(s.portMap)))
			mapped, err := ApplyPortMapToBytes(configBytes, s.portMap, s.logger)
			if err != nil {
				s.logger.Warn("Failed to apply port mapping on restart", zap.Error(err))
			} else {
				configBytes = mapped
			}
		}

		configPath := filepath.Join(s.configDir, "config.json")
		if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
			return nil, fmt.Errorf("failed to write config file: %w", err)
		}
		s.logger.Info("Updated Xray config", zap.String("path", configPath))

		// Extract users from config for tracking (pass hashes to store them)
		if s.internal != nil {
			if err := s.internal.ExtractUsersFromConfig(configBytes, req.Hashes); err != nil {
				s.logger.Warn("Failed to extract users from config", zap.Error(err))
			}
		}
	} else {
		// Use existing config
		configBytes = s.xrayCore.GetConfig()
	}

	// Restart the embedded Xray-core
	if err := s.xrayCore.Restart(ctx, configBytes); err != nil {
		s.isXrayOnline = false
		return &RestartResponse{
			Success: false,
			Message: err.Error(),
			Version: s.GetVersion(),
		}, nil
	}

	// Verify health
	isStarted := s.checkXrayHealth(ctx)
	if !isStarted {
		s.isXrayOnline = false
		s.logger.Error("Xray restart failed - health check failed")
		return &RestartResponse{
			Success: false,
			Message: "Xray restarted but health check failed",
			Version: s.GetVersion(),
		}, nil
	}

	version := s.GetVersion()

	s.isConfigured = true
	s.isXrayOnline = true
	s.logger.Info("Xray restarted successfully",
		zap.String("version", version),
		zap.Duration("elapsed", time.Since(startTime)))

	return &RestartResponse{
		Success: true,
		Message: "Xray restarted successfully",
		Version: version,
	}, nil
}

// GetStatusResponse represents the status of Xray (Node.js compatible)
type GetStatusResponse struct {
	IsRunning bool    `json:"isRunning"`
	Version   *string `json:"version"`
}

// GetStatus returns the current status and version of Xray
func (s *XrayService) GetStatus(ctx context.Context) (*GetStatusResponse, error) {
	isRunning := s.xrayCore.IsRunning()

	var version *string
	if isRunning {
		v := s.GetVersion()
		if v != "" && v != "unknown" {
			version = &v
		}
	}

	return &GetStatusResponse{
		IsRunning: isRunning,
		Version:   version,
	}, nil
}

// RestoreStart attempts to start Xray from the existing config file on disk
func (s *XrayService) RestoreStart(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.xrayCore.IsRunning() {
		return nil
	}

	configBytes, err := s.GetConfig()
	if err != nil {
		return err
	}
	if len(configBytes) == 0 {
		return fmt.Errorf("no config file found")
	}

	s.logger.Info("Attempting to restore Xray from local config...")

	// Extract users from config to restore internal state
	if s.internal != nil {
		// We pass nil for hashes as we can't recover them easily,
		// but at least user mapping will be restored for removal login
		// Note: passing nil hashes might reset tracking, so be careful.
		// However, ExtractUsersFromConfig clears existing state anyway.
		if err := s.internal.ExtractUsersFromConfig(configBytes, nil); err != nil {
			s.logger.Warn("Failed to restore users from config", zap.Error(err))
		}
	}

	// Start Xray
	if err := s.xrayCore.Start(ctx, configBytes); err != nil {
		s.isXrayOnline = false
		return fmt.Errorf("restore failed: %w", err)
	}

	// Verify health
	if !s.checkXrayHealth(ctx) {
		s.isXrayOnline = false
		return fmt.Errorf("restored Xray health check failed")
	}

	version := s.GetVersion()
	s.isConfigured = true
	s.isXrayOnline = true

	s.logger.Info("Xray restored successfully from local config",
		zap.String("version", version))

	return nil
}

// IsRunning returns true if Xray is running
func (s *XrayService) IsRunning(ctx context.Context) bool {
	return s.xrayCore.IsRunning()
}

// GetNodeHealthCheck returns the node health check response (Node.js compatible)
func (s *XrayService) GetNodeHealthCheck(ctx context.Context) *NodeHealthCheckResponse {
	s.mu.RLock()
	isXrayOnline := s.isXrayOnline
	s.mu.RUnlock()

	var xrayVersion *string
	if v := s.GetVersion(); v != "" && v != "unknown" {
		xrayVersion = &v
	}

	return &NodeHealthCheckResponse{
		Response: NodeHealthCheckResponseData{
			IsAlive:                  true,
			XrayInternalStatusCached: isXrayOnline,
			XrayVersion:              xrayVersion,
			NodeVersion:              nodeVersion,
		},
	}
}

// IsConfigured returns true if Xray has been configured
func (s *XrayService) IsConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isConfigured
}

// GetConfig returns the current Xray configuration
func (s *XrayService) GetConfig() (json.RawMessage, error) {
	configPath := filepath.Join(s.configDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	return data, nil
}

// GetVersion returns the Xray version from embedded core
func (s *XrayService) GetVersion() string {
	return s.xrayCore.Version()
}

// System information helper functions
func getCPUCores() int {
	return runtime.NumCPU()
}

func getCPUModel() string {
	// Try to read CPU model from /proc/cpuinfo on Linux
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "Unknown"
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "Unknown"
}

func getMemoryTotal() string {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Try to get system total memory from /proc/meminfo on Linux
	data, err := os.ReadFile("/proc/meminfo")
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					return parts[1] + " kB"
				}
			}
		}
	}

	// Fallback to Go runtime stats
	return fmt.Sprintf("%d MB", memStats.Sys/1024/1024)
}
