# Security Enhancement Roadmap for Credential Protection

## Overview

While dddns's current credential encryption (AES-256-GCM with device-specific keys) provides adequate security for its primary use case, this document outlines future enhancements to strengthen credential protection further. These improvements are designed to future-proof the security model without compromising usability.

## Current Security Model Assessment

### Threat Model Context

The current security implementation is designed for:
- **Primary Defense**: Prevent plaintext credential storage
- **Device Binding**: Credentials unusable if config files are copied
- **Casual Protection**: Deter opportunistic credential harvesting

It assumes:
- Physical device (router/server) is reasonably secured
- SSH access is properly configured with key-based auth
- If an attacker has root access, the system is already compromised

### Current Implementation Strengths

1. **AES-256-GCM**: Military-grade encryption with authentication
2. **Device-Specific Keys**: Hardware-bound encryption
3. **Proper File Permissions**: 0400 for encrypted configs
4. **No External Dependencies**: Pure Go crypto libraries
5. **Zero Configuration**: Works out of the box

## Enhanced Security Recommendations

### Priority 1: Improved Key Derivation Function (KDF)

**Current**: Simple SHA-256 hash of device ID + salt
**Enhancement**: Use PBKDF2 or Argon2 for key stretching

```go
// internal/crypto/kdf.go
package crypto

import (
    "crypto/sha256"
    "golang.org/x/crypto/pbkdf2"
    "golang.org/x/crypto/argon2"
)

// Enhanced key derivation with PBKDF2
func DeriveKeyPBKDF2(deviceID string, salt []byte, iterations int) []byte {
    return pbkdf2.Key([]byte(deviceID), salt, iterations, 32, sha256.New)
}

// Modern key derivation with Argon2id (memory-hard)
func DeriveKeyArgon2(deviceID string, salt []byte) []byte {
    return argon2.IDKey([]byte(deviceID), salt,
        1,      // time cost
        64*1024, // memory cost (64MB)
        4,       // parallelism
        32)      // key length
}
```

**Benefits**:
- Increases computational cost for brute force attacks
- Argon2 is memory-hard, resistant to GPU/ASIC attacks
- Industry best practice for password-based encryption

**Implementation Priority**: HIGH
**Complexity**: LOW
**Breaking Change**: NO (can migrate existing)

### Priority 2: Optional User Passphrase

**Enhancement**: Allow users to add an additional passphrase for extra security

```yaml
# config.yaml
security:
  require_passphrase: true  # Optional feature
  passphrase_hint: "Your memorable phrase"  # Optional hint
```

```go
// Enhanced encryption with optional passphrase
func EncryptWithPassphrase(data []byte, passphrase string) (string, error) {
    deviceKey := GetDeviceKey()

    var finalKey []byte
    if passphrase != "" {
        // Combine device key with user passphrase
        combined := append(deviceKey, []byte(passphrase)...)
        finalKey = DeriveKeyArgon2(string(combined), salt)
    } else {
        finalKey = deviceKey
    }

    return encryptAESGCM(data, finalKey)
}
```

**Benefits**:
- Two-factor encryption (something you have + something you know)
- User-controlled security level
- Optional - doesn't break existing deployments

**Implementation Priority**: MEDIUM
**Complexity**: MEDIUM
**Breaking Change**: NO (opt-in feature)

### Priority 3: Secure Random Salt Generation

**Current**: Hardcoded salt `"dddns-vault-2025"`
**Enhancement**: Generate and store unique salt per installation

```go
// internal/crypto/salt.go
package crypto

import (
    "crypto/rand"
    "encoding/base64"
    "os"
    "path/filepath"
)

const saltFile = ".salt"

func GetOrCreateSalt(configDir string) ([]byte, error) {
    saltPath := filepath.Join(configDir, saltFile)

    // Try to read existing salt
    if data, err := os.ReadFile(saltPath); err == nil {
        return base64.StdEncoding.DecodeString(string(data))
    }

    // Generate new salt
    salt := make([]byte, 32)
    if _, err := rand.Read(salt); err != nil {
        return nil, err
    }

    // Store salt (with restricted permissions)
    encoded := base64.StdEncoding.EncodeToString(salt)
    if err := os.WriteFile(saltPath, []byte(encoded), 0400); err != nil {
        return nil, err
    }

    return salt, nil
}
```

**Benefits**:
- Unique salt per installation
- Prevents rainbow table attacks
- No hardcoded values in binary

**Implementation Priority**: HIGH
**Complexity**: LOW
**Breaking Change**: NO (backward compatible)

### Priority 4: Key Rotation Mechanism

**Enhancement**: Support credential re-encryption with new keys

```go
// cmd/secure_rotate.go
var rotateCmd = &cobra.Command{
    Use:   "rotate",
    Short: "Rotate encryption keys for all secure configs",
    RunE: func(cmd *cobra.Command, args []string) error {
        // 1. Generate new device key/salt
        newSalt := generateNewSalt()

        // 2. Decrypt all credentials with old key
        credentials := decryptWithOldKey()

        // 3. Re-encrypt with new key
        encryptWithNewKey(credentials, newSalt)

        // 4. Backup old config
        backupOldConfig()

        // 5. Save new config with version marker
        saveVersionedConfig(v2)

        return nil
    },
}
```

