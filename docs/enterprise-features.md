# Enterprise Features Guide

This guide covers the production-ready enterprise features available in the TUF Repository Server, including webhook systems, circuit breakers, request coalescing, storage backends, and retry logic.

## Overview

The TUF Server includes enterprise-grade features designed for high-availability, scalable production deployments:

- **✅ Webhook System** - Event-driven notifications for repository changes
- **✅ Circuit Breakers** - Automatic failure detection and recovery
- **✅ Request Coalescing** - Duplicate request optimization
- **✅ Multiple Storage Backends** - Filesystem, S3, and S3-compatible storage
- **✅ Retry Logic** - Automatic retry with exponential backoff
- **✅ All Enabled by Default** - Production-ready out of the box

## Webhook System

### Overview

The webhook system provides real-time notifications for repository events, enabling integration with CI/CD pipelines, monitoring systems, and automated workflows.

### Configuration

```json
{
  "webhooks": {
    "enabled": true,
    "max_endpoints": 50,
    "timeout": "30s",
    "retry_attempts": 3,
    "initial_wait": "1s",
    "max_wait": "30s"
  }
}
```

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/admin/webhooks/` | Create webhook endpoint |
| `GET` | `/admin/webhooks/` | List webhook endpoints |
| `GET` | `/admin/webhooks/{id}` | Get webhook details |
| `PUT` | `/admin/webhooks/{id}` | Update webhook |
| `DELETE` | `/admin/webhooks/{id}` | Delete webhook |
| `POST` | `/admin/webhooks/{id}/test` | Test webhook |

### Creating a Webhook

```bash
curl -X POST http://localhost:8080/admin/webhooks/ \
  -H "X-API-Key: admin-key-example" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://your-service.com/webhook",
    "events": [
      "metadata.updated",
      "target.added", 
      "target.removed",
      "repository.created"
    ],
    "headers": {
      "Authorization": "Bearer your-webhook-token",
      "X-Source": "tuf-server"
    },
    "active": true
  }'
```

### Webhook Event Types

| Event | Description | Payload Includes |
|-------|-------------|------------------|
| `metadata.updated` | TUF metadata was updated | repository, role, version |
| `target.added` | New target file added | repository, target_path, size, hash |
| `target.removed` | Target file removed | repository, target_path |
| `repository.created` | New repository created | namespace, name, config |
| `repository.deleted` | Repository deleted | namespace, name |
| `auth.login` | User authentication | username, timestamp, ip |
| `auth.api_key_used` | API key used | key_name, endpoint, ip |

### Webhook Payload Example

```json
{
  "event": "metadata.updated",
  "timestamp": "2025-01-15T10:30:00Z",
  "repository": {
    "namespace": "production",
    "name": "firmware"
  },
  "data": {
    "role": "targets",
    "version": 142,
    "hash": "sha256:abc123...",
    "size": 2048
  },
  "server": {
    "version": "2.0.0",
    "instance": "tuf-server-prod-01"
  }
}
```

### Event Polling (Sidecar Pattern)

For environments where outbound HTTP is restricted:

```bash
# Poll for new events
curl http://localhost:8080/webhooks/poll?since=1642320000

# Get specific event
curl http://localhost:8080/webhooks/events/12345
```

## Circuit Breakers

### Overview

Circuit breakers prevent cascading failures by automatically stopping requests to failing services and allowing them time to recover.

### Configuration

```json
{
  "circuit_breaker": {
    "enabled": true,
    "max_requests": 3,
    "interval": "60s",
    "timeout": "30s", 
    "threshold": 0.5,
    "min_requests": 5
  }
}
```

### Parameters

- **max_requests**: Maximum concurrent requests when half-open
- **interval**: Time to wait before moving from open to half-open
- **timeout**: Request timeout threshold
- **threshold**: Failure rate threshold (0.0-1.0)
- **min_requests**: Minimum requests before calculating failure rate

### Circuit Breaker States

1. **Closed**: Normal operation, requests pass through
2. **Open**: Failing, requests are immediately rejected
3. **Half-Open**: Testing recovery, limited requests allowed

### Monitoring

```bash
# Get circuit breaker statistics
curl -H "X-API-Key: admin-key-example" \
  http://localhost:8080/admin/stats/circuit-breakers
