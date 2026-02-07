// Package xraycore provides an embedded Xray-core instance
// This replaces the external Xray process + gRPC approach with a direct Go integration
package xraycore

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	// Xray-core imports
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"

	// Services for direct API access
	routerConfig "github.com/xtls/xray-core/app/router"
	appstats "github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/protocol"
	cserial "github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/features/stats"
	"github.com/xtls/xray-core/proxy"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
)

// Instance represents an embedded Xray-core instance
type Instance struct {
	mu        sync.RWMutex
	logger    *zap.Logger
	instance  *core.Instance
	config    []byte // Current config JSON
	version   string
	running   bool
	startTime time.Time
}

// Config for creating a new Instance
type Config struct {
	Logger *zap.Logger
}

// New creates a new embedded Xray-core instance manager
func New(cfg *Config) *Instance {
	return &Instance{
		logger:  cfg.Logger,
		version: core.Version(),
	}
}

// Version returns the Xray-core version
func (x *Instance) Version() string {
	return x.version
}

// IsRunning returns true if Xray is running
func (x *Instance) IsRunning() bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.running && x.instance != nil
}

// Start starts Xray with the given JSON configuration
func (x *Instance) Start(ctx context.Context, configJSON []byte) error {
	x.mu.Lock()
	defer x.mu.Unlock()

	// Stop existing instance if running
	if x.instance != nil {
		if err := x.instance.Close(); err != nil {
			x.logger.Warn("Error closing existing Xray instance", zap.Error(err))
		}
		x.instance = nil
		x.running = false
	}

	x.logger.Info("Starting Xray-core", zap.String("version", x.version))

	// Parse JSON config
	jsonConfig, err := serial.DecodeJSONConfig(bytes.NewReader(configJSON))
	if err != nil {
		return fmt.Errorf("failed to parse Xray config: %w", err)
	}

	// Convert to protobuf config
	pbConfig, err := jsonConfig.Build()
	if err != nil {
		return fmt.Errorf("failed to build Xray config: %w", err)
	}

	// Create and start instance
	instance, err := core.New(pbConfig)
	if err != nil {
		return fmt.Errorf("failed to create Xray instance: %w", err)
	}

	if err := instance.Start(); err != nil {
		return fmt.Errorf("failed to start Xray instance: %w", err)
	}

	x.instance = instance
	x.config = configJSON
	x.running = true
	x.startTime = time.Now()

	x.logger.Info("Xray-core started successfully")
	return nil
}

// Stop stops the Xray instance
func (x *Instance) Stop() error {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.instance == nil {
		return nil
	}

	x.logger.Info("Stopping Xray-core")

	if err := x.instance.Close(); err != nil {
		return fmt.Errorf("failed to stop Xray instance: %w", err)
	}

	x.instance = nil
	x.running = false
	x.config = nil

	x.logger.Info("Xray-core stopped")
	return nil
}

// Restart restarts Xray with new configuration
func (x *Instance) Restart(ctx context.Context, configJSON []byte) error {
	return x.Start(ctx, configJSON)
}

// ============= Handler Service (User Management) =============

// getInboundProxy gets the inbound proxy from a handler
func (x *Instance) getInboundProxy(ctx context.Context, inboundTag string) (proxy.Inbound, error) {
	handler := x.instance.GetFeature(inbound.ManagerType())
	if handler == nil {
		return nil, fmt.Errorf("inbound handler manager not found")
	}

	handlerManager := handler.(inbound.Manager)
	inboundHandler, err := handlerManager.GetHandler(ctx, inboundTag)
	if err != nil {
		return nil, fmt.Errorf("failed to get inbound handler: %w", err)
	}

	gi, ok := inboundHandler.(proxy.GetInbound)
	if !ok {
		return nil, fmt.Errorf("handler does not support GetInbound")
	}

	return gi.GetInbound(), nil
}

