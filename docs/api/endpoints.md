# TUF Server API Documentation

## Overview

The TUF server provides a RESTful API for accessing TUF metadata and target files. All endpoints follow TUF specification patterns and include appropriate security headers.

## Base URL

```
http://localhost:8080  (development)
https://your-domain.com  (production)
```

## Authentication

This implementation has **authentication enabled by default** for administrative operations. Public TUF metadata and target files remain accessible without authentication.

### Default Credentials (Change in Production!)

**Admin User:**
- Username: `admin`
- Password: `changeme`

**API Key:**
- Key: `admin-key-example`
- Description: "Default admin API key - change in production"

### Authentication Methods

**API Key (Recommended for services):**
```bash
curl -H "X-API-Key: admin-key-example" http://localhost:8080/admin/stats
```

**JWT Token (For interactive use):**
```bash
# Login to get JWT token
curl -X POST http://localhost:8080/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "changeme"}'

# Use JWT token
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" http://localhost:8080/admin/stats
```

## Content Types

- **Metadata files**: `application/json`
- **Target files**: Detected based on file extension
- **API responses**: `application/json`

## Endpoints

### Health and Monitoring

#### GET /health

Returns server health status and basic system information.

**Response Example:**
```json
{
  "status": "healthy",
  "timestamp": "2024-08-15T10:30:00Z",
  "version": "2.0.0",
  "uptime": "2h15m30s",
  "repository_path": "/app/tuf-repository-v2",
  "checks": {
    "repository_access": "ok",
    "disk_space": "ok",
    "memory": "ok"
  }
}
```

**Response Codes:**
- `200`: Server is healthy
- `503`: Server is unhealthy

#### GET /metrics

Returns Prometheus-compatible metrics for monitoring.

**Response Example:**
```
# HELP tuf_requests_total Total number of requests
# TYPE tuf_requests_total counter
tuf_requests_total{method="GET",endpoint="/metadata/root.json",status="200"} 150

# HELP tuf_request_duration_seconds Request duration in seconds
# TYPE tuf_request_duration_seconds histogram
tuf_request_duration_seconds_bucket{method="GET",endpoint="/metadata/root.json",le="0.1"} 120
```

#### GET /api/v1/status

Returns current server status and operational state.

**Response Example:**
```json
{
  "status": "operational",
  "timestamp": "2025-08-16T19:40:57-04:00",
  "request_id": "req-12345"
}
```

**Response Codes:**
- `200`: Server is operational
- `503`: Server is experiencing issues

#### GET /api/v1/info

Returns detailed repository information and configuration.

**Response Example:**
```json
{
  "repository": {
    "path": "/app/tuf-repository-v2",
    "metadata_files": 4,
    "target_files": 1,
    "total_size": "15.2 KB",
    "server_port": 8080
  },
  "server": {
    "version": "2.0.0",
    "uptime": "2h15m30s",
    "total_requests": 1250,
    "tls_enabled": false,
    "compression_enabled": true,
    "rate_limiting_enabled": true
  },
  "tuf": {
    "specification_version": "1.0.31",
    "go_tuf_version": "v2.1.1",
    "supported_algorithms": ["ed25519", "sha256"]
  }
}
```

### TUF Metadata Endpoints

#### GET /metadata/

Lists all available metadata files.

**Response Example:**
```json
{
  "metadata_files": [
    {
      "name": "root.json",
      "size": 2048,
      "modified": "2024-08-15T10:00:00Z",
      "checksum": "sha256:abc123...",
      "download_url": "http://localhost:8080/metadata/root.json"
    },
    {
      "name": "targets.json",
      "size": 1024,
      "modified": "2024-08-15T10:01:00Z",
      "checksum": "sha256:def456...",
      "download_url": "http://localhost:8080/metadata/targets.json"
    }
  ],
  "count": 4,
  "timestamp": "2024-08-15T10:30:00Z"
}
```

#### GET /metadata/root.json

Returns the root metadata file containing root keys and role definitions.

**Response Headers:**
```
Content-Type: application/json
Cache-Control: public, max-age=3600
ETag: "abc123..."
```

