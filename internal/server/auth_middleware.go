package server

import (
	"github.com/gin-gonic/gin"
)

// authMiddleware returns the authentication middleware
func (s *GinServer) authMiddleware() gin.HandlerFunc {
	if s.authManager != nil && s.config.Auth.Enabled {
		return s.authManager.AuthMiddleware()
	}
	// Return a no-op middleware if auth is disabled
	return func(c *gin.Context) {
		c.Next()
	}
}