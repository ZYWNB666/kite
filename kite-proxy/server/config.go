package server

import (
	"sync"
)

// Config holds the runtime configuration for kite-proxy.
type Config struct {
	// Port is the HTTP port kite-proxy listens on.
	Port string

	// KiteURL is the base URL of the upstream kite server, e.g. "https://kite.example.com".
	KiteURL string

	// APIKey is the kite API key used to authenticate with the kite server.
	// It is stored only in memory and never written to disk.
	APIKey string
}

var (
	globalConfigMu sync.RWMutex
	globalConfig   *Config
)

// GetConfig returns a copy of the current global configuration.
func GetConfig() Config {
	globalConfigMu.RLock()
	defer globalConfigMu.RUnlock()
	if globalConfig == nil {
		return Config{}
	}
	return *globalConfig
}

// SetConfig atomically replaces the global configuration.
// After updating, the kubeconfig cache is cleared so that the next proxy
// request will re-fetch credentials from the new kite server.
func SetConfig(cfg Config) {
	globalConfigMu.Lock()
	globalConfig = &cfg
	globalConfigMu.Unlock()

	// Invalidate the kubeconfig cache because credentials may have changed.
	globalCache.Clear()
}