**Response Example:**
```json
{
  "signatures": [
    {
      "keyid": "76571ce8bf2842f4...",
      "sig": "6d5361cc01c5bac1eca2627b..."
    }
  ],
  "signed": {
    "_type": "root",
    "spec_version": "1.0.31",
    "version": 1,
    "expires": "2025-08-15T15:47:45Z",
    "keys": {
      "76571ce8bf2842f4...": {
        "keytype": "ed25519",
        "scheme": "ed25519",
        "keyval": {
          "public": "302a300506032b6570032100..."
        }
      }
    },
    "roles": {
      "root": {
        "keyids": ["76571ce8bf2842f4..."],
        "threshold": 1
      },
      "targets": {
        "keyids": ["a1b2c3d4e5f6..."],
        "threshold": 1
      },
      "snapshot": {
        "keyids": ["b2c3d4e5f6a1..."],
        "threshold": 1
      },
      "timestamp": {
        "keyids": ["c3d4e5f6a1b2..."],
        "threshold": 1
      }
    }
  }
}
```

#### GET /metadata/targets.json

Returns the targets metadata file listing all available target files.

**Response Example:**
```json
{
  "signatures": [
    {
      "keyid": "a1b2c3d4e5f6...",
      "sig": "7e8f9a0b1c2d3e4f..."
    }
  ],
  "signed": {
    "_type": "targets",
    "spec_version": "1.0.31",
    "version": 1,
    "expires": "2024-09-15T15:47:45Z",
    "targets": {
      "sample.txt": {
        "length": 13,
        "hashes": {
          "sha256": "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"
        }
      }
    }
  }
}
```

#### GET /metadata/snapshot.json

Returns the snapshot metadata file with current metadata versions.

#### GET /metadata/timestamp.json

Returns the timestamp metadata file with current snapshot version.

### Target File Endpoints

#### GET /targets/

Lists all available target files.

**Response Example:**
```json
{
  "target_files": [
    {
      "name": "sample.txt",
      "size": 13,
      "modified": "2024-08-15T10:00:00Z",
      "checksum": "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
      "content_type": "text/plain",
      "download_url": "http://localhost:8080/targets/sample.txt"
    }
  ],
  "count": 1,
  "timestamp": "2024-08-15T10:30:00Z"
}
```

#### GET /targets/{filename}

Downloads a specific target file.

**Path Parameters:**
- `filename`: Name of the target file to download

**Response Headers:**
```
Content-Type: text/plain (varies by file type)
Cache-Control: public, max-age=3600
Content-Length: 13
ETag: "def456..."
```

**Security Features:**
- Path traversal protection (rejects `..` sequences)
- File existence validation
- Appropriate MIME type detection
- Secure HTTP headers

## Error Responses

### Standard Error Format

```json
{
  "error": "Error description",
  "code": "ERROR_CODE",
  "timestamp": "2024-08-15T10:30:00Z",
  "path": "/metadata/nonexistent.json"
}
```

### Error Codes

- **400 Bad Request**: Invalid file path or malformed request
- **404 Not Found**: Requested file does not exist
- **429 Too Many Requests**: Rate limit exceeded
- **500 Internal Server Error**: Server-side error
- **503 Service Unavailable**: Server maintenance or overloaded

## Security Headers

All responses include security headers:

```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'self'
```

For HTTPS deployments:
```
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

## Rate Limiting

The server implements token bucket rate limiting:

- **Default Limit**: 100 requests per minute per IP
- **Burst Size**: 10 requests
- **Headers**: Rate limit information in response headers
- **Whitelist**: Configurable IP whitelist
- **Blacklist**: Configurable IP blacklist

**Rate Limit Headers:**
```
X-RateLimit-Limit: 100
X-RateLimit-Window: 60s
Retry-After: 60
```

## CORS Support

Configurable CORS support for web clients:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, HEAD, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
Access-Control-Max-Age: 3600
```

## Compression

Automatic gzip compression for supported content types:

- **Trigger**: Client sends `Accept-Encoding: gzip`
- **Content Types**: JSON, text files, HTML
- **Headers**: `Content-Encoding: gzip`, `Vary: Accept-Encoding`

## Caching

Appropriate cache headers for different content types:

- **Metadata Files**: `Cache-Control: public, max-age=3600`
- **Target Files**: `Cache-Control: public, max-age=86400`
- **API Responses**: `Cache-Control: public, max-age=300`
- **Health Endpoint**: `Cache-Control: no-cache`

## WebDAV Support

The server does not support WebDAV methods (PUT, DELETE, PROPFIND, etc.) for security reasons. All write operations should be performed through the repository initialization process.

## Client Integration Examples

## Administrative Endpoints (Authentication Required)

All administrative endpoints require authentication using either API keys or JWT tokens.

### Authentication Management

#### POST /admin/auth/login

Login with username/password to receive a JWT token.

**Request:**
```json
{
  "username": "admin",
  "password": "changeme"
}
```