// AddUser adds a user to an inbound using MemoryUser
func (x *Instance) AddUser(ctx context.Context, inboundTag string, user *protocol.MemoryUser) error {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return fmt.Errorf("Xray instance not running")
	}

	inboundProxy, err := x.getInboundProxy(ctx, inboundTag)
	if err != nil {
		return err
	}

	um, ok := inboundProxy.(proxy.UserManager)
	if !ok {
		return fmt.Errorf("inbound does not support user management")
	}

	return um.AddUser(ctx, user)
}

// RemoveUser removes a user from an inbound
func (x *Instance) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return fmt.Errorf("Xray instance not running")
	}

	inboundProxy, err := x.getInboundProxy(ctx, inboundTag)
	if err != nil {
		return err
	}

	um, ok := inboundProxy.(proxy.UserManager)
	if !ok {
		return fmt.Errorf("inbound does not support user management")
	}

	return um.RemoveUser(ctx, email)
}

// ============= Stats Service =============

// GetStats gets stats by pattern
func (x *Instance) GetStats(ctx context.Context, pattern string, reset bool) (map[string]int64, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return nil, fmt.Errorf("Xray instance not running")
	}

	statsFeature := x.instance.GetFeature(stats.ManagerType())
	if statsFeature == nil {
		return nil, fmt.Errorf("stats feature not found")
	}

	// Try to cast to app/stats.Manager which has VisitCounters
	manager, ok := statsFeature.(*appstats.Manager)
	if !ok {
		return nil, fmt.Errorf("stats manager does not support VisitCounters")
	}

	result := make(map[string]int64)

	manager.VisitCounters(func(name string, counter stats.Counter) bool {
		if pattern == "" || matchPattern(name, pattern) {
			if reset {
				result[name] = counter.Set(0)
			} else {
				result[name] = counter.Value()
			}
		}
		return true
	})

	return result, nil
}

// GetSystemStats returns Xray system statistics
func (x *Instance) GetSystemStats(ctx context.Context) (*SystemStats, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return nil, fmt.Errorf("Xray instance not running")
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := uint32(0)
	if !x.startTime.IsZero() {
		uptime = uint32(time.Since(x.startTime).Seconds())
	}

	return &SystemStats{
		NumGoroutine: uint32(runtime.NumGoroutine()),
		NumGC:        memStats.NumGC,
		Alloc:        memStats.Alloc,
		TotalAlloc:   memStats.TotalAlloc,
		Sys:          memStats.Sys,
		Mallocs:      memStats.Mallocs,
		Frees:        memStats.Frees,
		LiveObjects:  memStats.Mallocs - memStats.Frees,
		Uptime:       uptime,
	}, nil
}

// SystemStats represents system statistics
type SystemStats struct {
	NumGoroutine uint32 `json:"numGoroutine"`
	NumGC        uint32 `json:"numGC"`
	Alloc        uint64 `json:"alloc"`
	TotalAlloc   uint64 `json:"totalAlloc"`
	Sys          uint64 `json:"sys"`
	Mallocs      uint64 `json:"mallocs"`
	Frees        uint64 `json:"frees"`
	LiveObjects  uint64 `json:"liveObjects"`
	Uptime       uint32 `json:"uptime"`
}

// GetUserOnlineStatus checks if a user is online (has active connections)
func (x *Instance) GetUserOnlineStatus(ctx context.Context, email string) (bool, error) {
	allStats, err := x.GetStats(ctx, fmt.Sprintf("user>>>%s>>>", email), false)
	if err != nil {
		return false, err
	}

	// User is online if there's any traffic
	for _, value := range allStats {
		if value > 0 {
			return true, nil
		}
	}
	return false, nil
}

// ============= Router Service (IP Blocking) =============