```

Response:
```json
{
  "storage": {
    "state": "closed",
    "requests": 1250,
    "successes": 1248,
    "failures": 2,
    "failure_rate": 0.002,
    "last_failure": "2025-01-15T10:25:00Z"
  },
  "webhook": {
    "state": "half-open",
    "requests": 45,
    "successes": 42,
    "failures": 3,
    "failure_rate": 0.067,
    "last_failure": "2025-01-15T10:28:30Z"
  }
}
```

## Request Coalescing

### Overview

Request coalescing optimizes performance by deduplicating identical concurrent requests, reducing load on storage backends and improving response times.

### Configuration

```json
{
  "coalescing": {
    "enabled": true,
    "ttl": "5s",
    "max_wait": "30s"
  }
}
```

### Parameters

- **ttl**: How long to cache responses for identical requests
- **max_wait**: Maximum time to wait for an ongoing request

### How It Works

1. **First Request**: Processes normally, response cached
2. **Duplicate Requests**: Return cached response or wait for ongoing request
3. **Cache Expiry**: After TTL, requests process normally again

### Monitoring

```bash
# Get coalescing statistics
curl -H "X-API-Key: admin-key-example" \
  http://localhost:8080/admin/stats/coalescing
```

Response:
```json
{
  "total_requests": 5000,
  "coalesced_requests": 1200,
  "cache_hits": 800,
  "cache_misses": 4200,
  "hit_rate": 0.16,
  "average_wait_time": "150ms"
}
```

## Storage Backends

### Overview

Multiple storage backends support different deployment scenarios from local development to cloud-scale production.

### Available Backends

| Backend | Use Case | Features |
|---------|----------|----------|
| `filesystem` | Development, single-node | Local disk storage |
| `s3` | Production, cloud | AWS S3 and S3-compatible |

### Filesystem Backend

**Configuration:**
```json
{
  "storage": {
    "type": "filesystem",
    "properties": {
      "base_path": "./tuf-repository"
    }
  }
}
```

**Features:**
- Fast local access
- Simple backup with filesystem tools
- No external dependencies
- Suitable for single-node deployments

### S3 Backend

**Configuration:**
```json
{
  "storage": {
    "type": "s3",
    "properties": {
      "bucket": "my-tuf-repository",
      "prefix": "tuf/",
      "region": "us-east-1",
      "endpoint": ""
    }
  }
}
```

**S3-Compatible Services:**
```json
{
  "storage": {
    "type": "s3",
    "properties": {
      "bucket": "tuf-repo",
      "prefix": "repositories/",
      "region": "us-east-1",
      "endpoint": "https://minio.example.com",
      "force_path_style": true
    }
  }
}
```

**Features:**
- Scalable cloud storage
- High availability and durability
- Versioning and lifecycle management
- Global distribution via CloudFront
- Works with MinIO, DigitalOcean Spaces, etc.

### Environment Configuration

```bash
# AWS S3 Authentication
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"

# S3-Compatible Services
export AWS_ENDPOINT_URL="https://minio.example.com"
export AWS_S3_FORCE_PATH_STYLE="true"
```

## Retry Logic

### Overview

Automatic retry with exponential backoff improves resilience against transient failures in storage and network operations.

### Configuration

```json
{
  "retry": {
    "enabled": true,
    "max_retries": 3,
    "initial_delay": "100ms",
    "max_delay": "10s",
    "multiplier": 2.0,
    "jitter_fraction": 0.1
  }
}
```

### Parameters

- **max_retries**: Maximum retry attempts
- **initial_delay**: Starting delay between retries
- **max_delay**: Maximum delay between retries
- **multiplier**: Exponential backoff multiplier
- **jitter_fraction**: Random jitter to prevent thundering herd

### Retry Behavior

| Attempt | Base Delay | With Jitter | Total Time |
|---------|------------|-------------|------------|
| 1       | 100ms      | 90-110ms    | ~100ms |
| 2       | 200ms      | 180-220ms   | ~300ms |
| 3       | 400ms      | 360-440ms   | ~700ms |
| 4       | 800ms      | 720-880ms   | ~1.5s |

### Monitoring

Retry statistics are included in storage backend metrics:

```bash
curl -H "X-API-Key: admin-key-example" \
  http://localhost:8080/admin/stats
