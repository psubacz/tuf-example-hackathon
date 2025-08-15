# TUF Example System Architecture

## Overview

This project implements a production-grade TUF (The Update Framework) system using the official go-tuf v2 library. The architecture consists of three main components working together to provide secure software update distribution.

## System Components

### 1. TUF Repository Server

**Locations**: 
- `cmd/tuf-server/`
- `internal/server/`
- `internal/tuf/`

The server component provides secure hosting and distribution of TUF metadata and target files.

#### Core Responsibilities:
- Initialize and manage TUF repository structure
- Generate cryptographic keys for all TUF roles (root, targets, snapshot, timestamp)
- Sign metadata using real Ed25519 cryptographic signatures
- Serve metadata and target files over HTTP/HTTPS
- Provide repository health monitoring and metrics

#### Key Features:
- **Security Middleware**: CSP headers, XSS protection, rate limiting
- **Compression**: Gzip compression for efficient transfers
- **Monitoring**: Real-time metrics, health checks, request logging
- **CORS Support**: Cross-origin resource sharing for web clients
- **TLS Support**: HTTPS with security headers (HSTS, etc.)

### 2. TUF Client
**Location**: `cmd/tuf-client/`

The client component demonstrates secure consumption of TUF repositories using the official go-tuf v2 client library.

#### Core Responsibilities:
- Initialize client with repository metadata
- Verify cryptographic signatures on all metadata
- Download and verify target files
- Maintain local cache of verified metadata
- Detect and prevent rollback attacks

#### Security Features:
- **Signature Verification**: Validates all metadata signatures
- **Freshness Checking**: Prevents replay attacks using timestamps
- **Consistent Snapshots**: Ensures metadata consistency
- **Cache Management**: Maintains verified metadata locally

### 3. Container Infrastructure
**Location**: `build/package/`, `charts/`

Production-ready containerization and orchestration support.

#### Container Services:
- **tuf-server**: Server container with health checks and volumes
- **tuf-client**: Client container with server dependency management
- **Networking**: Isolated bridge network for secure communication
- **Persistence**: Volumes for repository data, logs, and client cache

#### Deployment Options:
- **Podman Compose**: Local development and testing
- **Helm Charts**: Kubernetes production deployment
- **CI/CD**: GitHub Actions for automated builds

## TUF Security Architecture

### Role-Based Key Management

```
Root Role (root.json)
├── Self-signed with root key
├── Defines public keys for all roles
├── Long expiration (1 year)
└── Threshold: 1 signature required

Targets Role (targets.json)
├── Signed by targets key
├── Lists all available target files
├── Includes file hashes and sizes
└── Medium expiration (30 days)

Snapshot Role (snapshot.json)
├── Signed by snapshot key
├── Lists versions of all metadata
├── Prevents mix-and-match attacks
└── Short expiration (1 day)

Timestamp Role (timestamp.json)
├── Signed by timestamp key
├── Indicates latest snapshot version
├── Prevents freeze attacks
└── Very short expiration (1 hour)
```

### Cryptographic Security

- **Key Generation**: Ed25519 keys for all roles
- **Signature Algorithm**: Ed25519 with sigstore integration
- **Hash Algorithm**: SHA256 for file integrity
- **Key Storage**: Private keys in PEM format (production should use HSM)

## Data Flow Architecture

### Repository Initialization Flow

1. **Key Generation**: Generate Ed25519 key pairs for all TUF roles
2. **Directory Setup**: Create repository structure (metadata/, targets/)
3. **Root Metadata**: Create and sign root.json with root key
4. **Target Addition**: Add sample files to targets directory
5. **Targets Metadata**: Create and sign targets.json with targets key
6. **Snapshot Creation**: Create and sign snapshot.json with snapshot key
7. **Timestamp Generation**: Create and sign timestamp.json with timestamp key

### Client Update Flow

1. **Root Bootstrap**: Download and verify root.json
2. **Timestamp Check**: Download and verify timestamp.json
3. **Snapshot Verification**: Download and verify snapshot.json
4. **Targets Metadata**: Download and verify targets.json
5. **File Download**: Download target files with hash verification
6. **Cache Update**: Update local cache with verified metadata

### Server Request Flow

```
Client Request
    ↓
Rate Limiting Middleware
    ↓
Security Headers Middleware
    ↓
Compression Middleware
    ↓
Logging Middleware
    ↓
Route Handler (metadata/targets)
    ↓
File Serving with Content-Type
    ↓
Response with Security Headers
```

## Security Considerations

### Attack Mitigation

- **Key Compromise**: Separate keys for different roles limit blast radius
- **Malicious Repository**: Client verifies all signatures before trust
- **Transport Attacks**: End-to-end cryptographic integrity
- **Rollback Attacks**: Timestamp validation prevents old metadata
- **Mix-and-Match**: Snapshot ensures metadata consistency
- **Freeze Attacks**: Expiration times force regular updates

### Production Hardening

- **Key Management**: Use HSMs or secure key management systems
- **Network Security**: Deploy behind load balancers with TLS termination
- **Access Control**: Restrict repository modification access
- **Monitoring**: Comprehensive logging and alerting
- **Backup**: Secure backup of signing keys and repository data

## Scalability Architecture

### Horizontal Scaling

- **Load Balancing**: Multiple server instances behind load balancer
- **CDN Integration**: Content delivery network for global distribution
- **Database Backend**: Replace file storage with database for metadata
- **Caching**: Redis/Memcached for frequently accessed metadata

### Performance Optimization

- **Compression**: Gzip compression reduces bandwidth
- **Caching Headers**: Appropriate cache TTLs for different file types
- **Connection Pooling**: Efficient HTTP connection management
- **Asynchronous Processing**: Non-blocking I/O for high concurrency

## Integration Points

### CI/CD Integration

- **Automated Builds**: GitHub Actions for container builds
- **Testing**: Automated TUF workflow testing
- **Security Scanning**: Container and dependency vulnerability scanning
- **Deployment**: Automated deployment to staging/production

### Monitoring Integration

- **Metrics**: Prometheus-compatible metrics endpoint
- **Health Checks**: Kubernetes-compatible health endpoints
- **Logging**: Structured JSON logging for log aggregation
- **Alerting**: Integration with monitoring systems (Grafana, etc.)

## Technology Stack

- **Go 1.24**: Primary language with strong cryptography support
- **go-tuf v2**: Official TUF implementation library
- **sigstore**: Cryptographic signature handling
- **Podman/Docker**: Containerization platform
- **Kubernetes/Helm**: Container orchestration
- **GitHub Actions**: CI/CD pipeline
- **nginx**: Reverse proxy and load balancing (optional)