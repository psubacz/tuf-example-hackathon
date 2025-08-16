package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Client represents an HTTP client for the TUF server
type Client struct {
	config     *Config
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new TUF server client
func NewClient(config *Config) *Client {
	return &Client{
		config:  config,
		baseURL: config.ServerURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs an HTTP request with authentication
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication headers
	if c.config.APIKey != "" {
		req.Header.Set("X-API-Key", c.config.APIKey)
	} else if c.config.JWTToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.JWTToken)
	}

	// Add content type for JSON requests
	if body != nil && method != "GET" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// get performs a GET request
func (c *Client) get(path string) (*http.Response, error) {
	return c.doRequest("GET", path, nil)
}

// post performs a POST request
func (c *Client) post(path string, body interface{}) (*http.Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	return c.doRequest("POST", path, bytes.NewReader(jsonBody))
}

// delete performs a DELETE request
func (c *Client) delete(path string) (*http.Response, error) {
	return c.doRequest("DELETE", path, nil)
}

// GetStatus retrieves server status
func (c *Client) GetStatus() (*ServerStatus, error) {
	resp, err := c.get("/api/v1/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var status ServerStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &status, nil
}

// CheckConnectivity checks server connectivity
func (c *Client) CheckConnectivity() (*ConnectivityResult, error) {
	result := &ConnectivityResult{
		ServerURL: c.baseURL,
		Timestamp: time.Now(),
	}

	// Check health endpoint
	healthResp, err := c.get("/health")
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	defer healthResp.Body.Close()

	result.HealthStatus = healthResp.StatusCode
	
	// Check ready endpoint
	readyResp, err := c.get("/health/ready")
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	defer readyResp.Body.Close()

	result.ReadyStatus = readyResp.StatusCode

	// Check admin access if authenticated
	if c.config.APIKey != "" || c.config.JWTToken != "" {
		statsResp, err := c.get("/admin/stats")
		if err != nil {
			result.AdminAccess = false
		} else {
			statsResp.Body.Close()
			result.AdminAccess = statsResp.StatusCode == http.StatusOK
		}
	}

	result.Connected = result.HealthStatus == http.StatusOK
	return result, nil
}

// UploadFile uploads a file to the TUF repository
func (c *Client) UploadFile(localPath, remotePath string) (*UploadResult, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file field
	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Add path field
	if err := writer.WriteField("path", remotePath); err != nil {
		return nil, fmt.Errorf("failed to write path field: %w", err)
	}

	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", c.baseURL+"/admin/targets/add", body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	// Add authentication
	if c.config.APIKey != "" {
		req.Header.Set("X-API-Key", c.config.APIKey)
	} else if c.config.JWTToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.JWTToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	result := &UploadResult{
		LocalPath:  localPath,
		RemotePath: remotePath,
		Size:       fileInfo.Size(),
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Error = string(body)
	} else {
		result.Success = true
	}

	return result, nil
}

// DeleteFile deletes a file from the TUF repository
func (c *Client) DeleteFile(path string) (*DeleteResult, error) {
	req := map[string]string{"path": path}
	
	resp, err := c.post("/admin/targets/remove", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := &DeleteResult{
		Path:       path,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Error = string(body)
	} else {
		result.Success = true
	}

	return result, nil
}

// VerifyFile verifies a file in the TUF repository
func (c *Client) VerifyFile(path string) (*VerifyResult, error) {
	// First, get file metadata
	metaResp, err := c.get("/targets/" + path)
	if err != nil {
		return nil, err
	}
	defer metaResp.Body.Close()

	result := &VerifyResult{
		Path:   path,
		Exists: metaResp.StatusCode == http.StatusOK,
	}

	if result.Exists {
		// Get content length
		if contentLength := metaResp.Header.Get("Content-Length"); contentLength != "" {
			fmt.Sscanf(contentLength, "%d", &result.Size)
		}

		// Calculate checksum
		hasher := newSHA256Hasher()
		if _, err := io.Copy(hasher, metaResp.Body); err != nil {
			return nil, fmt.Errorf("failed to calculate checksum: %w", err)
		}
		result.Checksum = hasher.Sum()

		// Check if file is in metadata
		targetsResp, err := c.get("/metadata/targets.json")
		if err == nil {
			defer targetsResp.Body.Close()
			var targets TargetsMetadata
			if err := json.NewDecoder(targetsResp.Body).Decode(&targets); err == nil {
				if target, ok := targets.Signed.Targets[path]; ok {
					result.InMetadata = true
					result.MetadataChecksum = target.Hashes["sha256"]
					result.Valid = result.Checksum == result.MetadataChecksum
				}
			}
		}
	}

	return result, nil
}

// Login authenticates with the server and gets a JWT token
func (c *Client) Login(username, password string) (*LoginResult, error) {
	req := map[string]string{
		"username": username,
		"password": password,
	}

	resp, err := c.post("/admin/auth/login", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := &LoginResult{
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusOK {
		var loginResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&loginResp); err == nil {
			if token, ok := loginResp["token"].(string); ok {
				result.Token = token
				result.Success = true
			}
			if exp, ok := loginResp["expires_at"].(string); ok {
				result.ExpiresAt = exp
			}
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		result.Error = string(body)
	}

	return result, nil
}

// GenerateAPIKey generates a new API key
func (c *Client) GenerateAPIKey(name, role string, permissions []string) (*APIKeyResult, error) {
	req := map[string]interface{}{
		"name":        name,
		"role":        role,
		"permissions": permissions,
	}

	resp, err := c.post("/admin/auth/generate-api-key", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := &APIKeyResult{
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusOK {
		var keyResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&keyResp); err == nil {
			if key, ok := keyResp["api_key"].(string); ok {
				result.APIKey = key
				result.Success = true
			}
			if name, ok := keyResp["name"].(string); ok {
				result.Name = name
			}
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		result.Error = string(body)
	}

	return result, nil
}

// RotateKeys initiates key rotation
func (c *Client) RotateKeys(keyType string) (*KeyRotationResult, error) {
	req := map[string]string{
		"key_type": keyType,
	}

	resp, err := c.post("/admin/keys/rotate", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result := &KeyRotationResult{
		KeyType:    keyType,
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode == http.StatusOK {
		result.Success = true
		var rotateResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&rotateResp); err == nil {
			if newKeyID, ok := rotateResp["new_key_id"].(string); ok {
				result.NewKeyID = newKeyID
			}
			if oldKeyID, ok := rotateResp["old_key_id"].(string); ok {
				result.OldKeyID = oldKeyID
			}
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		result.Error = string(body)
	}

	return result, nil
}