// AddRoutingRule adds a routing rule to block an IP
func (x *Instance) AddRoutingRule(ctx context.Context, ruleTag string, targetIP string, outboundTag string) error {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return fmt.Errorf("Xray instance not running")
	}

	routerFeature := x.instance.GetFeature(routing.RouterType())
	if routerFeature == nil {
		return fmt.Errorf("router feature not found")
	}

	r, ok := routerFeature.(routing.Router)
	if !ok {
		return fmt.Errorf("feature is not a Router")
	}

	// Create routing rule using xray-core's RoutingRule proto message
	// Import the router package and use its RoutingRule type
	rule := &routerConfig.RoutingRule{
		RuleTag: ruleTag,
		TargetTag: &routerConfig.RoutingRule_Tag{
			Tag: outboundTag,
		},
		SourceGeoip: []*routerConfig.GeoIP{
			{
				Cidr: []*routerConfig.CIDR{
					parseCIDR(targetIP),
				},
			},
		},
	}

	ruleMsg := cserial.ToTypedMessage(rule)
	return r.AddRule(ruleMsg, false)
}

// parseCIDR parses an IP or CIDR string into a CIDR proto message
func parseCIDR(ip string) *routerConfig.CIDR {
	// Handle CIDR notation
	parts := strings.Split(ip, "/")
	ipAddr := parts[0]
	prefix := uint32(32) // Default for IPv4
	if len(parts) == 2 {
		if p, err := fmt.Sscanf(parts[1], "%d", &prefix); err == nil && p > 0 {
			// prefix parsed
		}
	}

	// Parse IP address
	ipBytes := parseIPToBytes(ipAddr)
	if len(ipBytes) == 16 && prefix == 32 {
		prefix = 128 // IPv6 default
	}

	return &routerConfig.CIDR{
		Ip:     ipBytes,
		Prefix: prefix,
	}
}

// parseIPToBytes converts IP string to bytes
func parseIPToBytes(ip string) []byte {
	// Try IPv4 first
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		result := make([]byte, 4)
		for i, p := range parts {
			var v int
			fmt.Sscanf(p, "%d", &v)
			result[i] = byte(v)
		}
		return result
	}
	// For IPv6, we'd need more complex parsing
	// For now, return empty (will need proper implementation if IPv6 is needed)
	return nil
}

// RemoveRoutingRule removes a routing rule by tag
func (x *Instance) RemoveRoutingRule(ctx context.Context, ruleTag string) error {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return fmt.Errorf("Xray instance not running")
	}

	routerFeature := x.instance.GetFeature(routing.RouterType())
	if routerFeature == nil {
		return fmt.Errorf("router feature not found")
	}

	r, ok := routerFeature.(routing.Router)
	if !ok {
		return fmt.Errorf("feature is not a Router")
	}

	return r.RemoveRule(ruleTag)
}

// ============= Helper Functions =============

func matchPattern(name, pattern string) bool {
	return strings.HasPrefix(name, pattern)
}

// CreateVlessUser creates a VLESS MemoryUser account
func CreateVlessUser(email, uuid, flow string, level uint32) (*protocol.MemoryUser, error) {
	account := &vless.Account{
		Id:   uuid,
		Flow: flow,
	}
	memoryAccount, err := account.AsAccount()
	if err != nil {
		return nil, err
	}
	return &protocol.MemoryUser{
		Email:   email,
		Level:   level,
		Account: memoryAccount,
	}, nil
}

// CreateTrojanUser creates a Trojan MemoryUser account
func CreateTrojanUser(email, password string, level uint32) (*protocol.MemoryUser, error) {
	account := &trojan.Account{
		Password: password,
	}
	memoryAccount, err := account.AsAccount()
	if err != nil {
		return nil, err
	}
	return &protocol.MemoryUser{
		Email:   email,
		Level:   level,
		Account: memoryAccount,
	}, nil
}

// CreateShadowsocksUser creates a Shadowsocks MemoryUser account
func CreateShadowsocksUser(email, password string, cipherType shadowsocks.CipherType, level uint32) (*protocol.MemoryUser, error) {
	account := &shadowsocks.Account{
		Password:   password,
		CipherType: cipherType,
	}
	memoryAccount, err := account.AsAccount()
	if err != nil {
		return nil, err
	}
	return &protocol.MemoryUser{
		Email:   email,
		Level:   level,
		Account: memoryAccount,
	}, nil
}

