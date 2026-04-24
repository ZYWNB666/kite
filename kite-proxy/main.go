package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zxh326/kite-proxy/server"
	"k8s.io/klog/v2"
)

func main() {
	var (
		port    string
		kiteURL string
		apiKey  string
	)

	flag.StringVar(&port, "port", envOrDefault("PORT", "8090"), "port to listen on")
	flag.StringVar(&kiteURL, "kite-url", os.Getenv("KITE_URL"), "kite server URL (e.g. https://kite.example.com)")
	flag.StringVar(&apiKey, "api-key", os.Getenv("KITE_API_KEY"), "kite API key for authentication")
	flag.Parse()

	klog.Infof("Starting kite-proxy on port %s", port)
	if kiteURL != "" {
		klog.Infof("Kite server URL: %s", kiteURL)
	}

	cfg := &server.Config{
		Port:    port,
		KiteURL: kiteURL,
		APIKey:  apiKey,
	}

	srv := server.New(cfg)
	if err := srv.Run(); err != nil {
		klog.Fatalf("Failed to start kite-proxy: %v", err)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func init() {
	// Print banner
	fmt.Println(`
 _    _ _         ______
| |  (_) |       (_____ \
| |  _| |_ ___   _____) )_ __  ___  _  _  _   _
| | | |  _/ _ \ |  ____/ '__/ / _ \| |/ || | | |
| |___| | ||  __/| |    | |  | (_) |  /  | |_| |
|_______)_|\___/ |_|    |_|   \___/|_|    \__  |
                                          (____/
kite-proxy - Kubernetes API Forwarding Proxy
`)
}
