# Multi-Repository Management Guide

This guide explains how to use the TUF Server's multi-repository support to manage multiple isolated TUF repositories with namespace-based organization.

## Overview

The TUF Server supports hosting multiple TUF repositories simultaneously, each with its own namespace and configuration. This enables organizations to:

- Isolate different projects or teams
- Manage separate security domains
- Implement different access controls per repository
- Scale repository management across multiple teams

## Repository Structure

Repositories are organized using a two-level hierarchy:

```
namespace/repository
```

- **Namespace**: Top-level organizational unit (e.g., company, team, project)
- **Repository**: Specific TUF repository within the namespace

## API Endpoints

### Repository Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/repositories` | List all public repositories |
| `POST` | `/admin/repositories/` | Create a new repository (admin) |
| `GET` | `/admin/repositories/` | List all repositories (admin) |
| `GET` | `/admin/repositories/{namespace}/{name}` | Get repository details (admin) |
| `PUT` | `/admin/repositories/{namespace}/{name}` | Update repository (admin) |
| `DELETE` | `/admin/repositories/{namespace}/{name}` | Delete repository (admin) |

### Repository Access

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/repositories/` | Browse public repositories |
| `GET` | `/repositories/{namespace}/{name}` | Get public repository info |
| `GET` | `/api/v1/repositories/{namespace}/{name}/stats` | Repository statistics |

### TUF Content Access

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/metadata/{namespace}/{name}/{role}.json` | Repository-specific metadata |
| `GET` | `/targets/{namespace}/{name}/{path}` | Repository-specific targets |

## Creating Repositories

### Using the Admin API

**Prerequisites**: Authentication must be enabled and you need admin credentials.

1. **Create a Repository**:
```bash
curl -X POST http://localhost:8080/admin/repositories/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "namespace": "myorg",
    "name": "frontend",
    "description": "Frontend application repository",
    "config": {
      "public": true,
      "max_file_size": 104857600,
      "retention_days": 90,
      "allowed_clients": ["*"]
    }
  }'
```

2. **Response**:
```json
{
  "namespace": "myorg",
  "name": "frontend",
  "description": "Frontend application repository",
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-15T10:30:00Z",
  "config": {
    "public": true,
    "max_file_size": 104857600,
    "retention_days": 90,
    "allowed_clients": ["*"]
  }
}
```

### Repository Configuration Options

```json
{
  "public": true,                    // Whether repository is publicly accessible
  "max_file_size": 104857600,        // Maximum file size in bytes (100MB)
  "retention_days": 90,              // How long to keep old versions
  "allowed_clients": ["*"],          // Client access control (* = all)
  "signing_keys": {                  // Repository signing keys
    "root": "key_id_here"
  },
  "webhook_endpoints": [             // Webhook URLs for events
    "https://example.com/webhook"
  ]
}
```

## Using Repositories

### Accessing Repository Content

Once a repository is created, you can access its TUF content using namespace/repository URLs:

1. **Metadata Files**:
```bash
# Root metadata
curl http://localhost:8080/metadata/myorg/frontend/root.json

# Targets metadata  
curl http://localhost:8080/metadata/myorg/frontend/targets.json

# Timestamp metadata
curl http://localhost:8080/metadata/myorg/frontend/timestamp.json

# Snapshot metadata
curl http://localhost:8080/metadata/myorg/frontend/snapshot.json
```

2. **Target Files**:
```bash
# Download a specific target file
curl http://localhost:8080/targets/myorg/frontend/app.exe

# Download from subdirectory
curl http://localhost:8080/targets/myorg/frontend/binaries/v1.0/app.exe
```

### Repository Information

1. **List Public Repositories**:
```bash
curl http://localhost:8080/api/v1/repositories
```

2. **Get Repository Details**:
```bash
curl http://localhost:8080/repositories/myorg/frontend
```

