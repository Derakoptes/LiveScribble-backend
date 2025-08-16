package auth

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORSMiddleware() gin.HandlerFunc {
	// Check if all origins should be allowed
	allowAllOrigins := strings.ToLower(os.Getenv("ALLOW_ALL_ORIGINS")) == "true"

	// Parse allowed origins from environment variable
	var allowedOrigins []string //Should be set in json
	originsEnv := os.Getenv("ALLOWED_ORIGINS")
	if originsEnv != "" {
		if err := json.Unmarshal([]byte(originsEnv), &allowedOrigins); err != nil {
			allowedOrigins = []string{} 
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		var originAllowed bool
		if allowAllOrigins {
			c.Header("Access-Control-Allow-Origin", "*")
			originAllowed = true
		} else if len(allowedOrigins) > 0 && slices.Contains(allowedOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			originAllowed = true
		} else if len(allowedOrigins) == 0 && origin == "" {
			originAllowed = true
		}

		if !originAllowed {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version, Sec-WebSocket-Protocol")
		c.Header("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}
