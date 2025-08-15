# TUF Deployment Guide

## Overview

This guide covers deploying the TUF system in various environments, from local development to production Kubernetes clusters.

## Local Development Deployment

### Prerequisites

- Go 1.24+
- Podman or Docker
- Make

### Quick Start

```bash
# Clone repository
git clone <repository-url>
cd tuf-example-hackathon

# Setup dependencies
make setup

# Initialize TUF repository
make init-repo

# Start server
go run ./cmd/tuf-server

# Test client (in another terminal)
make run-client
```

### Environment Variables

```bash
# Server Configuration
export TUF_PORT=8080
export TUF_HOST=0.0.0.0
export TUF_REPOSITORY_PATH=./tuf-repository-v2
export TUF_LOG_LEVEL=info
export TUF_METRICS_ENABLED=true

# TLS Configuration (optional)
export TUF_TLS_ENABLED=false
export TUF_TLS_CERT_FILE=/path/to/cert.pem
export TUF_TLS_KEY_FILE=/path/to/key.pem

# Client Configuration
export TUF_SERVER_URL=http://localhost:8080
export TUF_CACHE_DIR=./tuf-client-v2-cache
```

## Container Deployment

### Podman Compose (Recommended)

```bash
# Build and start services
make build-containers
make run-containers

# View logs
cd build/package
podman-compose logs -f

# Stop services
make stop-containers
```

### Manual Container Commands

```bash
# Build images
cd build/package
podman build -f Dockerfile.tuf-server -t tuf-server:latest ../../
podman build -f Dockerfile.tuf-client -t tuf-client:latest ../../

# Run server
podman run -d \
  --name tuf-server \
  -p 8080:8080 \
  -v tuf-repository:/app/tuf-repository-v2 \
  -e TUF_PORT=8080 \
  tuf-server:latest

# Run client
podman run --rm \
  --name tuf-client \
  --link tuf-server \
  -e TUF_SERVER_URL=http://tuf-server:8080 \
  tuf-client:latest
```

## Kubernetes Deployment

### Using Helm Charts

```bash
# Install Helm chart
helm install tuf-server ./charts/tuf-server \
  --set image.tag=latest \
  --set service.type=LoadBalancer \
  --set persistence.enabled=true

# Check status
kubectl get pods -l app.kubernetes.io/name=tuf-server

# Get service URL
kubectl get svc tuf-server
```

### Manual Kubernetes Deployment

```yaml
# tuf-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tuf-server
spec:
  replicas: 3
  selector:
    matchLabels:
      app: tuf-server
  template:
    metadata:
      labels:
        app: tuf-server
    spec:
      containers:
      - name: tuf-server
        image: tuf-server:latest
        ports:
        - containerPort: 8080
        env:
        - name: TUF_PORT
          value: "8080"
        - name: TUF_REPOSITORY_PATH
          value: "/app/tuf-repository-v2"
        volumeMounts:
        - name: tuf-repository
          mountPath: /app/tuf-repository-v2
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tuf-repository
        persistentVolumeClaim:
          claimName: tuf-repository-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: tuf-server
spec:
  selector:
    app: tuf-server
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: tuf-repository-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

```bash
# Deploy to Kubernetes
kubectl apply -f tuf-deployment.yaml

# Check deployment
kubectl get deployments
kubectl get pods
kubectl get services
```

## Production Deployment Considerations

### Security Configuration

#### TLS/HTTPS Setup

```bash
# Generate certificates (or use existing)
openssl req -x509 -newkey rsa:4096 -keyout tuf-server-key.pem -out tuf-server-cert.pem -days 365 -nodes

