# TUF Golang Project - Educational Demo

This is a simplified demonstration of The Update Framework (TUF) concepts using Go. This project shows how to:

1. Create TUF repository metadata structure 
2. Demonstrate TUF roles and their relationships
3. Show secure file verification workflow
4. Understand TUF security guarantees

> **Note**: This is an educational demonstration using basic Go structures. For production use, you should use the official [go-tuf v2 library](https://github.com/theupdateframework/go-tuf).

## What is TUF?

The Update Framework (TUF) is a framework for securing software update systems. It provides:

- **Compromise resilience**: Multiple keys and roles prevent single points of failure
- **Integrity protection**: Cryptographic signatures ensure files haven't been tampered with
- **Freshness guarantees**: Timestamps prevent rollback attacks
- **Minimized trust**: Separation of concerns across different roles

## Project Structure

```
tuf-golang-project/
├── go.mod              # Go module definition with TUF v2 dependencies
├── main.go             # Repository setup using TUF v2 metadata API
├── client.go           # Client example using TUF v2 updater package
├── add_targets.go      # Script to add additional targets
├── README.md           # This file
├── Makefile            # Build and run commands
├── tuf-repository/     # TUF repository (created when running main.go)
│   ├── metadata/       # TUF metadata files
│   └── targets/        # Target files
└── client-cache/       # Client cache directory (created when running client.go)
```

## Prerequisites

- Go 1.21 or later
- Internet connection for downloading dependencies

## Getting Started

1. **Initialize the project:**
   ```bash
   cd tuf-golang-project
   # No dependencies needed - uses Go standard library only!
   ```

2. **Create the TUF repository:**
   ```bash
   go run main.go
   ```
   This will:
   - Create TUF metadata structure (root, targets, snapshot, timestamp)
   - Generate a sample target file
   - Demonstrate TUF role relationships
   - Save metadata in JSON format

3. **Test the TUF client:**
   ```bash
   go run client.go
   ```
   This will:
   - Load and parse TUF metadata
   - Demonstrate secure file verification workflow
   - Show integrity checking process
   - Explain TUF security benefits

4. **Add more targets (optional):**
   ```bash
   go run add_targets.go
   ```
   This adds more example files and updates the targets metadata.

## TUF Roles Explained

- **Root**: The root of trust, signs other role metadata
- **Targets**: Defines which files are available and their metadata
- **Snapshot**: References specific versions of targets and other metadata
- **Timestamp**: Provides freshness guarantees for the snapshot

## Key Features Demonstrated

1. **TUF Metadata Structure**: Shows all four TUF roles and their relationships
2. **File Integrity**: Demonstrates how TUF tracks file sizes and hashes
3. **Version Management**: Shows how metadata versions prevent rollback attacks
4. **Security Model**: Explains the multi-role security architecture
5. **Verification Workflow**: Step-by-step client verification process

## Security Benefits

- **Integrity**: All files are cryptographically signed
- **Authenticity**: Signatures verify the source of files
- **Freshness**: Timestamps prevent replay attacks
- **Availability**: Multiple signature thresholds provide resilience

## Next Steps

To extend this project, you could:

1. Add multiple target files
2. Implement key rotation
3. Set up a web server to host the repository
4. Add delegation for distributed signing
5. Implement custom metadata for application-specific needs

## References

- [TUF Specification](https://theupdateframework.github.io/specification/latest/)
- [Go-TUF Library](https://github.com/theupdateframework/go-tuf)
- [TUF Documentation](https://theupdateframework.io/)