**Response:**
```json
{
  "success": true,
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "username": "admin",
    "role": "admin",
    "permissions": ["*"]
  }
}
```

#### POST /admin/auth/generate-api-key

Generate a new API key (requires existing authentication).

**Request:**
```json
{
  "name": "CI System",
  "role": "admin",
  "permissions": ["*"],
  "expires_in": "30d"
}
```

#### GET /admin/auth/verify

Verify current authentication status.

### Repository Management

#### POST /admin/repositories/

Create a new repository namespace.

**Request:**
```json
{
  "namespace": "production",
  "name": "firmware",
  "description": "Production firmware repository",
  "config": {
    "public": false,
    "max_file_size": 104857600
  }
}
```

#### GET /admin/repositories/

List all repositories (admin view).

#### GET /admin/repositories/{namespace}/{name}

Get detailed repository information.

#### PUT /admin/repositories/{namespace}/{name}

Update repository configuration.

#### DELETE /admin/repositories/{namespace}/{name}

Delete a repository and all its contents.

### Webhook Management

#### POST /admin/webhooks/

Create a webhook endpoint.

**Request:**
```json
{
  "url": "https://your-service.com/webhook",
  "secret": "webhook-secret",
  "events": ["file.added", "file.updated"],
  "retry_config": {
    "max_attempts": 3,
    "initial_wait": "1s"
  }
}
```

#### GET /admin/webhooks/

List all webhook endpoints.

#### GET /admin/webhooks/{id}

Get webhook details.

#### PUT /admin/webhooks/{id}

Update webhook configuration.

#### DELETE /admin/webhooks/{id}

Delete webhook endpoint.

#### POST /admin/webhooks/{id}/test

Test webhook delivery.

### Target File Management

#### POST /admin/targets/add

Upload a new target file.

**Request:** Multipart form data
- `file`: File to upload
- `path`: Target path (optional)

#### POST /admin/targets/remove

Remove a target file.

**Request:**
```json
{
  "path": "path/to/file.txt"
}
```

### System Administration

#### GET /admin/stats

Get system statistics including cache, uptime, and performance metrics.

#### GET /admin/stats/circuit-breakers

Get circuit breaker status and statistics.

#### GET /admin/stats/coalescing

Get request coalescing statistics.

#### GET /admin/audit/logs

View audit logs for administrative actions.

### curl Examples

#### Public Endpoints (No Authentication Required)

```bash
# Get server health
curl -s http://localhost:8080/health | jq

# Get server status
curl -s http://localhost:8080/api/v1/status | jq

# Download root metadata
curl -s http://localhost:8080/metadata/root.json | jq

# List all target files
curl -s http://localhost:8080/targets/ | jq

# Download a target file
curl -O http://localhost:8080/targets/sample.txt

# Get server metrics
curl -s http://localhost:8080/metrics
```

#### Administrative Examples (With Default API Key)

```bash
# Get server statistics
curl -s -H "X-API-Key: admin-key-example" \
  http://localhost:8080/admin/stats | jq

# Login to get JWT token
JWT_TOKEN=$(curl -s -X POST http://localhost:8080/admin/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "changeme"}' | jq -r '.token')

# Use JWT token for admin operations
curl -s -H "Authorization: Bearer $JWT_TOKEN" \
  http://localhost:8080/admin/stats | jq

# Upload a file using API key
curl -X POST \
  -H "X-API-Key: admin-key-example" \
  -F "file=@myfile.txt" \
  -F "path=uploads/myfile.txt" \
  http://localhost:8080/admin/targets/add

# Create a webhook
curl -X POST \
  -H "X-API-Key: admin-key-example" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://your-webhook.com/tuf-events",
    "secret": "your-webhook-secret",
    "events": ["file.added", "file.updated"]
  }' \
  http://localhost:8080/admin/webhooks/
```

### go-tuf v2 Client

```go
// Initialize client
opts := &client.Options{
    RootURL: "http://localhost:8080",
    CacheDir: "./cache",
}
c, err := client.New(opts)

// Download and verify target
target, err := c.Download("sample.txt")
```

## Development and Testing

### Local Development

```bash
# Start server
make init-repo
go run ./cmd/tuf-server

# Test endpoints
curl http://localhost:8080/health
curl http://localhost:8080/metadata/
curl http://localhost:8080/targets/
```

### Integration Testing

```bash
# Run full integration test
make test-ota

# Manual client test
go run ./cmd/tuf-client --server http://localhost:8080
```