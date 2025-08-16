package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"tuf-golang-project/internal/repository"
	"tuf-golang-project/internal/webhook"
)

// setupRepositoryRoutes sets up repository management routes
func (s *GinServer) setupRepositoryRoutes(router *gin.RouterGroup) {
	// Repository management endpoints (admin only)
	admin := router.Group("/admin/repositories")
	admin.Use(s.authMiddleware())
	{
		admin.POST("/", s.createRepository)
		admin.GET("/", s.listRepositories)
		admin.GET("/:namespace/:name", s.getRepository)
		admin.PUT("/:namespace/:name", s.updateRepository)
		admin.DELETE("/:namespace/:name", s.deleteRepository)
		admin.GET("/:namespace/:name/stats", s.getRepositoryStats)
	}

	// Public repository endpoints (with namespace support)
	router.GET("/repositories", s.listPublicRepositories)
	router.GET("/repositories/:namespace/:name", s.getPublicRepository)
}

// createRepository creates a new repository
func (s *GinServer) createRepository(c *gin.Context) {
	var req struct {
		Namespace   string                       `json:"namespace" binding:"required"`
		Name        string                       `json:"name" binding:"required"`
		Description string                       `json:"description"`
		Config      *repository.RepositoryConfig `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default config if not provided
	if req.Config == nil {
		req.Config = &repository.RepositoryConfig{
			Public:        false,
			MaxFileSize:   100 * 1024 * 1024, // 100MB
			RetentionDays: 90,
		}
	}

	repo, err := s.repoManager.CreateRepository(req.Namespace, req.Name, req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Publish webhook event
	s.publishEvent(webhook.EventRepoCreated, fmt.Sprintf("%s/%s", req.Namespace, req.Name), map[string]interface{}{
		"namespace":   req.Namespace,
		"name":        req.Name,
		"description": req.Description,
	})

	s.auditLog(c, "repository.created", gin.H{
		"namespace": req.Namespace,
		"name":      req.Name,
	})

	c.JSON(http.StatusCreated, repo)
}

// listRepositories lists all repositories (admin view)
func (s *GinServer) listRepositories(c *gin.Context) {
	namespace := c.Query("namespace")
	repos := s.repoManager.ListRepositories(namespace)
	
	c.JSON(http.StatusOK, gin.H{
		"repositories": repos,
		"total":        len(repos),
		"namespace":    namespace,
	})
}

// listPublicRepositories lists public repositories only
func (s *GinServer) listPublicRepositories(c *gin.Context) {
	namespace := c.Query("namespace")
	allRepos := s.repoManager.ListRepositories(namespace)
	
	// Filter public repositories
	var publicRepos []*repository.Repository
	for _, repo := range allRepos {
		if repo.IsPublic() {
			publicRepos = append(publicRepos, repo)
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"repositories": publicRepos,
		"total":        len(publicRepos),
		"namespace":    namespace,
	})
}

// getRepository retrieves a specific repository
func (s *GinServer) getRepository(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	
	repo, err := s.repoManager.GetRepository(namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, repo)
}

// getPublicRepository retrieves a public repository
func (s *GinServer) getPublicRepository(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	
	repo, err := s.repoManager.GetRepository(namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Check if repository is public
	if !repo.IsPublic() {
		c.JSON(http.StatusForbidden, gin.H{"error": "repository is not public"})
		return
	}

	c.JSON(http.StatusOK, repo)
}

// updateRepository updates repository configuration
func (s *GinServer) updateRepository(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	
	var config repository.RepositoryConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.repoManager.UpdateRepository(namespace, name, &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.auditLog(c, "repository.updated", gin.H{
		"namespace": namespace,
		"name":      name,
	})

	c.JSON(http.StatusOK, gin.H{"message": "repository updated"})
}

// deleteRepository deletes a repository
func (s *GinServer) deleteRepository(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	
	if err := s.repoManager.DeleteRepository(namespace, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Publish webhook event
	s.publishEvent(webhook.EventRepoDeleted, fmt.Sprintf("%s/%s", namespace, name), map[string]interface{}{
		"namespace": namespace,
		"name":      name,
	})

	s.auditLog(c, "repository.deleted", gin.H{
		"namespace": namespace,
		"name":      name,
	})

	c.JSON(http.StatusNoContent, nil)
}

// getRepositoryStats retrieves repository statistics
func (s *GinServer) getRepositoryStats(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	
	repo, err := s.repoManager.GetRepository(namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Get target files
	targets, err := repo.ListTargets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate stats
	var totalSize int64
	for _, target := range targets {
		// Would need to get file info to calculate size
		_ = target
	}

	c.JSON(http.StatusOK, gin.H{
		"namespace":    namespace,
		"name":         name,
		"total_files":  len(targets),
		"total_size":   totalSize,
		"created_at":   repo.CreatedAt,
		"updated_at":   repo.UpdatedAt,
		"config":       repo.Config,
	})
}

// Middleware to extract repository from request path
func (s *GinServer) repositoryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to extract namespace/name from various path patterns
		path := c.Request.URL.Path
		
		// Pattern: /api/v1/:namespace/:name/...
		if parts := strings.Split(path, "/"); len(parts) >= 5 && parts[1] == "api" {
			if parts[3] != "" && parts[4] != "" {
				namespace := parts[3]
				name := parts[4]
				
				repo, err := s.repoManager.GetRepository(namespace, name)
				if err == nil {
					c.Set("repository", repo)
					c.Set("repository_id", fmt.Sprintf("%s/%s", namespace, name))
				}
			}
		}
		
		// Pattern: /repositories/:namespace/:name/...
		if strings.HasPrefix(path, "/repositories/") {
			parts := strings.Split(path[len("/repositories/"):], "/")
			if len(parts) >= 2 {
				namespace := parts[0]
				name := parts[1]
				
				repo, err := s.repoManager.GetRepository(namespace, name)
				if err == nil {
					c.Set("repository", repo)
					c.Set("repository_id", fmt.Sprintf("%s/%s", namespace, name))
				}
			}
		}
		
		c.Next()
	}
}

