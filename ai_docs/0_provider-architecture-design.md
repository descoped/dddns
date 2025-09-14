# DNS Provider Architecture Design for dddns

## Executive Summary

This document outlines the architectural design for extending dddns to support multiple DNS providers beyond AWS Route53. The design maintains backward compatibility, implements provider-specific credential encryption, and supports multi-target DNS updates with profiles.

## Current State Analysis

### Existing Architecture Flow
1. **IP Detection**: Get public IP via `checkip.amazonaws.com`
2. **Cache Comparison**: Check against cached IP (`last-ip.txt`)
3. **Provider Update**: Update Route53 A record if changed
4. **Cache Update**: Store new IP for next run

### Key Characteristics
- Single provider (AWS Route53) hardcoded
- Device-specific encryption for credentials
- Platform-aware configuration paths
- Minimal dependencies, optimized for constrained devices

## Proposed Provider Architecture

### 1. Provider Interface Design

```go
// internal/providers/provider.go
package providers

import (
    "context"
)

// DNSProvider defines the common interface all DNS providers must implement
type DNSProvider interface {
    // GetCurrentIP retrieves the current IP for the hostname
    GetCurrentIP(ctx context.Context) (string, error)

    // UpdateIP updates the DNS record with new IP
    UpdateIP(ctx context.Context, newIP string, dryRun bool) error

    // ValidateConfig checks if provider configuration is valid
    ValidateConfig() error

    // GetProviderName returns the provider identifier
    GetProviderName() string
}

// ProviderConfig is the base configuration all providers share
type ProviderConfig struct {
    Provider string `mapstructure:"provider"`
    Hostname string `mapstructure:"hostname"`
    TTL      int64  `mapstructure:"ttl"`
}

// ProviderFactory creates provider instances based on configuration
type ProviderFactory interface {
    CreateProvider(config map[string]interface{}) (DNSProvider, error)
}
```

### 2. Provider Implementations

#### AWS Route53 Provider
```go
// internal/providers/aws/route53.go
package aws

type Route53Provider struct {
    client       route53API
    config       *Route53Config
}

type Route53Config struct {
    ProviderConfig
    AWSRegion    string `mapstructure:"aws_region"`
    AWSAccessKey string `mapstructure:"aws_access_key"`
    AWSSecretKey string `mapstructure:"aws_secret_key"`
    HostedZoneID string `mapstructure:"hosted_zone_id"`
}
```

#### Domeneshop Provider
```go
// internal/providers/domeneshop/domeneshop.go
package domeneshop

type DomeneshopProvider struct {
    client  *httpClient
    config  *DomeneshopConfig
}

type DomeneshopConfig struct {
    ProviderConfig
    DomainID string `mapstructure:"domain_id"`
    Token    string `mapstructure:"token"`
    Secret   string `mapstructure:"secret"`
}
```

#### Google Cloud DNS Provider
```go
// internal/providers/gcp/clouddns.go
package gcp

type CloudDNSProvider struct {
    client  *dns.Service
    config  *CloudDNSConfig
}

type CloudDNSConfig struct {
    ProviderConfig
    ProjectID       string `mapstructure:"project_id"`
    ManagedZone     string `mapstructure:"managed_zone"`
    ServiceAccount  string `mapstructure:"service_account_json"`
}
```

#### Azure DNS Provider
```go
// internal/providers/azure/azuredns.go
package azure

type AzureDNSProvider struct {
    client  *armdns.RecordSetsClient
    config  *AzureDNSConfig
}

type AzureDNSConfig struct {
    ProviderConfig
    SubscriptionID  string `mapstructure:"subscription_id"`
    ResourceGroup   string `mapstructure:"resource_group"`
    ZoneName        string `mapstructure:"zone_name"`
    TenantID        string `mapstructure:"tenant_id"`
    ClientID        string `mapstructure:"client_id"`
    ClientSecret    string `mapstructure:"client_secret"`
}
```

## Secure Credentials Architecture

### 1. Provider-Specific Vault Keys

```go
// internal/crypto/provider_crypto.go
package crypto

// ProviderCredentials handles provider-specific credential encryption
type ProviderCredentials interface {
    // EncryptProviderCredentials encrypts provider-specific credentials
    EncryptProviderCredentials(provider string, credentials map[string]string) (string, error)

    // DecryptProviderCredentials decrypts provider-specific credentials
    DecryptProviderCredentials(provider string, vault string) (map[string]string, error)
}

// Implementation
func EncryptProviderCredentials(provider string, credentials map[string]string) (string, error) {
    // Serialize credentials to JSON
    jsonData, _ := json.Marshal(credentials)

    // Add provider-specific salt
    salt := fmt.Sprintf("dddns-%s-vault-2025", provider)

    // Use existing device key derivation with provider salt
    key := deriveProviderKey(provider, salt)

    // Encrypt using AES-256-GCM
    return encryptWithKey(key, jsonData)
}
```

### 2. Secure Config Structure

