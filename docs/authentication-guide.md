# Authentication & Authorization Guide

This guide explains how to use the TUF Server's authentication and authorization features, including JWT tokens and API key support.

## Overview

The TUF Server provides comprehensive authentication and authorization capabilities:

- **JWT Tokens** - Time-limited bearer tokens for user authentication
- **API Keys** - Long-lived keys for service-to-service authentication  
- **Role-Based Access Control** - Admin, write, and read permissions
- **Secure Defaults** - Authentication enabled by default with example credentials

## Authentication Methods

### 1. API Key Authentication

API keys provide simple, long-lived authentication for services and automated tools.

**Using API Keys in Requests:**

```bash
# Using X-API-Key header (recommended)
curl -H "X-API-Key: your-api-key-here" http://localhost:8080/admin/stats

# Using Authorization header
curl -H "Authorization: Bearer your-api-key-here" http://localhost:8080/admin/stats
```

**Default API Key:**
- Key: `admin-key-example`
- Description: "Default admin API key - change in production"
- Role: admin
- Permissions: `*` (all permissions)

### 2. JWT Token Authentication

JWT tokens provide secure, time-limited authentication with embedded claims.

**Login to Get JWT Token:**

```bash
curl -X POST http://localhost:8080/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "changeme"
  }'
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2025-01-16T10:30:00Z",
  "user": {
    "username": "admin",
    "role": "admin",
    "permissions": ["*"]
  }
}
```

**Using JWT Token:**

```bash
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  http://localhost:8080/admin/stats
```

## Default Credentials

⚠️ **Security Warning**: Change these default credentials in production!

### Default Admin User
- **Username**: `admin`
- **Password**: `changeme`
- **Role**: `admin`
- **Permissions**: `*` (full access)

### Default API Key
- **Key**: `admin-key-example`
- **Role**: `admin`
- **Permissions**: `*` (full access)

### Default JWT Secret
- **Secret**: `change-this-secret-in-production-use-at-least-32-chars`

## API Endpoints

### Authentication Endpoints

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| `POST` | `/admin/auth/login` | Login with username/password to get JWT | No |
| `POST` | `/admin/auth/generate-api-key` | Generate new API key | Yes (Admin) |
| `GET` | `/admin/auth/verify` | Verify current authentication | Yes |

### Admin Endpoints (Require Authentication)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/admin/stats` | Server statistics |
| `GET` | `/admin/audit/logs` | Audit logs |
| `POST` | `/admin/targets/add` | Add target files |
| `POST` | `/admin/targets/remove` | Remove target files |
| `POST` | `/admin/metadata/sign` | Sign metadata |

## Configuration

### Environment Variables

```bash
# Enable/disable authentication
export TUF_AUTH_ENABLED=true

# JWT configuration
export TUF_JWT_SECRET="your-secret-key-here"
export TUF_JWT_EXPIRATION="24h"

# API keys (JSON format)
export TUF_API_KEYS='{"key1":"Description 1","key2":"Description 2"}'
```

### Configuration File

```json
{
  "auth": {
    "enabled": true,
    "jwt_secret": "your-secret-key-here",
    "jwt_expiration": "24h",
    "api_keys": {
      "your-api-key": "Production API key",
      "service-key": "Service authentication"
    },
    "admin_users": [
      {
        "username": "admin",
        "password": "hashed-password-here",
        "role": "admin",
        "permissions": ["*"]
      }
    ]
  }
}
```

## Managing API Keys

### Generate New API Key

```bash
curl -X POST http://localhost:8080/admin/auth/generate-api-key \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Service Key",
    "role": "admin",
    "permissions": ["metadata:read", "targets:read"],
    "expires_in": "1y"
  }'
```

**Response:**
```json
{
  "api_key": "generated-key-here",
  "name": "Production Service Key",
  "role": "admin",
  "permissions": ["metadata:read", "targets:read"],
  "created_at": "2025-01-15T10:30:00Z",
  "expires_at": "2026-01-15T10:30:00Z"
}
```

## Role-Based Access Control

### Available Roles