```

## Production Configuration Examples

### High-Availability Setup

```json
{
  "port": 8080,
  "tls": {
    "enabled": true,
    "cert_file": "/etc/ssl/tuf-server.crt",
    "key_file": "/etc/ssl/tuf-server.key"
  },
  "storage": {
    "type": "s3",
    "properties": {
      "bucket": "production-tuf-repo",
      "prefix": "v1/",
      "region": "us-west-2"
    }
  },
  "auth": {
    "enabled": true,
    "jwt_secret": "production-secret-key-32-chars-min",
    "api_keys": {
      "prod-admin": "Production admin access",
      "ci-service": "CI/CD pipeline access",
      "monitoring": "Monitoring service access"
    }
  },
  "circuit_breaker": {
    "enabled": true,
    "max_requests": 5,
    "interval": "60s",
    "timeout": "30s",
    "threshold": 0.6,
    "min_requests": 10
  },
  "coalescing": {
    "enabled": true,
    "ttl": "10s",
    "max_wait": "30s"
  },
  "retry": {
    "enabled": true,
    "max_retries": 5,
    "initial_delay": "200ms",
    "max_delay": "30s",
    "multiplier": 2.0,
    "jitter_fraction": 0.2
  },
  "rate_limit": {
    "enabled": true,
    "requests": 5000,
    "window": "1h",
    "burst_size": 200
  }
}
```

### Development Setup

```json
{
  "port": 8080,
  "storage": {
    "type": "filesystem",
    "properties": {
      "base_path": "./dev-repository"
    }
  },
  "auth": {
    "enabled": false
  },
  "circuit_breaker": {
    "enabled": false
  },
  "coalescing": {
    "enabled": false
  },
  "retry": {
    "enabled": true,
    "max_retries": 2
  }
}
```

## Monitoring and Observability

### Health Checks

```bash
# Overall health
curl http://localhost:8080/health

# Detailed health with enterprise features
curl -H "X-API-Key: admin-key-example" \
  http://localhost:8080/admin/health/detailed
```

### Metrics

```bash
# Prometheus metrics
curl http://localhost:8080/metrics

# JSON metrics
curl -H "X-API-Key: admin-key-example" \
  http://localhost:8080/admin/stats
```

### Audit Logs

```bash
# View audit logs
curl -H "X-API-Key: admin-key-example" \
  http://localhost:8080/admin/audit/logs?limit=100
```

## Troubleshooting

### Common Issues

1. **Circuit Breaker Stuck Open**:
   ```bash
   # Check circuit breaker stats
   curl -H "X-API-Key: key" http://localhost:8080/admin/stats/circuit-breakers
   
   # Reset circuit breaker (if available)
   curl -X POST -H "X-API-Key: key" \
     http://localhost:8080/admin/circuit-breakers/reset
   ```

2. **High Coalescing Wait Times**:
   - Reduce `max_wait` in configuration
   - Check for slow storage backend
   - Monitor request patterns

3. **Storage Backend Failures**:
   - Verify credentials and permissions
   - Check network connectivity
   - Review retry configuration

4. **Webhook Delivery Failures**:
   - Verify endpoint URL accessibility
   - Check webhook logs
   - Test webhook endpoint manually

### Debug Commands

```bash
# Enable debug logging
export TUF_LOG_LEVEL=debug

# Check feature status
curl -H "X-API-Key: key" http://localhost:8080/admin/stats | jq .server

# View configuration
curl -H "X-API-Key: key" http://localhost:8080/admin/config

# Test storage backend
curl -X POST -H "X-API-Key: key" \
  http://localhost:8080/admin/storage/test
```

## Best Practices

### Production Deployment

1. **Enable All Enterprise Features**:
   - Circuit breakers prevent cascading failures
   - Request coalescing improves performance
   - Retry logic handles transient errors
   - Webhooks enable automation

2. **Configure Appropriate Timeouts**:
   - Circuit breaker timeout: 30-60s
   - Coalescing max wait: 10-30s
   - Retry max delay: 30-60s

3. **Monitor Key Metrics**:
   - Circuit breaker state and failure rates
   - Request coalescing hit rates
   - Storage backend latency and errors
   - Webhook delivery success rates

4. **Set Up Alerting**:
   - Circuit breaker opens
   - High failure rates
   - Storage backend unavailable
   - Webhook delivery failures

### Security Considerations

1. **Webhook Security**:
   - Use HTTPS for webhook endpoints
   - Implement webhook signature verification
   - Rotate webhook tokens regularly

2. **Storage Backend Security**:
   - Use IAM roles instead of access keys
   - Enable storage encryption at rest
   - Implement proper bucket policies

3. **Network Security**:
   - Use TLS for all communications
   - Implement proper firewall rules
   - Consider VPC/private networking

This enterprise feature set provides production-grade reliability, performance, and operational capabilities for your TUF repository server deployment.