```go
// internal/config/secure_provider_config.go
package config

type SecureProviderConfig struct {
    Provider            string `yaml:"provider"`
    Hostname            string `yaml:"hostname"`
    TTL                 int64  `yaml:"ttl"`
    CredentialsVault    string `yaml:"credentials_vault"` // Encrypted provider-specific creds

    // Non-sensitive provider-specific fields
    AWSRegion           string `yaml:"aws_region,omitempty"`
    HostedZoneID        string `yaml:"hosted_zone_id,omitempty"`
    DomainID            string `yaml:"domain_id,omitempty"`
    ProjectID           string `yaml:"project_id,omitempty"`
    ManagedZone         string `yaml:"managed_zone,omitempty"`
    SubscriptionID      string `yaml:"subscription_id,omitempty"`
    ResourceGroup       string `yaml:"resource_group,omitempty"`
    ZoneName            string `yaml:"zone_name,omitempty"`
}
```

## Configuration Schema

### 1. Multi-Target Configuration with Profiles

```yaml
# /data/.dddns/config.yaml (or config.secure)
version: "2"  # Config version for migration support

# Global settings
ip_cache_file: /data/.dddns/last-ip.txt
skip_proxy_check: false

# DNS targets (each is a profile)
targets:
  # Profile 1: AWS Route53
  home-aws:
    provider: aws
    aws_region: us-east-1
    hosted_zone_id: ZBCMVMPX00SYZ
    hostname: home.example.com
    ttl: 300
    aws_access_key: AKIAXXXXXXX  # Plain text (will be encrypted in .secure)
    aws_secret_key: xxxxxxxxx    # Plain text (will be encrypted in .secure)

  # Profile 2: Domeneshop
  home-domeneshop:
    provider: domeneshop
    domain_id: "123456"
    hostname: home.example.no
    ttl: 300
    token: "token123"      # Plain text (will be encrypted in .secure)
    secret: "secret456"    # Plain text (will be encrypted in .secure)

  # Profile 3: Google Cloud DNS (Experimental)
  home-gcp:
    provider: gcp
    project_id: my-project
    managed_zone: my-zone
    hostname: home.example.org
    ttl: 300
    service_account_json: |  # Will be encrypted in .secure
      {
        "type": "service_account",
        "project_id": "my-project",
        ...
      }
    experimental: true

  # Profile 4: Azure DNS (Experimental)
  home-azure:
    provider: azure
    subscription_id: xxxx-xxxx-xxxx
    resource_group: my-rg
    zone_name: example.net
    hostname: home.example.net
    ttl: 300
    tenant_id: xxxx-xxxx-xxxx     # Will be encrypted
    client_id: xxxx-xxxx-xxxx     # Will be encrypted
    client_secret: xxxxxxxxx       # Will be encrypted
    experimental: true

# Default targets to update (if not specified via CLI)
default_targets:
  - home-aws
  - home-domeneshop
```

### 2. Secure Configuration Format

```yaml
# /data/.dddns/config.secure
version: "2"

ip_cache_file: /data/.dddns/last-ip.txt
skip_proxy_check: false

targets:
  home-aws:
    provider: aws
    aws_region: us-east-1
    hosted_zone_id: ZBCMVMPX00SYZ
    hostname: home.example.com
    ttl: 300
    credentials_vault: "DsUQ1/kcAWLmxRiGnYb38uadJ73vzV..."  # Encrypted AWS creds

  home-domeneshop:
    provider: domeneshop
    domain_id: "123456"
    hostname: home.example.no
    ttl: 300
    credentials_vault: "KjH8/nmcBQLpxSjHoZc49vbeK84w..."  # Encrypted Domeneshop creds

default_targets:
  - home-aws
  - home-domeneshop
```

## CLI Command Updates

### 1. Update Command Enhancements

```bash
# Update all default targets
dddns update

# Update specific target
dddns update --target home-aws

# Update multiple specific targets
dddns update --target home-aws --target home-domeneshop

# Update all targets
dddns update --all

# Dry run for specific target
dddns update --target home-aws --dry-run
```

### 2. Config Commands

```bash
# Initialize config with provider selection
dddns config init --provider aws
dddns config init --provider domeneshop

# Add new target to existing config
dddns config add-target --name home-backup --provider domeneshop

# List configured targets
dddns config list-targets

# Validate specific target
dddns config check --target home-aws
```

### 3. Secure Commands

```bash
# Enable encryption for all targets
dddns secure enable

# Enable encryption for specific target
dddns secure enable --target home-aws

# Test encryption for all providers
dddns secure test
```

## Implementation Plan

### Phase 1: Provider Interface Foundation (Week 1)
1. **Create provider package structure**
   - `internal/providers/provider.go` - Interface definitions
   - `internal/providers/factory.go` - Provider factory
   - `internal/providers/registry.go` - Provider registration

2. **Refactor existing Route53 code**
   - Move to `internal/providers/aws/`
   - Implement DNSProvider interface
   - Maintain backward compatibility