# Configure TLS
export TUF_TLS_ENABLED=true
export TUF_TLS_CERT_FILE=/etc/ssl/certs/tuf-server-cert.pem
export TUF_TLS_KEY_FILE=/etc/ssl/private/tuf-server-key.pem
```

#### Rate Limiting Configuration

```bash
# Rate limiting environment variables
export TUF_RATE_LIMIT_ENABLED=true
export TUF_RATE_LIMIT_REQUESTS=100
export TUF_RATE_LIMIT_WINDOW=60s
export TUF_RATE_LIMIT_BURST=10
```

#### Security Headers

Production deployments automatically include:
- Content Security Policy (CSP)
- HTTP Strict Transport Security (HSTS)
- X-Frame-Options: DENY
- X-Content-Type-Options: nosniff
- X-XSS-Protection: 1; mode=block

### Key Management

#### Production Key Storage

**DO NOT use the default key generation in production!**

```bash
# Use Hardware Security Module (HSM)
# Configure with PKCS#11 or similar

# Or use cloud key management
# AWS KMS, Azure Key Vault, Google Cloud KMS

# Secure key storage options:
# - Hardware Security Modules (HSM)
# - Cloud key management services
# - Encrypted key storage with proper access controls
```

#### Key Rotation Strategy

```bash
# Plan for regular key rotation
# - Root keys: Annually or when compromised
# - Targets keys: Every 6 months
# - Snapshot keys: Every 3 months
# - Timestamp keys: Every month

# Implement automated key rotation
# Use tools like cert-manager for certificate rotation
```

### High Availability Setup

#### Load Balancer Configuration

```nginx
# nginx.conf example
upstream tuf_backend {
    server tuf-server-1:8080;
    server tuf-server-2:8080;
    server tuf-server-3:8080;
}

