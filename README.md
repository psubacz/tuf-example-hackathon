# TUF Production Project - go-tuf v2 Implementation

This project demonstrates production-grade implementation of The Update Framework (TUF) using the official **go-tuf v2** library with real cryptographic signatures and secure software update distribution.

## What is TUF?

The Update Framework (TUF) is a framework for securing software update systems. It provides:

- **Compromise resilience**: Multiple keys and roles prevent single points of failure
- **Integrity protection**: Cryptographic signatures ensure files haven't been tampered with  
- **Freshness guarantees**: Timestamps prevent rollback attacks
- **Minimized trust**: Separation of concerns across different roles

## 🔐 Production Features

- **Real Ed25519 cryptographic keys** for all TUF roles (root, targets, snapshot, timestamp)
- **Production-grade metadata signing** with the official go-tuf v2 library
- **TUF specification v1.0.31 compliance** using standardized metadata structures
- **Secure HTTP server** with enhanced security features
- **Official go-tuf v2 client** integration with proper validation
- **Container deployment** with security hardening

## Prerequisites

- Go 1.24 or later
- Docker/Podman (for containerized deployment)
- Internet connection for dependencies

## Project Structure

```
tuf-example-hackathon/
├── cmd/                         # Main applications
│   ├── tuf-server/             # Repository initialization & server
│   └── tuf-client/             # go-tuf v2 client
├── internal/                   # Private application code
│   ├── tuf/                   # TUF repository logic
│   │   ├── repository_v2.go   # go-tuf v2 implementation
│   │   └── keys.go           # Cryptographic key management
│   ├── server/                # HTTP server implementation
│   └── logger/                # Logging utilities
├── build/                     # Build and deployment files
│   ├── package/              # Container definitions
│   └── ci/                   # CI/CD configurations
├── charts/                    # Helm charts
├── docs/                      # Comprehensive documentation
│   ├── architecture/         # System architecture docs
│   ├── api/                  # API reference documentation
│   └── deployment.md         # Deployment guides
├── tuf-repository-v2/         # Generated TUF repository
│   ├── metadata/             # TUF metadata files
│   └── targets/             # Target files
├── tuf-client-v2-cache/       # Client cache directory
└── README.md                 # This guide
```

## 🚀 Quick Start

### 1. Setup Dependencies
```bash
make setup
```

### 2. Create TUF Repository
```bash
make init-repo
# Creates repository with real Ed25519 cryptographic keys
```

### 3. Test the Client
```bash
make run-client
```

### 4. Complete Integration Test
```bash
make test-ota
# Tests complete go-tuf v2 workflow
```

## 🔑 Cryptographic Security

The implementation uses production-grade security:

- **Ed25519 keys**: Generated for all TUF roles (root, targets, snapshot, timestamp)
- **Real signatures**: All metadata is cryptographically signed
- **SHA256 hashing**: Target files use genuine hash calculations
- **Signature validation**: Client properly verifies all metadata signatures

Example signed metadata structure:
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
    "expires": "2026-08-15T15:47:45.59277-04:00",
    "keys": { ... },
    "roles": { ... }
  }
}
```

## 🐳 Container Deployment

Run the complete TUF system using Podman containers:

```bash
# Build and run all services
make run-containers

# Stop services
make stop-containers

# Or manually with Podman
cd build/package
podman-compose -f podman-compose.yml up --build
podman-compose -f podman-compose.yml down

# For production deployment
podman-compose -f podman-compose.prod.yml up --build
```

The containerized setup includes:
- **tuf-server**: TUF repository server with go-tuf v2, health checks, and metrics
- **tuf-client**: Production go-tuf v2 client with automatic server dependency
- **Persistent volumes**: For repository data, configuration, logs, and client cache
- **Network isolation**: Dedicated bridge network for secure communication

## 🔍 Available Commands

| Command | Description |
|---------|-------------|
| `make setup` | Download Go dependencies |
| `make init-repo` | Create TUF repository with go-tuf v2 |
| `make run-client` | Run go-tuf v2 client |
| `make test-ota` | Complete integration test |
| `make build-containers` | Build container images |
| `make run-containers` | Run containerized services |
| `make stop-containers` | Stop containerized services |
| `make clean` | Remove generated files |
| `make help` | Show all commands |

## 🛡️ Security Model

TUF provides protection against various attacks:

- **Key compromise**: Multiple keys and thresholds
- **Malicious repositories**: Cryptographic verification
- **Transport attacks**: End-to-end integrity protection  
- **Rollback attacks**: Timestamp and version validation
- **Mix-and-match**: Consistent snapshots
- **Denial of service**: Reasonable expiration times

## 🌍 Production Use Cases

This implementation is suitable for:

- **Software distribution**: Secure application updates
- **IoT firmware updates**: Device software management
- **Container registries**: Secure image distribution
- **Package managers**: Language-specific package distribution
- **Content delivery**: Secure asset distribution
- **Configuration management**: Secure config updates

## 🔧 Configuration

Key configuration options:

```bash
# Server configuration
export TUF_PORT=8080
export TUF_REPOSITORY_PATH=./tuf-repository-v2
export TUF_LOG_LEVEL=info
export TUF_METRICS_ENABLED=true

# Client configuration  
export TUF_SERVER_URL=http://localhost:8080
export TUF_CACHE_DIR=./tuf-client-v2-cache
```

## 📖 Technical Implementation

### Key Generation
- **Ed25519 keys**: Cryptographically secure key generation
- **Key management**: Separate keys for each TUF role
- **Key IDs**: Hex-encoded public key identifiers

### Metadata Signing
- **Sigstore integration**: Uses `github.com/sigstore/sigstore/pkg/signature`
- **Real signatures**: Proper cryptographic signing workflow
- **Verification**: Client validates all signatures

### Target Management
- **Hash calculation**: Real SHA256 hashes for all files
- **Length tracking**: Accurate file size validation
- **Integrity checks**: Complete file verification

## 🚨 Security Considerations

For production deployment:

1. **Key storage**: Use HSMs or secure key management
2. **Key rotation**: Implement regular key updates
3. **Threshold signatures**: Use multiple signers for critical roles
4. **HTTPS only**: Never serve TUF metadata over HTTP in production
5. **Access control**: Restrict repository modification access
6. **Monitoring**: Log all repository operations
7. **Backup**: Secure backup of signing keys

## 📚 Documentation

For comprehensive documentation, see the [docs/](docs/) directory:

- **[Architecture Documentation](docs/architecture/)**: System design, components, and data flow
- **[API Reference](docs/api/endpoints.md)**: Complete REST API documentation
- **[Deployment Guide](docs/deployment.md)**: Production deployment instructions

## References

- [TUF Specification v1.0.31](https://theupdateframework.github.io/specification/latest/)
- [go-tuf v2 Library](https://github.com/theupdateframework/go-tuf/tree/v2)
- [TUF Documentation](https://theupdateframework.io/)
- [Ed25519 Signatures](https://ed25519.cr.yp.to/)
- [Sigstore Project](https://www.sigstore.dev/)

---

**⚠️ Note**: This project demonstrates production TUF implementation patterns. For actual production use, implement proper key management, secure infrastructure, and follow security best practices.