// CipherTypeFromInt converts int to shadowsocks.CipherType
func CipherTypeFromInt(t int) shadowsocks.CipherType {
	switch t {
	case 5:
		return shadowsocks.CipherType_AES_128_GCM
	case 6:
		return shadowsocks.CipherType_AES_256_GCM
	case 7:
		return shadowsocks.CipherType_CHACHA20_POLY1305
	case 8:
		return shadowsocks.CipherType_XCHACHA20_POLY1305
	case 9:
		return shadowsocks.CipherType_NONE
	default:
		return shadowsocks.CipherType_AES_256_GCM
	}
}

// UserStats represents user traffic statistics
type UserStats struct {
	Email    string `json:"email"`
	Uplink   int64  `json:"uplink"`
	Downlink int64  `json:"downlink"`
}

// GetUserStats gets traffic statistics for a specific user
func (x *Instance) GetUserStats(ctx context.Context, email string, reset bool) (*UserStats, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return nil, fmt.Errorf("Xray instance not running")
	}

	statsFeature := x.instance.GetFeature(stats.ManagerType())
	if statsFeature == nil {
		return nil, fmt.Errorf("stats feature not found")
	}

	manager := statsFeature.(stats.Manager)
	result := &UserStats{Email: email}

	// Get uplink stats
	uplinkName := fmt.Sprintf("user>>>%s>>>traffic>>>uplink", email)
	if counter := manager.GetCounter(uplinkName); counter != nil {
		if reset {
			result.Uplink = counter.Set(0)
		} else {
			result.Uplink = counter.Value()
		}
	}

	// Get downlink stats
	downlinkName := fmt.Sprintf("user>>>%s>>>traffic>>>downlink", email)
	if counter := manager.GetCounter(downlinkName); counter != nil {
		if reset {
			result.Downlink = counter.Set(0)
		} else {
			result.Downlink = counter.Value()
		}
	}

	return result, nil
}

// GetAllUserStats gets traffic statistics for all users
func (x *Instance) GetAllUserStats(ctx context.Context, reset bool) ([]*UserStats, error) {
	x.mu.RLock()
	defer x.mu.RUnlock()

	if x.instance == nil {
		return nil, fmt.Errorf("Xray instance not running")
	}

	statsFeature := x.instance.GetFeature(stats.ManagerType())
	if statsFeature == nil {
		return nil, fmt.Errorf("stats feature not found")
	}

	// Try to cast to app/stats.Manager which has VisitCounters
	manager, ok := statsFeature.(*appstats.Manager)
	if !ok {
		return nil, fmt.Errorf("stats manager does not support VisitCounters")
	}

	userTraffic := make(map[string]*UserStats)

	manager.VisitCounters(func(name string, counter stats.Counter) bool {
		// Parse counter name: user>>>email>>>traffic>>>uplink/downlink
		if !strings.HasPrefix(name, "user>>>") {
			return true
		}
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[2] != "traffic" {
			return true
		}

		email := parts[1]
		direction := parts[3]

		if _, exists := userTraffic[email]; !exists {
			userTraffic[email] = &UserStats{Email: email}
		}

		var value int64
		if reset {
			value = counter.Set(0)
		} else {
			value = counter.Value()
		}

		if direction == "uplink" {
			userTraffic[email].Uplink = value
		} else if direction == "downlink" {
			userTraffic[email].Downlink = value
		}

		return true
	})

	result := make([]*UserStats, 0, len(userTraffic))
	for _, s := range userTraffic {
		result = append(result, s)
	}

	return result, nil
}

// GetConfig returns the current configuration JSON
func (x *Instance) GetConfig() []byte {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.config
}

// Uptime returns seconds since start
func (x *Instance) Uptime() int64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	if x.startTime.IsZero() {
		return 0
	}
	return int64(time.Since(x.startTime).Seconds())
}