server {
    listen 443 ssl http2;
    server_name tuf.example.com;

    ssl_certificate /etc/ssl/certs/tuf.crt;
    ssl_certificate_key /etc/ssl/private/tuf.key;

    location / {
        proxy_pass http://tuf_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

#### Database Backend (Optional)

For high-scale deployments, consider database-backed metadata storage:

```yaml
# Database configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: tuf-config
data:
  config.yaml: |
    database:
      type: postgresql
      host: postgres.example.com
      port: 5432
      database: tuf_metadata
      user: tuf_user
      password_secret: tuf-db-password
    storage:
      type: s3
      bucket: tuf-targets
      region: us-west-2
```

### Monitoring and Observability

#### Prometheus Monitoring

```yaml
# prometheus-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
    scrape_configs:
    - job_name: 'tuf-server'
      static_configs:
      - targets: ['tuf-server:8080']
      metrics_path: /metrics
```

#### Grafana Dashboard

```json
{
  "dashboard": {
    "title": "TUF Server Metrics",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(tuf_requests_total[5m])"
          }
        ]
      },
      {
        "title": "Response Time",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(tuf_request_duration_seconds_bucket[5m]))"
          }
        ]
      }
    ]
  }
}
```

#### Log Aggregation

```yaml
# fluentd-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluentd-config
data:
  fluent.conf: |
    <source>
      @type tail
      path /var/log/containers/tuf-server*.log
      pos_file /var/log/fluentd-tuf.log.pos
      tag tuf.server
      format json
    </source>

    <match tuf.**>
      @type elasticsearch
      host elasticsearch.logging.svc.cluster.local
      port 9200
      index_name tuf-logs
    </match>
```

### Backup and Disaster Recovery

#### Repository Backup

```bash
#!/bin/bash
# backup-tuf-repository.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backups/tuf-repository-$DATE"

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup repository files
cp -r ./tuf-repository-v2 "$BACKUP_DIR/"

# Backup keys (if stored locally - NOT recommended for production)
# cp -r ./keys "$BACKUP_DIR/"

# Create tarball
tar -czf "$BACKUP_DIR.tar.gz" "$BACKUP_DIR"

# Upload to cloud storage
aws s3 cp "$BACKUP_DIR.tar.gz" s3://tuf-backups/

# Cleanup local backup
rm -rf "$BACKUP_DIR"
```

#### Disaster Recovery Plan

1. **Repository Corruption**: Restore from latest backup
2. **Key Compromise**: Revoke compromised keys, generate new keys
3. **Server Compromise**: Rebuild infrastructure, restore from backup
4. **Data Center Outage**: Failover to secondary region

### Performance Tuning

#### Server Optimization

```bash
# Increase file descriptor limits
ulimit -n 65536

# Tune Go runtime
export GOMAXPROCS=4
export GOGC=100

# Configure connection limits
export TUF_MAX_CONNECTIONS=1000
export TUF_READ_TIMEOUT=30s
export TUF_WRITE_TIMEOUT=30s
```

#### CDN Configuration

```yaml
# CloudFront configuration example
apiVersion: v1
kind: ConfigMap
metadata:
  name: cdn-config
data:
  cloudfront.json: |
    {
      "Origins": [
        {
          "DomainName": "tuf-origin.example.com",
          "Id": "tuf-origin",
          "CustomOriginConfig": {
            "HTTPPort": 443,
            "OriginProtocolPolicy": "https-only"
          }
        }
      ],
      "DefaultCacheBehavior": {
        "TargetOriginId": "tuf-origin",
        "TrustedSigners": ["self"],
        "ViewerProtocolPolicy": "redirect-to-https",
        "CachePolicyId": "tuf-cache-policy"
      }
    }
```

## Environment-Specific Configurations

### Development Environment

```yaml
# docker-compose.dev.yml
version: '3.8'
services:
  tuf-server:
    build:
      context: .
      dockerfile: build/package/Dockerfile.tuf-server
    ports:
      - "8080:8080"
    environment:
      - TUF_LOG_LEVEL=debug
      - TUF_METRICS_ENABLED=true
    volumes:
      - ./tuf-repository-v2:/app/tuf-repository-v2
    restart: unless-stopped
```

### Staging Environment

```yaml
# docker-compose.staging.yml
version: '3.8'
services:
  tuf-server:
    image: tuf-server:staging
    ports:
      - "443:8080"
    environment:
      - TUF_TLS_ENABLED=true
      - TUF_LOG_LEVEL=info
      - TUF_RATE_LIMIT_ENABLED=true
    volumes:
      - tuf-repository:/app/tuf-repository-v2
      - ./certs:/etc/ssl/certs
    restart: always
```

### Production Environment

```yaml
# docker-compose.prod.yml
version: '3.8'
services:
  tuf-server:
    image: tuf-server:v1.0.0
    ports:
      - "443:8080"
    environment:
      - TUF_TLS_ENABLED=true
      - TUF_LOG_LEVEL=warn
      - TUF_RATE_LIMIT_ENABLED=true
      - TUF_METRICS_ENABLED=true
    volumes:
      - tuf-repository:/app/tuf-repository-v2
      - ./certs:/etc/ssl/certs
    restart: always
    deploy:
      replicas: 3
      resources:
        limits:
          memory: 512M
          cpus: '0.5'
```

## Troubleshooting

### Common Issues

#### Server Won't Start

```bash
# Check port availability
netstat -tulpn | grep :8080

# Check file permissions
ls -la ./tuf-repository-v2

# Check logs
docker logs tuf-server
```

#### Certificate Issues

```bash
# Verify certificate
openssl x509 -in cert.pem -text -noout

# Check certificate chain
openssl verify -CAfile ca.pem cert.pem

# Test TLS connection
openssl s_client -connect localhost:8080 -servername localhost
```

#### Performance Issues

```bash
# Monitor resource usage
top -p $(pgrep tuf-server)

# Check network connections
netstat -an | grep :8080

# Analyze logs for bottlenecks
tail -f /var/log/tuf-server.log | grep "slow request"
```

### Health Checks

```bash
# Server health
curl -f http://localhost:8080/health

# Repository verification
curl -s http://localhost:8080/info | jq '.repository'

# Metrics check
curl -s http://localhost:8080/metrics | grep tuf_requests_total
```