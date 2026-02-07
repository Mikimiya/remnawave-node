package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/Mikimiya/remnawave-node/internal/config"
	"github.com/Mikimiya/remnawave-node/internal/middleware"
	"github.com/Mikimiya/remnawave-node/internal/services"
	"github.com/Mikimiya/remnawave-node/pkg/logger"
	"github.com/Mikimiya/remnawave-node/pkg/xraycore"
	"github.com/gin-gonic/gin"
)

// Server represents the HTTP server
type Server struct {
	cfg        *config.Config
	log        *logger.Logger
	mainServer *http.Server
	router     *gin.Engine

	// Services
	xrayService     *services.XrayService
	handlerService  *services.HandlerService
	statsService    *services.StatsService
	visionService   *services.VisionService
	internalService *services.InternalService

	// Embedded Xray-core
	xrayCore *xraycore.Instance
}

// New creates a new server instance
func New(cfg *config.Config, log *logger.Logger) (*Server, error) {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create main router
	router := gin.New()
	router.Use(middleware.Recovery(log))
	router.Use(middleware.Decompress(log)) // Handle gzip compressed request bodies
	router.Use(middleware.Logger(log))

	// Create embedded Xray-core instance
	xrayCoreInstance := xraycore.New(&xraycore.Config{
		Logger: log.Desugar(),
	})

	// Create services
	// Internal service must be created first as other services depend on it
	internalService := services.NewInternalService(&services.InternalConfig{
		DisableHashCheck: cfg.DisableHashedSetCheck,
	}, log.Desugar())

	xrayService := services.NewXrayService(&services.XrayConfig{
		ConfigDir:             "/var/lib/remnawave-node",
		DisableHashedSetCheck: cfg.DisableHashedSetCheck,
		PortMap:               cfg.PortMap,
	}, xrayCoreInstance, internalService, log.Desugar())

	handlerService := services.NewHandlerService(xrayCoreInstance, internalService, log.Desugar())
	statsService := services.NewStatsService(xrayCoreInstance, log.Desugar())
	visionService := services.NewVisionService(&services.VisionConfig{
		BlockTag: "block",
	}, xrayCoreInstance, log.Desugar())

	srv := &Server{
		cfg:             cfg,
		log:             log,
		router:          router,
		xrayCore:        xrayCoreInstance,
		xrayService:     xrayService,
		handlerService:  handlerService,
		statsService:    statsService,
		visionService:   visionService,
		internalService: internalService,
	}

	// Setup routes
	srv.setupRoutes()

	// Try to restore Xray state from config file
	go func() {
		// Give the server a moment to start
		time.Sleep(1 * time.Second)
		if err := srv.restoreXrayState(); err != nil {
			log.Warn("Failed to restore Xray state", "error", err)
		}
	}()

	return srv, nil
}

// Start starts the main HTTP server with mTLS
func (s *Server) Start() error {
	return s.startMainServer()
}

// startMainServer starts the main HTTPS server with mTLS
func (s *Server) startMainServer() error {
	// Create TLS config
	tlsConfig, err := s.createTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to create TLS config: %w", err)
	}

	addr := fmt.Sprintf(":%d", s.cfg.NodePort)
	s.mainServer = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    65536, // 64KB
	}

	s.log.Infow("Starting main server",
		"port", s.cfg.NodePort,
		"tls", true,
		"mtls", true,
	)

	// Start with TLS
	return s.mainServer.ListenAndServeTLS("", "")
}

// createTLSConfig creates the mTLS configuration
func (s *Server) createTLSConfig() (*tls.Config, error) {
	payload := s.cfg.NodePayload

	// Parse server certificate
	cert, err := tls.X509KeyPair([]byte(payload.NodeCertPem), []byte(payload.NodeKeyPem))
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Create CA cert pool for client verification
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(payload.CACertPem)) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, nil
}

// Shutdown gracefully shuts down the server and Xray-core
func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Stop embedded Xray-core
	if s.xrayCore != nil {
		if err := s.xrayCore.Stop(); err != nil {
			s.log.Errorw("Xray-core shutdown error", "error", err)
		}
	}

	// Shutdown main server
	if s.mainServer != nil {
		if err := s.mainServer.Shutdown(shutdownCtx); err != nil {
			s.log.Errorw("Main server shutdown error", "error", err)
		}
	}

	return nil
}

// restoreXrayState tries to start Xray from existing config file
func (s *Server) restoreXrayState() error {
	configBytes, err := s.xrayService.GetConfig()
	if err != nil {
		return err // file might not exist or other error
	}
	if len(configBytes) == 0 {
		return nil // no config
	}

	s.log.Info("Restoring Xray state from config file...")

	// Create a dummy start request with just the config
	// We don't have the "hashes" here, so we might lose optimization on next sync, but that's fine
	// Or we could try to re-calculate them if we had the logic exposed.
	// For now, just starting it is the priority.
	ctx := context.Background()

	// We need to parse it to pass valid StartRequest if we want to use Service.Start,
	// or we can use xrayCore directly?
	// xrayService.Start expects StartRequest structure which wraps the config.
	// But StartRequest expects separate fields.
	// Actually, xrayService.Start takes xrayConfig map, generates full config, and writes to file.
	// Since we already have the FULL config file (generated), we should probably use xrayCore.Start directly
	// OR, update xrayService to support starting from file.

	// Let's use xrayCore direct start for restoration, but we need to update xrayService state (online status)
	// The Cleanest way is to decode the config and call xrayService.Start, BUT `config.json` is the *Result* of `generateApiConfig`.
	// So if we feed it back to `Start`, it might get wrapped again (policy/stats added again).
	// `xrayService.GetConfig()` reads `config.json` which is the FULL config.

	// Let's modify xrayService to allow "StartFromExistingConfig"
	return s.xrayService.RestoreStart(ctx)
}
