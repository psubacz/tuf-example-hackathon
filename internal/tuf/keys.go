package tuf

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

// KeyPair represents a cryptographic key pair
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
	KeyID      string
	tufKey     *metadata.Key // Store the go-tuf v2 key object
}

// GenerateED25519Key generates a new Ed25519 key pair
func GenerateED25519Key() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
	}

	// Use go-tuf v2's key ID generation to ensure consistency
	tufKey, err := metadata.KeyFromPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUF key: %w", err)
	}

	return &KeyPair{
		PublicKey:  pub,
		PrivateKey: priv,
		KeyID:      tufKey.ID(), // Use the key ID from go-tuf v2
		tufKey:     tufKey,           // Store the TUF key for later use
	}, nil
}

// ToTUFKey converts the key pair to a TUF metadata Key
func (kp *KeyPair) ToTUFKey() *metadata.Key {
	// Return the properly generated go-tuf v2 key
	return kp.tufKey
}

// GetKeyID returns the key identifier
func (kp *KeyPair) GetKeyID() string {
	return kp.KeyID
}

// KeyManager manages TUF keys for different roles
type KeyManager struct {
	rootKey      *KeyPair
	targetsKey   *KeyPair
	snapshotKey  *KeyPair
	timestampKey *KeyPair
}

// NewKeyManager creates a new key manager with generated keys
func NewKeyManager() (*KeyManager, error) {
	rootKey, err := GenerateED25519Key()
	if err != nil {
		return nil, fmt.Errorf("failed to generate root key: %w", err)
	}

	targetsKey, err := GenerateED25519Key()
	if err != nil {
		return nil, fmt.Errorf("failed to generate targets key: %w", err)
	}

	snapshotKey, err := GenerateED25519Key()
	if err != nil {
		return nil, fmt.Errorf("failed to generate snapshot key: %w", err)
	}

	timestampKey, err := GenerateED25519Key()
	if err != nil {
		return nil, fmt.Errorf("failed to generate timestamp key: %w", err)
	}

	return &KeyManager{
		rootKey:      rootKey,
		targetsKey:   targetsKey,
		snapshotKey:  snapshotKey,
		timestampKey: timestampKey,
	}, nil
}

// GetRootKey returns the root key pair
func (km *KeyManager) GetRootKey() *KeyPair {
	return km.rootKey
}

// GetTargetsKey returns the targets key pair
func (km *KeyManager) GetTargetsKey() *KeyPair {
	return km.targetsKey
}

// GetSnapshotKey returns the snapshot key pair
func (km *KeyManager) GetSnapshotKey() *KeyPair {
	return km.snapshotKey
}

// GetTimestampKey returns the timestamp key pair
func (km *KeyManager) GetTimestampKey() *KeyPair {
	return km.timestampKey
}

// GetKeys returns all keys as a map for TUF metadata
func (km *KeyManager) GetKeys() map[string]*metadata.Key {
	return map[string]*metadata.Key{
		km.rootKey.GetKeyID():      km.rootKey.ToTUFKey(),
		km.targetsKey.GetKeyID():   km.targetsKey.ToTUFKey(),
		km.snapshotKey.GetKeyID():  km.snapshotKey.ToTUFKey(),
		km.timestampKey.GetKeyID(): km.timestampKey.ToTUFKey(),
	}
}

// GetRoles returns role definitions for TUF metadata
func (km *KeyManager) GetRoles() map[string]*metadata.Role {
	return map[string]*metadata.Role{
		"root": {
			KeyIDs:    []string{km.rootKey.GetKeyID()},
			Threshold: 1,
		},
		"targets": {
			KeyIDs:    []string{km.targetsKey.GetKeyID()},
			Threshold: 1,
		},
		"snapshot": {
			KeyIDs:    []string{km.snapshotKey.GetKeyID()},
			Threshold: 1,
		},
		"timestamp": {
			KeyIDs:    []string{km.timestampKey.GetKeyID()},
			Threshold: 1,
		},
	}
}

// GetKeyPairs returns all key pairs mapped by role name
func (km *KeyManager) GetKeyPairs() map[string]*KeyPair {
	return map[string]*KeyPair{
		"root":      km.rootKey,
		"targets":   km.targetsKey,
		"snapshot":  km.snapshotKey,
		"timestamp": km.timestampKey,
	}
}