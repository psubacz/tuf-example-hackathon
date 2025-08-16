package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret        string
	JWTIssuer        string
	JWTExpiration    time.Duration
	APIKeys          map[string]APIKey
	EnableJWT        bool
	EnableAPIKeys    bool
	RequireAuth      bool
}

// APIKey represents an API key with metadata
type APIKey struct {
	Key         string
	Name        string
	Role        string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	Permissions []string
}

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

// AuthManager handles authentication
type AuthManager struct {
	config    *AuthConfig
	jwtSecret []byte
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(config *AuthConfig) *AuthManager {
	if config.JWTSecret == "" {
		// Generate a random secret if not provided
		secret := make([]byte, 32)
		rand.Read(secret)
		config.JWTSecret = hex.EncodeToString(secret)
	}
	
	if config.JWTExpiration == 0 {
		config.JWTExpiration = 24 * time.Hour
	}
	
	if config.JWTIssuer == "" {
		config.JWTIssuer = "tuf-server"
	}
	
	// Initialize default API keys if none provided
	if config.APIKeys == nil {
		config.APIKeys = make(map[string]APIKey)
	}
	
	return &AuthManager{
		config:    config,
		jwtSecret: []byte(config.JWTSecret),
	}
}

// GenerateJWT generates a new JWT token
func (am *AuthManager) GenerateJWT(username, role string, permissions []string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			Issuer:    am.config.JWTIssuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(am.config.JWTExpiration)),
			ID:        generateTokenID(),
		},
		Username:    username,
		Role:        role,
		Permissions: permissions,
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(am.jwtSecret)
}

// ValidateJWT validates a JWT token
func (am *AuthManager) ValidateJWT(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return am.jwtSecret, nil
	})
	
	if err != nil {
		return nil, err
	}
	
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	
	return nil, errors.New("invalid token")
}

// ValidateAPIKey validates an API key
func (am *AuthManager) ValidateAPIKey(key string) (*APIKey, error) {
	// Remove any "Bearer " prefix if present
	key = strings.TrimPrefix(key, "Bearer ")
	key = strings.TrimPrefix(key, "ApiKey ")
	
	apiKey, exists := am.config.APIKeys[key]
	if !exists {
		return nil, errors.New("invalid API key")
	}
	
	// Check if key has expired
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, errors.New("API key expired")
	}
	
	return &apiKey, nil
}

// GenerateAPIKey generates a new API key
func (am *AuthManager) GenerateAPIKey(name, role string, permissions []string, expiresIn *time.Duration) (string, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", err
	}
	
	key := hex.EncodeToString(keyBytes)
	
	var expiresAt *time.Time
	if expiresIn != nil {
		expTime := time.Now().Add(*expiresIn)
		expiresAt = &expTime
	}
	
	am.config.APIKeys[key] = APIKey{
		Key:         key,
		Name:        name,
		Role:        role,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
		Permissions: permissions,
	}
	
	return key, nil
}

// AuthMiddleware returns a Gin middleware for authentication
func (am *AuthManager) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !am.config.RequireAuth {
			c.Next()
			return
		}
		
		// Try to authenticate with different methods
		authenticated := false
		var userRole string
		var permissions []string
		
		// Check Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			// Try JWT first
			if am.config.EnableJWT && strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if claims, err := am.ValidateJWT(token); err == nil {
					authenticated = true
					userRole = claims.Role
					permissions = claims.Permissions
					c.Set("username", claims.Username)
					c.Set("user_id", claims.Subject)
				}
			}
			
			// Try API key
			if !authenticated && am.config.EnableAPIKeys {
				if apiKey, err := am.ValidateAPIKey(authHeader); err == nil {
					authenticated = true
					userRole = apiKey.Role
					permissions = apiKey.Permissions
					c.Set("api_key_name", apiKey.Name)
				}
			}
		}
		
		// Check X-API-Key header
		if !authenticated && am.config.EnableAPIKeys {
			apiKeyHeader := c.GetHeader("X-API-Key")
			if apiKeyHeader != "" {
				if apiKey, err := am.ValidateAPIKey(apiKeyHeader); err == nil {
					authenticated = true
					userRole = apiKey.Role
					permissions = apiKey.Permissions
					c.Set("api_key_name", apiKey.Name)
				}
			}
		}
		
		if !authenticated {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"message": "Please provide a valid JWT token or API key",
			})
			c.Abort()
			return
		}
		
		// Set user context
		c.Set("authenticated", true)
		c.Set("role", userRole)
		c.Set("permissions", permissions)
		
		c.Next()
	}
}

// RequireRole returns a middleware that requires a specific role
func (am *AuthManager) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied",
				"message": "Role not found",
			})
			c.Abort()
			return
		}
		
		roleStr, ok := userRole.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied",
				"message": "Invalid role type",
			})
			c.Abort()
			return
		}
		
		// Check if user has any of the required roles
		hasRole := false
		for _, role := range roles {
			if roleStr == role {
				hasRole = true
				break
			}
		}
		
		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied",
				"message": fmt.Sprintf("Required role: %v", roles),
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// RequirePermission returns a middleware that requires specific permissions
func (am *AuthManager) RequirePermission(requiredPerms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userPerms, exists := c.Get("permissions")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied",
				"message": "Permissions not found",
			})
			c.Abort()
			return
		}
		
		perms, ok := userPerms.([]string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Access denied",
				"message": "Invalid permissions type",
			})
			c.Abort()
			return
		}
		
		// Check if user has all required permissions
		for _, required := range requiredPerms {
			hasPermission := false
			for _, perm := range perms {
				if perm == required || perm == "*" { // "*" is wildcard for all permissions
					hasPermission = true
					break
				}
			}
			
			if !hasPermission {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "Access denied",
					"message": fmt.Sprintf("Missing permission: %s", required),
				})
				c.Abort()
				return
			}
		}
		
		c.Next()
	}
}

// Helper functions

func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ConstantTimeCompare performs constant time comparison of two strings
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}