3. **Repository Statistics**:
```bash
curl http://localhost:8080/api/v1/repositories/myorg/frontend/stats
```

## Default Repository Behavior

For backward compatibility, requests without namespace/repository are served from the default repository:

- **Default Namespace**: `default`
- **Default Repository**: `main`

Examples:
```bash
# These are equivalent:
curl http://localhost:8080/metadata/root.json
curl http://localhost:8080/metadata/default/main/root.json

# These are equivalent:
curl http://localhost:8080/targets/myfile.txt
curl http://localhost:8080/targets/default/main/myfile.txt
```

## Client Configuration

### TUF Client Setup

Configure your TUF client to use repository-specific URLs:

```python
# Python TUF client example
from tuf.ngclient import Updater

# For repository: myorg/frontend
metadata_base_url = "http://localhost:8080/metadata/myorg/frontend/"
targets_base_url = "http://localhost:8080/targets/myorg/frontend/"

updater = Updater(
    metadata_dir="./metadata",
    metadata_base_url=metadata_base_url,
    target_base_url=targets_base_url,
    target_dir="./targets"
)
```

### Repository-Specific Root Keys

Each repository should have its own root keys. Download the root metadata for your specific repository:

```bash
# Download root metadata for your repository
curl -o root.json http://localhost:8080/metadata/myorg/frontend/root.json

# Verify and use this root.json with your TUF client
```

## Storage Backend

Repositories are stored in the configured storage backend with the following structure:

```
storage_backend/
├── repositories/
│   ├── default/
│   │   └── main/
│   │       ├── metadata/
│   │       ├── targets/
│   │       └── temp/
│   └── myorg/
│       └── frontend/
│           ├── metadata/
│           ├── targets/
│           └── temp/
```

## Security Considerations

### Access Control

1. **Public Repositories**: Accessible to anyone
2. **Private Repositories**: Require authentication and proper client authorization
3. **Client Allowlists**: Control which clients can access specific repositories

### Namespace Isolation

- Repositories in different namespaces are completely isolated
- No cross-namespace access without explicit configuration
- Namespace-level access controls can be implemented

### Best Practices

1. **Use Meaningful Namespaces**: Organize by team, project, or environment
2. **Configure Appropriate Access**: Don't make sensitive repositories public
3. **Regular Key Rotation**: Implement key rotation policies per repository
4. **Monitor Access**: Use audit logs to track repository access
5. **Backup Strategy**: Implement repository-specific backup procedures

## Troubleshooting

### Common Issues

1. **Repository Not Found**: Verify namespace and repository name spelling
2. **Access Denied**: Check if repository is public or if you have proper authentication
3. **File Not Found**: Ensure files exist in the specific repository path

### Debug Commands

```bash
# List all repositories
curl http://localhost:8080/api/v1/repositories

# Check repository health
curl http://localhost:8080/health

# View server logs for repository operations
tail -f server.log | grep repository
```

## Migration from Single Repository

To migrate from a single repository setup to multi-repository:

1. **Backup Existing Repository**: Save your current repository data
2. **Create Target Repository**: Use the API to create your new repository
3. **Copy Content**: Move metadata and targets to the new repository structure
4. **Update Clients**: Reconfigure clients to use the new repository URLs
5. **Test Thoroughly**: Verify all operations work with the new setup

## Examples

### Complete Workflow

```bash
# 1. Create a new repository
curl -X POST http://localhost:8080/admin/repositories/ \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "acme",
    "name": "webapp",
    "config": {
      "public": true,
      "max_file_size": 52428800
    }
  }'

# 2. Check repository was created
curl http://localhost:8080/repositories/acme/webapp

# 3. Access repository metadata
curl http://localhost:8080/metadata/acme/webapp/root.json

# 4. Upload targets (would require additional tooling)
# 5. Configure TUF client to use acme/webapp repository
```

This multi-repository support enables scalable, secure management of multiple TUF repositories within a single server instance.