```yaml
# config.secure with versioning
version: "2"
encryption_version: 2
credentials_vault: "new_encrypted_data"
rotation_date: "2024-01-15T10:00:00Z"
```

**Benefits**:
- Periodic key rotation for compliance
- Recovery from potential key compromise
- Audit trail of rotations

**Implementation Priority**: LOW
**Complexity**: HIGH
**Breaking Change**: NO (versioned configs)

### Priority 5: Memory Protection

**Current**: Unused `SecureWipe()` function
**Enhancement**: Active memory protection for sensitive data

```go
// internal/crypto/memory.go
package crypto

import (
    "runtime"
    "unsafe"
)

// SecureString holds sensitive data with cleanup
type SecureString struct {
    data []byte
}

func NewSecureString(s string) *SecureString {
    return &SecureString{
        data: []byte(s),
    }
}

func (s *SecureString) String() string {
    return string(s.data)
}

func (s *SecureString) Destroy() {
    // Overwrite memory
    for i := range s.data {
        s.data[i] = 0
    }

    // Force garbage collection
    s.data = nil
    runtime.GC()
}

// Use defer for automatic cleanup
defer credentials.Destroy()
```

**Benefits**:
- Reduces credential exposure time in memory
- Protects against memory dumps
- Best practice for sensitive data handling

**Implementation Priority**: MEDIUM
**Complexity**: MEDIUM
**Breaking Change**: NO (internal only)

### Priority 6: Hardware Security Module Integration

**Enhancement**: Optional HSM/TPM support for enterprise users

```go
// internal/crypto/hsm/tpm.go
// +build linux,amd64

package hsm

import "github.com/google/go-tpm/tpm2"

func TPMAvailable() bool {
    // Check for TPM 2.0 device
    _, err := tpm2.OpenTPM("/dev/tpm0")
    return err == nil
}

func TPMSeal(data []byte) ([]byte, error) {
    // Seal data to TPM with PCR values
    // Data can only be unsealed on same hardware state
}
```

**Platform Support**:
- **Linux**: TPM 2.0 via /dev/tpm0
- **Windows**: TPM via Windows DPAPI
- **macOS**: Keychain Services
- **UDM/Routers**: Usually not available

**Benefits**:
- Hardware-backed encryption
- Tamper-resistant key storage
- Enterprise compliance (FIPS 140-2)

**Implementation Priority**: LOW
**Complexity**: VERY HIGH
**Breaking Change**: NO (optional feature)

## Implementation Roadmap

### Phase 1: Quick Wins (1-2 days)
1. ✅ Replace SHA-256 with PBKDF2
2. ✅ Generate unique salt per installation
3. ✅ Implement SecureWipe for memory cleanup

### Phase 2: User Features (1 week)
1. ⏳ Optional passphrase support
2. ⏳ Secure passphrase prompt (no echo)
3. ⏳ Passphrase strength validation

### Phase 3: Advanced Features (2 weeks)
1. ⏳ Key rotation mechanism
2. ⏳ Config versioning
3. ⏳ Migration tools

### Phase 4: Enterprise Features (1 month)
1. ⏳ TPM integration (Linux/Windows)
2. ⏳ Keychain integration (macOS)
3. ⏳ PKCS#11 support

## Security Comparison

| Feature | Current | Enhanced | Benefit |
|---------|---------|----------|---------|
| **Encryption** | AES-256-GCM | AES-256-GCM | No change needed |
| **Key Derivation** | SHA-256 | Argon2id | 1000x harder to crack |
| **Salt** | Hardcoded | Random per-install | Unique protection |
| **Passphrase** | None | Optional | User-controlled security |
| **Key Rotation** | None | Supported | Compliance ready |
| **Memory Protection** | Basic | Active wiping | Reduced exposure |
| **HSM Support** | None | Optional | Enterprise ready |

## Backward Compatibility

All enhancements maintain backward compatibility:

1. **Automatic Migration**: Old configs detected and migrated
2. **Version Detection**: Config version field identifies format
3. **Graceful Fallback**: New features are opt-in
4. **No Breaking Changes**: Existing deployments continue working

```go
// Automatic version detection
func LoadConfig(path string) (*Config, error) {
    // Check version
    version := detectConfigVersion(path)

    switch version {
    case 1:
        return loadV1Config(path)
    case 2:
        return loadV2Config(path)
    default:
        return nil, ErrUnknownVersion
    }
}
```

## Security Testing Checklist

- [ ] Penetration testing with common tools
- [ ] Brute force resistance testing
- [ ] Memory dump analysis
- [ ] Key extraction attempts
- [ ] Cross-platform compatibility
- [ ] Performance impact measurement
- [ ] Migration testing
- [ ] Backward compatibility verification

## Conclusion

These security enhancements provide defense-in-depth without compromising dddns's core principles:

1. **Maintains Simplicity**: Core functionality unchanged
2. **Preserves Performance**: Minimal overhead
3. **Ensures Compatibility**: No breaking changes
4. **Improves Security**: Significantly harder to compromise
5. **Future-Proof**: Ready for evolving threats

The phased approach allows incremental improvements based on user needs and threat landscape evolution. Priority should be given to Phase 1 improvements (PBKDF2 and unique salts) as they provide significant security gains with minimal implementation effort.