package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	loginVisitors   = make(map[string]*visitor)
	loginVisitorsMu sync.Mutex
)

func init() {
	go cleanupLoginVisitors()
}

func cleanupLoginVisitors() {
	for {
		time.Sleep(3 * time.Minute)
		loginVisitorsMu.Lock()
		for ip, v := range loginVisitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(loginVisitors, ip)
			}
		}
		loginVisitorsMu.Unlock()
	}
}

func getLoginLimiter(ip string) *rate.Limiter {
	loginVisitorsMu.Lock()
	defer loginVisitorsMu.Unlock()

	if v, exists := loginVisitors[ip]; exists {
		v.lastSeen = time.Now()
		return v.limiter
	}

	limiter := rate.NewLimiter(rate.Every(3*time.Second), 3) // 1 request/3sec, burst of 3
	loginVisitors[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

func LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !getLoginLimiter(ip).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts, please try again later"})
			c.Abort()
			return
		}
		c.Next()
	}
}