- **admin**: Full access to all operations
- **write**: Can modify repositories and metadata
- **read**: Read-only access to public repositories

### Permission System

Permissions use a hierarchical format:

```
*                    # All permissions
metadata:*           # All metadata operations
metadata:read        # Read metadata only
targets:*            # All target operations
targets:write        # Write targets
repositories:*       # All repository operations
repositories:create  # Create repositories
admin:*              # All admin operations
```

### Example Permission Sets

```json
{
  "admin_user": ["*"],
  "ci_service": ["metadata:read", "targets:read", "targets:write"],
  "read_only": ["metadata:read", "targets:read"],
  "repo_manager": ["repositories:*", "metadata:*"]
}
```

## Security Best Practices

### Production Deployment

1. **Change Default Credentials**:
   ```bash
   # Generate secure JWT secret (32+ characters)
   openssl rand -hex 32
   
   # Generate secure API keys
   openssl rand -hex 24
   ```

2. **Use Hashed Passwords**:
   ```json
   {
     "admin_users": [
       {
         "username": "admin",
         "password": "$2a$10$hashed-password-here",
         "role": "admin",
         "permissions": ["*"]
       }
     ]
   }
   ```

3. **Rotate API Keys Regularly**:
   - Set expiration dates on API keys
   - Monitor key usage in audit logs
   - Revoke unused or compromised keys

4. **Use HTTPS in Production**:
   ```json
   {
     "tls": {
       "enabled": true,
       "cert_file": "/path/to/cert.pem",
       "key_file": "/path/to/key.pem"
     }
   }
   ```

### Network Security

- Use API keys only over HTTPS
- Implement IP allowlists for sensitive operations
- Monitor authentication attempts in logs
- Use short JWT expiration times (1-24 hours)

## Troubleshooting

### Common Issues

1. **Authentication Required Error**:
   ```json
   {"error": "Authentication required"}
   ```
   **Solution**: Include valid API key or JWT token in request

2. **Invalid API Key**:
   ```json
   {"error": "Invalid API key"}
   ```
   **Solution**: Verify API key is correct and not expired

3. **Insufficient Permissions**:
   ```json
   {"error": "Insufficient permissions"}
   ```
   **Solution**: Check user role and permissions for the endpoint

### Debug Commands

```bash
# Verify authentication
curl -H "X-API-Key: your-key" http://localhost:8080/admin/auth/verify

# Check server stats (requires auth)
curl -H "X-API-Key: your-key" http://localhost:8080/admin/stats

# View audit logs (admin only)
curl -H "X-API-Key: your-key" http://localhost:8080/admin/audit/logs
```

## Client Integration Examples

### Go Client

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

func loginAndGetToken(username, password string) (string, error) {
    loginData := map[string]string{
        "username": username,
        "password": password,
    }
    
    jsonData, _ := json.Marshal(loginData)
    resp, err := http.Post("http://localhost:8080/admin/auth/login", 
        "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    
    return result["token"].(string), nil
}

func makeAuthenticatedRequest(token, endpoint string) (*http.Response, error) {
    req, _ := http.NewRequest("GET", endpoint, nil)
    req.Header.Set("Authorization", "Bearer "+token)
    
    client := &http.Client{}
    return client.Do(req)
}
```

### Python Client

```python
import requests

def login_and_get_token(username, password):
    response = requests.post('http://localhost:8080/admin/auth/login', 
        json={'username': username, 'password': password})
    return response.json()['token']

def make_authenticated_request(token, endpoint):
    headers = {'Authorization': f'Bearer {token}'}
    return requests.get(endpoint, headers=headers)

# Usage
token = login_and_get_token('admin', 'changeme')
response = make_authenticated_request(token, 'http://localhost:8080/admin/stats')
```

### Shell Script

```bash
#!/bin/bash

# Login and get token
TOKEN=$(curl -s -X POST http://localhost:8080/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"changeme"}' | \
  jq -r '.token')

# Use token for authenticated requests
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/admin/stats
```

This authentication system provides enterprise-grade security for your TUF repository server while maintaining ease of use for both interactive and automated access patterns.