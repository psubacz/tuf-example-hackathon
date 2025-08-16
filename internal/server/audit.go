package server

import (
	"time"

	"github.com/gin-gonic/gin"
)

// AuditEntry represents an audit log entry
type AuditEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Action    string                 `json:"action"`
	User      string                 `json:"user,omitempty"`
	IP        string                 `json:"ip"`
	Method    string                 `json:"method"`
	Path      string                 `json:"path"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Success   bool                   `json:"success"`
}

// auditLog logs an audit event
func (s *GinServer) auditLog(c *gin.Context, action string, data map[string]interface{}) {
	entry := AuditEntry{
		Timestamp: time.Now(),
		Action:    action,
		IP:        c.ClientIP(),
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		Data:      data,
		Success:   true,
	}

	// Extract user from context if authenticated
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(string); ok {
			entry.User = u
		}
	}

	// Log the audit entry
	s.logger.Info("Audit log",
		"action", entry.Action,
		"user", entry.User,
		"ip", entry.IP,
		"method", entry.Method,
		"path", entry.Path,
		"data", entry.Data,
	)

	// In a production system, this would also:
	// - Store in a database
	// - Send to a SIEM system
	// - Trigger webhooks for certain actions
}