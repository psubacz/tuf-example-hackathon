# TUF Edge Container Architecture Documentation

This documentation describes a comprehensive architecture for secure file distribution to edge containers using The Update Framework (TUF).

## 📋 Documentation Overview

### Architecture Documents

| Document | Description | Audience |
|----------|-------------|----------|
| [Edge Deployment](./architecture/edge-deployment.md) | Main architecture overview and system components | Architects, DevOps Engineers |
| [Security Model](./architecture/security-model.md) | Comprehensive security design and threat model | Security Engineers, Compliance Teams |
| [Deployment Patterns](./architecture/deployment-patterns.md) | Various deployment patterns for different platforms | DevOps Engineers, Platform Teams |

### Diagrams

| Diagram | Description | Format |
|---------|-------------|--------|
| [System Overview](./diagrams/system-overview.mmd) | High-level system architecture | Mermaid |
| [Edge Container Flow](./diagrams/edge-container-flow.mmd) | Detailed container initialization sequence | Mermaid |
| [Security Architecture](./diagrams/security-architecture.mmd) | Security layers and threat mitigation | Mermaid |

### Examples

| Example | Description | Technology |
|---------|-------------|------------|
| [Kubernetes Deployment](./examples/kubernetes/edge-deployment.yaml) | Production-ready Kubernetes deployment | Kubernetes |
| [Docker Compose Stack](./examples/docker-compose/edge-stack.yml) | Complete development environment | Docker Compose |
| [Standalone Examples](./examples/standalone/) | Standalone deployment scripts | Shell/Docker |

## 🏗️ Architecture Summary

### Core Concept

The architecture implements a **secure init container pattern** where:

1. **Edge containers** start with a TUF init container
2. **Init container** downloads and verifies files from TUF repository
3. **Files are shared** with the main application container via volumes
4. **Main application** uses pre-verified, tamper-proof files

### Key Benefits

- ✅ **Cryptographic Security**: All files cryptographically verified using TUF
- ✅ **Supply Chain Protection**: Prevents malicious file injection
- ✅ **Rollback Protection**: Timestamps prevent downgrade attacks  
- ✅ **Scalable Distribution**: Support for thousands of edge locations
- ✅ **Platform Agnostic**: Works with Kubernetes, Docker, standalone

## 🔧 Quick Start

### Prerequisites

- Docker and Docker Compose
- Kubernetes cluster (for K8s examples)
- Go 1.24+ (for building custom components)

### 1. Build the Edge Init Container

```bash
# Build the edge init container
docker build -f build/package/Dockerfile.edge-init -t tuf-edge-init:latest .
```

### 2. Run Development Stack

```bash
# Start the complete development stack
cd docs/examples/docker-compose
docker-compose -f edge-stack.yml up --build
```

### 3. Deploy to Kubernetes

```bash
# Deploy to Kubernetes
kubectl apply -f docs/examples/kubernetes/edge-deployment.yaml
```

## 🎯 Use Cases

### 1. IoT Edge Devices
- **Scenario**: IoT devices need secure configuration and firmware updates
- **Pattern**: IoT Device Pattern with offline capabilities
- **Benefits**: Bandwidth efficiency, intermittent connectivity support

### 2. Edge Computing Nodes  
- **Scenario**: Edge computing clusters running multiple applications
- **Pattern**: Edge Computing Node Pattern with shared TUF service
- **Benefits**: High performance, dynamic workload scheduling

### 3. CDN Edge Servers
- **Scenario**: Content delivery network requiring frequent content updates
- **Pattern**: CDN Edge Pattern with parallel downloads
- **Benefits**: High throughput, global distribution

### 4. Air-Gapped Environments
- **Scenario**: Secure facilities with no external network access
- **Pattern**: Air-Gapped Pattern with offline repositories
- **Benefits**: Maximum security, compliance requirements

## 🔐 Security Features

### Cryptographic Verification
- **Ed25519/RSA signatures** on all metadata and files
- **SHA-256 hash verification** for file integrity
- **Threshold signatures** to prevent single points of failure

### Defense in Depth
- **TLS 1.3** with certificate pinning for transport security
- **Container security** with read-only filesystems and non-root users
- **Network policies** for micro-segmentation
- **Audit logging** for compliance and forensics

### Key Management
- **Offline root keys** stored in HSMs
- **Automatic key rotation** for online keys
- **Delegated keys** for regional operations
- **Emergency key recovery** procedures

