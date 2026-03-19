package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/Mikimiya/remnawave-node/internal/middleware"
	"github.com/Mikimiya/remnawave-node/internal/services"
	"github.com/gin-gonic/gin"
)

// Route constants
const (
	RootPath = "/node"
)

// Controller names
const (
	XrayController     = "xray"
	StatsController    = "stats"
	HandlerController  = "handler"
	VisionController   = "vision"
	InternalController = "internal"
)

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	// Root-level routes (no /node prefix, no JWT) — matches Node.js routes served on
	// 127.0.0.1:61001 with PortGuard. In the Go version there is no separate port;
	// mTLS on the main server provides equivalent authentication.
	s.router.POST("/vision/block-ip", s.handleBlockIP)
	s.router.POST("/vision/unblock-ip", s.handleUnblockIP)
	s.router.GET("/internal/get-config", s.handleRawGetConfig)

	// Apply JWT auth middleware to main router
	authMiddleware := middleware.JWTAuth(s.cfg.NodePayload.JWTPublicKey, s.log)

	// Main API routes (with auth)
	node := s.router.Group(RootPath)
	node.Use(authMiddleware)
	{
		// Xray routes
		xray := node.Group("/" + XrayController)
		{
			xray.POST("/start", s.handleXrayStart)
			xray.GET("/stop", s.handleXrayStop)
			xray.GET("/status", s.handleXrayStatus)
			xray.GET("/healthcheck", s.handleNodeHealthCheck)
		}

		// Stats routes
		stats := node.Group("/" + StatsController)
		{
			stats.POST("/get-user-online-status", s.handleGetUserOnlineStatus)
			stats.POST("/get-users-stats", s.handleGetUsersStats)
			stats.GET("/get-system-stats", s.handleGetSystemStats)
			stats.POST("/get-inbound-stats", s.handleGetInboundStats)
			stats.POST("/get-outbound-stats", s.handleGetOutboundStats)
			stats.POST("/get-all-inbounds-stats", s.handleGetAllInboundsStats)
			stats.POST("/get-all-outbounds-stats", s.handleGetAllOutboundsStats)
			stats.POST("/get-combined-stats", s.handleGetCombinedStats)
		}

		// Handler routes
		handler := node.Group("/" + HandlerController)
		{
			handler.POST("/add-user", s.handleAddUser)
			handler.POST("/add-users", s.handleAddUsers)
			handler.POST("/remove-user", s.handleRemoveUser)
			handler.POST("/remove-users", s.handleRemoveUsers)
			handler.POST("/get-inbound-users-count", s.handleGetInboundUsersCount)
			handler.POST("/get-inbound-users", s.handleGetInboundUsers)
		}

		// Vision routes
		vision := node.Group("/" + VisionController)
		{
			vision.POST("/block-ip", s.handleBlockIP)
			vision.POST("/unblock-ip", s.handleUnblockIP)
		}

		// Internal routes
		internal := node.Group("/" + InternalController)
		{
			internal.GET("/get-config", s.handleGetConfig)
		}
	}
}

// === Xray Handlers ===

