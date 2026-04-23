package server

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

//go:embed dist
var distFS embed.FS

// Server is the kite-proxy HTTP server.
type Server struct {
	cfg *Config
}

// New creates a new Server with the provided configuration.
func New(cfg *Config) *Server {
	// Initialise global config with the startup values.
	SetConfig(*cfg)
	return &Server{cfg: cfg}
}

// Run starts the HTTP server and blocks until it exits.
func (s *Server) Run() error {
	r := gin.Default()

	// Serve the embedded frontend.
	distSub, err := fs.Sub(distFS, "dist")
	if err != nil {
		klog.Warningf("Failed to load embedded UI: %v – UI will not be available", err)
	} else {
		r.StaticFS("/ui", http.FS(distSub))
		// Redirect root to the UI.
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/ui/")
		})
	}

	// ── REST API ──────────────────────────────────────────────────────────────
	api := r.Group("/api")

	// Configuration
	api.GET("/config", handleGetConfig)
	api.POST("/config", handleSetConfig)

	// Cluster list (fetched from kite server)
	api.GET("/clusters", handleListClusters)

	// Generate a local kubeconfig for kubectl
	api.GET("/kubeconfig", handleGetKubeconfig)

	// Cache management
	api.DELETE("/cache", handleClearCache)
	api.POST("/cache/:cluster", handlePrewarm)

	// Status / health
	api.GET("/status", handleStatus)
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ── Kubernetes API Proxy ──────────────────────────────────────────────────
	// Requests to /proxy/:cluster/* are forwarded to the real K8s API server
	// using the in-memory kubeconfig fetched from kite.
	r.Any("/proxy/:cluster/*path", HandleProxy)

	addr := ":" + s.cfg.Port
	klog.Infof("kite-proxy listening on %s", addr)
	klog.Infof("UI available at http://localhost:%s/ui/", s.cfg.Port)
	klog.Infof("K8s proxy endpoint: http://localhost:%s/proxy/<cluster-name>/", s.cfg.Port)
	return r.Run(addr)
}