## 📊 Performance Characteristics

### Scalability
- **Thousands of edge locations** supported
- **Parallel file downloads** for large file sets
- **CDN integration** for global distribution
- **Caching strategies** to minimize bandwidth

### Efficiency
- **Incremental updates** to minimize data transfer
- **Delta synchronization** for large files
- **Compression support** for bandwidth-constrained environments
- **Intelligent caching** with LRU eviction

## 🔍 Monitoring and Observability

### Metrics Collection
```bash
# Key metrics tracked
signature_verifications_total{status="success|failure"}
file_downloads_total{result="success|failure"}
init_container_duration_seconds
cache_hit_ratio
```

### Logging Strategy
```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "level": "info",
  "component": "tuf-edge-init",
  "event": "file_verified",
  "edge_location": "us-west-2-edge-01",
  "file": "config.json",
  "duration_ms": 1250
}
```

### Health Checks
- **Init container health**: File verification completion
- **Application readiness**: Verified files available
- **TUF server health**: Repository accessibility

## 🚀 Deployment Options

### Kubernetes (Recommended)
- **Production-ready** with full security controls
- **Native orchestration** with init container pattern
- **Comprehensive monitoring** with Prometheus integration
- **Network policies** for security isolation

### Docker Compose
- **Development environments** and testing
- **Simple orchestration** for smaller deployments
- **Easy debugging** and troubleshooting
- **Local development** workflows

### Standalone
- **Legacy environments** without orchestration
- **Custom deployment** scenarios
- **Embedded systems** with constraints
- **Offline environments** with local repositories

## 📝 Configuration Examples

### Basic Edge Init Configuration
```yaml
environment:
  - TUF_SERVER_URL=https://tuf.example.com
  - TUF_TARGET_DIR=/shared
  - TUF_CACHE_DIR=/cache
  - TUF_LOG_LEVEL=info
  - TUF_VERIFY_ALL=true
```

### Security-Hardened Configuration
```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65534
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
```

### Performance-Optimized Configuration
```yaml
environment:
  - TUF_PARALLEL_DOWNLOADS=10
  - TUF_CACHE_SIZE=1GB
  - TUF_COMPRESSION=true
  - TUF_BANDWIDTH_LIMIT=100MB/s
```

## 🔧 Customization

### Building Custom Init Containers
```bash
# Extend the base init container
FROM tuf-edge-init:latest
COPY custom-config.json /config/
COPY custom-scripts/ /scripts/
RUN chmod +x /scripts/*
```

### Custom Target Patterns
```bash
# Download specific file types
edge-init --targets="*.json,*.yaml,config.*,models/*.tgz"

# Verify all available targets
edge-init --verify-all --one-shot
```

### Integration with Existing Applications
```go
// Check for TUF completion before starting app
func waitForTUFInit() error {
    for i := 0; i < 60; i++ {
        if _, err := os.Stat("/shared/.tuf-init-complete"); err == nil {
            return nil
        }
        time.Sleep(5 * time.Second)
    }
    return errors.New("TUF initialization timeout")
}
```

## 🐛 Troubleshooting

### Common Issues

**Init Container Fails to Start**
```bash
# Check logs
kubectl logs <pod-name> -c tuf-init

# Verify TUF server accessibility
curl -v https://tuf.example.com/health
```

**File Verification Failures**
```bash
# Check signature verification
edge-init --debug --verify-signatures

# Verify root keys
cat /config/root.json | jq .keys
```

**Performance Issues**
```bash
# Monitor download times
kubectl top pods --containers

# Check network connectivity
traceroute tuf.example.com
```

## 📚 Additional Resources

- [TUF Specification](https://theupdateframework.github.io/specification/latest/)
- [Container Security Best Practices](https://kubernetes.io/docs/concepts/security/)
- [Kubernetes Init Containers](https://kubernetes.io/docs/concepts/workloads/pods/init-containers/)
- [Docker Multi-stage Builds](https://docs.docker.com/develop/dev-best-practices/dockerfile_best-practices/)

## 🤝 Contributing

Contributions to the architecture documentation and examples are welcome:

1. **Architecture Improvements**: Propose enhancements to security or performance
2. **Deployment Examples**: Add examples for new platforms or use cases
3. **Documentation**: Improve clarity and completeness of documentation
4. **Testing**: Add test scenarios and validation procedures

## 📄 License

This architecture documentation is provided under the same license as the main TUF project.

---

**Built with ❤️ for secure edge computing**