func (s *Server) handleXrayStart(c *gin.Context) {
	// Read raw body for debugging
	bodyBytes, _ := c.GetRawData()
	s.log.Debugw("Received xray start request", "body", string(bodyBytes))

	// Re-set body for binding
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req services.StartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.log.Errorw("Failed to bind JSON for xray start", "error", err, "body", string(bodyBytes))
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request format: %v", err)})
		return
	}

	// Use context.Background() so xray operations are NOT cancelled when the HTTP
	// connection is closed. This is critical: large batch operations (like AddUsers
	// with hundreds of users) may take longer than the HTTP timeout. If we used
	// c.Request.Context(), a timeout would cancel xray-core operations mid-batch,
	// leaving some users added and others silently dropped.
	resp, err := s.xrayService.Start(context.Background(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// StartResponse already has "response" wrapper, return directly
	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleXrayStop(c *gin.Context) {
	resp, err := s.xrayService.Stop(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleXrayStatus(c *gin.Context) {
	resp, err := s.xrayService.GetStatus(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleNodeHealthCheck(c *gin.Context) {
	resp := s.xrayService.GetNodeHealthCheck(context.Background())
	c.JSON(http.StatusOK, resp)
}

// === Stats Handlers ===

func (s *Server) handleGetUserOnlineStatus(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.statsService.GetUserOnlineStatus(context.Background(), &services.GetUserOnlineStatusRequest{
		Email: req.Username,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetUsersStats(c *gin.Context) {
	var req struct {
		Reset bool `json:"reset"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Default to not resetting
		req.Reset = false
	}

	resp, err := s.statsService.GetAllUsersStats(context.Background(), &services.GetAllUsersStatsRequest{
		Reset: req.Reset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetSystemStats(c *gin.Context) {
	resp, err := s.statsService.GetSystemStats(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetInboundStats(c *gin.Context) {
	var req struct {
		Tag   string `json:"tag"`
		Reset bool   `json:"reset"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.statsService.GetInboundStats(context.Background(), &services.GetInboundStatsRequest{
		Tag:   req.Tag,
		Reset: req.Reset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetOutboundStats(c *gin.Context) {
	var req struct {
		Tag   string `json:"tag"`
		Reset bool   `json:"reset"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.statsService.GetOutboundStats(context.Background(), &services.GetOutboundStatsRequest{
		Tag:   req.Tag,
		Reset: req.Reset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetAllInboundsStats(c *gin.Context) {
	var req struct {
		Reset bool `json:"reset"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reset = false
	}

	resp, err := s.statsService.GetAllInboundsStats(context.Background(), &services.GetAllInboundsStatsRequest{
		Reset: req.Reset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetAllOutboundsStats(c *gin.Context) {
	var req struct {
		Reset bool `json:"reset"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reset = false
	}

	resp, err := s.statsService.GetAllOutboundsStats(context.Background(), &services.GetAllOutboundsStatsRequest{
		Reset: req.Reset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetCombinedStats(c *gin.Context) {
	var req struct {
		Reset bool `json:"reset"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reset = false
	}

	resp, err := s.statsService.GetCombinedStats(context.Background(), &services.GetCombinedStatsRequest{
		Reset: req.Reset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

// === Handler Handlers ===

func (s *Server) handleAddUser(c *gin.Context) {
	var req services.AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.handlerService.AddUser(context.Background(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleAddUsers(c *gin.Context) {
	var req services.AddUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.handlerService.AddUsers(context.Background(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleRemoveUser(c *gin.Context) {
	var req services.RemoveUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.handlerService.RemoveUser(context.Background(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleRemoveUsers(c *gin.Context) {
	var req services.RemoveUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.handlerService.RemoveUsers(context.Background(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetInboundUsersCount(c *gin.Context) {
	var req struct {
		Tag string `json:"tag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.handlerService.GetInboundUsersCount(context.Background(), req.Tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleGetInboundUsers(c *gin.Context) {
	var req struct {
		Tag string `json:"tag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.handlerService.GetInboundUsers(context.Background(), req.Tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

// === Vision Handlers ===

func (s *Server) handleBlockIP(c *gin.Context) {
	var req services.BlockIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.visionService.BlockIP(context.Background(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

func (s *Server) handleUnblockIP(c *gin.Context) {
	var req services.UnblockIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.visionService.UnblockIP(context.Background(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"response": resp,
	})
}

// === Internal Handlers ===

func (s *Server) handleGetConfig(c *gin.Context) {
	resp := s.internalService.GetConfig()
	c.JSON(http.StatusOK, resp)
}

// handleRawGetConfig returns the raw xray config JSON without any wrapper.
// Matches Node.js InternalController on port 61001: returns Record<string, unknown> directly.
// Served at GET /internal/get-config (no /node prefix, no JWT).
func (s *Server) handleRawGetConfig(c *gin.Context) {
	cfg := s.internalService.GetConfig()
	if cfg.Config == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	c.Data(http.StatusOK, "application/json", cfg.Config)
}