3. **Update configuration loading**
   - Support both legacy and new config formats
   - Auto-migration from v1 to v2 config

### Phase 2: Domeneshop Provider (Week 1-2)
1. **Implement Domeneshop provider**
   - HTTP client with basic auth
   - Domain ID resolution
   - A record management

2. **Add provider-specific encryption**
   - Token/secret vault storage
   - Provider-aware decryption

3. **Integration testing**
   - Mock HTTP responses
   - End-to-end with test account

### Phase 3: Multi-Target Support (Week 2)
1. **Update configuration parser**
   - Target definitions
   - Default target selection
   - Profile validation

2. **Enhance update command**
   - Parallel target updates
   - Target-specific error handling
   - Consolidated reporting

3. **Update cache management**
   - Per-target IP caching
   - Cache file format update

### Phase 4: Experimental Providers (Week 3)
1. **Google Cloud DNS provider**
   - Service account authentication
   - Zone/record management
   - Mark as experimental

2. **Azure DNS provider**
   - Service principal auth
   - Record set operations
   - Mark as experimental

3. **Documentation**
   - Provider setup guides
   - Migration documentation
   - Experimental disclaimer

### Phase 5: Testing & Polish (Week 3-4)
1. **Comprehensive testing**
   - Unit tests for all providers
   - Integration tests with mocks
   - Cross-platform validation

2. **Performance optimization**
   - Parallel updates
   - Connection pooling
   - Memory profiling

3. **Documentation completion**
   - Update README
   - Provider-specific guides
   - Configuration examples

## Migration Strategy

### Backward Compatibility
1. **Config auto-detection**
   - Check for version field
   - If missing, assume v1 (current format)
   - Auto-migrate on first run

2. **Legacy command support**
   - `dddns update` works with v1 config
   - Maps to single "default" target
   - No breaking changes

3. **Gradual migration path**
   - Users can continue with Route53-only
   - Add providers incrementally
   - Secure migration independent

### Config Migration Example

```go
func MigrateV1ToV2(v1Config *Config) *ConfigV2 {
    return &ConfigV2{
        Version: "2",
        IPCacheFile: v1Config.IPCacheFile,
        SkipProxyCheck: v1Config.SkipProxy,
        Targets: map[string]TargetConfig{
            "default": {
                Provider: "aws",
                AWSRegion: v1Config.AWSRegion,
                HostedZoneID: v1Config.HostedZoneID,
                Hostname: v1Config.Hostname,
                TTL: v1Config.TTL,
                AWSAccessKey: v1Config.AWSAccessKey,
                AWSSecretKey: v1Config.AWSSecretKey,
            },
        },
        DefaultTargets: []string{"default"},
    }
}
```

## Security Considerations

1. **Credential Isolation**
   - Each provider has separate vault
   - Provider-specific encryption salts
   - No credential cross-contamination

2. **Minimal Permissions**
   - Document minimum IAM/RBAC requirements
   - Provider-specific permission guides
   - Principle of least privilege

3. **Experimental Warnings**
   - Clear marking of untested providers
   - Disclaimer in documentation
   - Warning on first use

## Performance Considerations

1. **Parallel Updates**
   - Concurrent target updates
   - Configurable timeout per provider
   - Fail-fast on errors

2. **Connection Reuse**
   - HTTP client pooling for REST APIs
   - SDK client caching
   - Minimize TLS handshakes

3. **Memory Constraints**
   - Lazy provider initialization
   - Stream large responses
   - Target <20MB total usage

## Testing Strategy

1. **Unit Tests**
   - Provider interface compliance
   - Configuration parsing
   - Encryption/decryption

2. **Integration Tests**
   - Mock provider backends
   - Full update cycle
   - Error scenarios

3. **Manual Testing**
   - Real Domeneshop account
   - UDM device testing
   - Cross-platform validation

## Success Metrics

1. **Backward Compatibility**
   - Zero breaking changes
   - Seamless migration
   - No user intervention required

2. **Provider Support**
   - Domeneshop fully functional
   - GCP/Azure marked experimental
   - Clear documentation

3. **Performance**
   - <20MB memory usage
   - <5s total update time
   - Parallel execution efficient

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing deployments | High | Extensive backward compatibility testing |
| Provider API changes | Medium | Version pinning, regular testing |
| Credential leak | High | Encryption, secure file permissions |
| Memory bloat | Medium | Profiling, lazy loading |
| Complex configuration | Low | Clear examples, validation |

## Timeline

- **Week 1**: Provider interface, Route53 refactor
- **Week 2**: Domeneshop implementation, multi-target
- **Week 3**: Experimental providers, testing
- **Week 4**: Documentation, polish, release prep

## Conclusion

This architecture provides a clean, extensible way to support multiple DNS providers while maintaining dddns's core principles of simplicity and efficiency. The design ensures backward compatibility, implements secure credential management, and enables flexible multi-target configurations suitable for various deployment scenarios.