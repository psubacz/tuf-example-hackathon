package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"tuf-golang-project/internal/webhook"
)

// setupWebhookRoutes sets up webhook-related routes
func (s *GinServer) setupWebhookRoutes(router *gin.RouterGroup) {
	// Webhook management endpoints (admin only)
	admin := router.Group("/admin/webhooks")
	admin.Use(s.authMiddleware())
	{
		admin.POST("/", s.createWebhook)
		admin.GET("/", s.listWebhooks)
		admin.GET("/:id", s.getWebhook)
		admin.PUT("/:id", s.updateWebhook)
		admin.DELETE("/:id", s.deleteWebhook)
		admin.POST("/:id/test", s.testWebhook)
	}

	// Public polling endpoint for sidecars
	router.GET("/webhooks/poll", s.pollEvents)
	router.GET("/webhooks/events/:id", s.getEvent)
}

// createWebhook creates a new webhook endpoint
func (s *GinServer) createWebhook(c *gin.Context) {
	var endpoint webhook.WebhookEndpoint
	if err := c.ShouldBindJSON(&endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.webhookManager.RegisterEndpoint(&endpoint); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.auditLog(c, "webhook.created", gin.H{
		"webhook_id": endpoint.ID,
		"url":        endpoint.URL,
	})

	c.JSON(http.StatusCreated, endpoint)
}

// listWebhooks lists all webhook endpoints
func (s *GinServer) listWebhooks(c *gin.Context) {
	endpoints := s.webhookManager.ListEndpoints()
	c.JSON(http.StatusOK, gin.H{
		"webhooks": endpoints,
		"total":    len(endpoints),
	})
}

// getWebhook retrieves a specific webhook
func (s *GinServer) getWebhook(c *gin.Context) {
	id := c.Param("id")
	
	endpoint, ok := s.webhookManager.GetEndpoint(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	c.JSON(http.StatusOK, endpoint)
}

// updateWebhook updates a webhook endpoint
func (s *GinServer) updateWebhook(c *gin.Context) {
	id := c.Param("id")
	
	var endpoint webhook.WebhookEndpoint
	if err := c.ShouldBindJSON(&endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Ensure ID matches
	endpoint.ID = id
	
	if err := s.webhookManager.RegisterEndpoint(&endpoint); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.auditLog(c, "webhook.updated", gin.H{"webhook_id": id})

	c.JSON(http.StatusOK, endpoint)
}

// deleteWebhook removes a webhook endpoint
func (s *GinServer) deleteWebhook(c *gin.Context) {
	id := c.Param("id")
	
	if err := s.webhookManager.UnregisterEndpoint(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.auditLog(c, "webhook.deleted", gin.H{"webhook_id": id})

	c.JSON(http.StatusNoContent, nil)
}

// testWebhook sends a test event to a webhook
func (s *GinServer) testWebhook(c *gin.Context) {
	id := c.Param("id")
	
	endpoint, ok := s.webhookManager.GetEndpoint(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}

	// Get repository from context or default
	repo := c.Query("repository")
	if repo == "" {
		repo = "default/test"
	}

	// Send test event
	err := s.webhookManager.Publish(
		webhook.EventType("webhook.test"),
		repo,
		map[string]interface{}{
			"message":    "This is a test event",
			"webhook_id": id,
			"timestamp":  time.Now(),
		},
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Test event sent",
		"webhook": endpoint.URL,
	})
}

// pollEvents allows sidecars to poll for events
func (s *GinServer) pollEvents(c *gin.Context) {
	repository := c.Query("repository")
	if repository == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repository parameter required"})
		return
	}

	// Parse since timestamp
	sinceStr := c.Query("since")
	var since time.Time
	if sinceStr != "" {
		sinceUnix, err := strconv.ParseInt(sinceStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since timestamp"})
			return
		}
		since = time.Unix(sinceUnix, 0)
	} else {
		since = time.Now().Add(-1 * time.Hour)
	}

	// Parse limit
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 100
	}

	// Get events
	events, err := s.webhookManager.GetEventsSince(repository, since, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Add long polling support
	if len(events) == 0 && c.Query("wait") == "true" {
		timeout := 30 * time.Second
		if timeoutStr := c.Query("timeout"); timeoutStr != "" {
			if t, err := strconv.Atoi(timeoutStr); err == nil {
				timeout = time.Duration(t) * time.Second
			}
		}

		// Wait for new events
		ctx := c.Request.Context()
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			c.JSON(http.StatusRequestTimeout, []webhook.Event{})
			return
		case <-timer.C:
			// Check one more time
			events, _ = s.webhookManager.GetEventsSince(repository, since, limit)
		}
	}

	c.JSON(http.StatusOK, events)
}

// getEvent retrieves a specific event by ID
func (s *GinServer) getEvent(c *gin.Context) {
	id := c.Param("id")
	
	event, err := s.webhookManager.GetEvent(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}

	c.JSON(http.StatusOK, event)
}

// publishEvent publishes a webhook event
func (s *GinServer) publishEvent(eventType webhook.EventType, repository string, data map[string]interface{}) {
	if s.webhookManager == nil {
		return
	}

	if err := s.webhookManager.Publish(eventType, repository, data); err != nil {
		s.logger.Error("Failed to publish webhook event", 
			"type", eventType,
			"repository", repository,
			"error", err,
		)
	}
}