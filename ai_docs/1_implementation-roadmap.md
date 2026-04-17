# Implementation Roadmap for DNS Provider Architecture

## Overview
This document provides a detailed, file-by-file implementation roadmap for adding multi-provider support to dddns.

## File Structure Changes

```
dddns/
├── cmd/
│   ├── root.go                 [MODIFY: Add v2 config support]
│   ├── update.go                [MODIFY: Multi-target support]
│   ├── config.go                [MODIFY: Add provider selection]
│   ├── secure.go                [MODIFY: Provider-aware encryption]
│   └── target.go                [NEW: Target management commands]
│
├── internal/
│   ├── providers/               [NEW PACKAGE]
│   │   ├── provider.go          [NEW: Interface definitions]
│   │   ├── factory.go           [NEW: Provider factory]
│   │   ├── registry.go          [NEW: Provider registration]
│   │   ├── aws/                 [NEW PACKAGE]
│   │   │   └── route53.go       [REFACTOR: From internal/dns/]
│   │   ├── domeneshop/          [NEW PACKAGE]
│   │   │   ├── client.go        [NEW: HTTP client]
│   │   │   └── provider.go      [NEW: Provider implementation]
│   │   ├── gcp/                 [NEW PACKAGE]
│   │   │   └── clouddns.go      [NEW: GCP implementation]
│   │   └── azure/               [NEW PACKAGE]
│   │       └── azuredns.go      [NEW: Azure implementation]
│   │
│   ├── config/
│   │   ├── config.go            [MODIFY: Remove AWS-specific fields]
│   │   ├── config_v2.go         [NEW: Multi-target config]
│   │   ├── migration.go         [NEW: V1 to V2 migration]
│   │   └── secure_config_v2.go  [NEW: Provider-aware encryption]
│   │
│   ├── crypto/
│   │   ├── device_crypto.go     [KEEP: Core encryption]
│   │   └── provider_vault.go    [NEW: Provider-specific vaults]
│   │
│   └── dns/
│       └── route53.go           [DEPRECATE: Move to providers/aws/]
```

## Implementation Steps

### Step 1: Create Provider Interface Package

**File: `internal/providers/provider.go`**
```go
package providers

import (
    "context"
    "time"
)

// DNSProvider defines the interface for all DNS providers
type DNSProvider interface {
    // Core operations
    GetCurrentIP(ctx context.Context) (string, error)
    UpdateIP(ctx context.Context, newIP string, dryRun bool) error

    // Configuration
    ValidateConfig() error
    GetProviderName() string
    IsExperimental() bool

    // Metadata
    GetHostname() string
    GetTTL() int64
}

// ProviderConfig base configuration
type ProviderConfig struct {
    Provider     string `mapstructure:"provider" yaml:"provider"`
    Hostname     string `mapstructure:"hostname" yaml:"hostname"`
    TTL          int64  `mapstructure:"ttl" yaml:"ttl"`
    Experimental bool   `mapstructure:"experimental" yaml:"experimental,omitempty"`
}

// ProviderError for provider-specific errors
type ProviderError struct {
    Provider string
    Op       string
    Err      error
}
```

**File: `internal/providers/factory.go`**
```go
package providers

import (
    "fmt"
    "github.com/descoped/dddns/internal/crypto"
)

// Factory creates provider instances
type Factory struct {
    providers map[string]ProviderConstructor
}

// ProviderConstructor creates a provider from config
type ProviderConstructor func(config map[string]interface{}, vault crypto.ProviderVault) (DNSProvider, error)

// NewFactory creates a new provider factory
func NewFactory() *Factory {
    f := &Factory{
        providers: make(map[string]ProviderConstructor),
    }
    f.registerProviders()
    return f
}

// CreateProvider creates a provider instance
func (f *Factory) CreateProvider(name string, config map[string]interface{}, vault crypto.ProviderVault) (DNSProvider, error) {
    constructor, exists := f.providers[name]
    if !exists {
        return nil, fmt.Errorf("unknown provider: %s", name)
    }
    return constructor(config, vault)
}
```

### Step 2: Refactor Route53 to Provider Pattern

**File: `internal/providers/aws/route53.go`**
```go
package aws

import (
    "context"
    "fmt"

    "github.com/aws/aws-sdk-go-v2/service/route53"
    "github.com/descoped/dddns/internal/providers"
)

// Route53Provider implements DNSProvider for AWS Route53
type Route53Provider struct {
    client       *route53.Client
    config       *Config
}

// Config for Route53
type Config struct {
    providers.ProviderConfig
    AWSRegion    string `mapstructure:"aws_region" yaml:"aws_region"`
    HostedZoneID string `mapstructure:"hosted_zone_id" yaml:"hosted_zone_id"`

    // Credentials (decrypted from vault)
    AWSAccessKey string
    AWSSecretKey string
}

// NewRoute53Provider creates a new Route53 provider
func NewRoute53Provider(config map[string]interface{}, vault crypto.ProviderVault) (providers.DNSProvider, error) {
    // Parse config
    cfg := &Config{}
    if err := mapstructure.Decode(config, cfg); err != nil {
        return nil, err
    }

    // Decrypt credentials if vault provided
    if vault != nil {
        creds, err := vault.DecryptCredentials("aws", config["credentials_vault"].(string))
        if err != nil {
            return nil, err
        }
        cfg.AWSAccessKey = creds["access_key"]
        cfg.AWSSecretKey = creds["secret_key"]
    }

    // Create AWS client
    // ... existing Route53 client creation logic ...

    return &Route53Provider{
        client: client,
        config: cfg,
    }, nil
}

// Implement DNSProvider interface methods
func (r *Route53Provider) GetCurrentIP(ctx context.Context) (string, error) {
    // ... existing logic from route53.go ...
}

func (r *Route53Provider) UpdateIP(ctx context.Context, newIP string, dryRun bool) error {
    // ... existing logic from route53.go ...
}

func (r *Route53Provider) ValidateConfig() error {
    // Validate required fields
    if r.config.HostedZoneID == "" {
        return fmt.Errorf("hosted_zone_id is required")
    }
    // ... other validation ...
    return nil
}

func (r *Route53Provider) GetProviderName() string {
    return "aws"
}

func (r *Route53Provider) IsExperimental() bool {
    return false
}
```

### Step 3: Implement Domeneshop Provider

**File: `internal/providers/domeneshop/client.go`**
```go
package domeneshop

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// Client for Domeneshop API
type Client struct {
    baseURL    string
    token      string
    secret     string
    httpClient *http.Client
}

// NewClient creates a new Domeneshop API client
func NewClient(token, secret string) *Client {
    return &Client{
        baseURL: "https://api.domeneshop.no/v0",
        token:   token,
        secret:  secret,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

// DNSRecord represents a DNS record
type DNSRecord struct {
    ID   int    `json:"id,omitempty"`
    Host string `json:"host"`
    Type string `json:"type"`
    Data string `json:"data"`
    TTL  int    `json:"ttl"`
}

// GetARecord retrieves the A record for a hostname
func (c *Client) GetARecord(domainID, hostname string) (*DNSRecord, error) {
    url := fmt.Sprintf("%s/domains/%s/dns", c.baseURL, domainID)

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.SetBasicAuth(c.token, c.secret)
    req.URL.Query().Add("host", hostname)
    req.URL.Query().Add("type", "A")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("API error: %d", resp.StatusCode)
    }

    var records []DNSRecord
    if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
        return nil, err
    }

    if len(records) == 0 {
        return nil, nil
    }

    return &records[0], nil
}

// UpdateARecord updates or creates an A record
func (c *Client) UpdateARecord(domainID string, record *DNSRecord) error {
    var url string
    var method string

    if record.ID > 0 {
        // Update existing
        url = fmt.Sprintf("%s/domains/%s/dns/%d", c.baseURL, domainID, record.ID)
        method = "PUT"
    } else {
        // Create new
        url = fmt.Sprintf("%s/domains/%s/dns", c.baseURL, domainID)
        method = "POST"
    }

    body, err := json.Marshal(record)
    if err != nil {
        return err
    }

    req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
    if err != nil {
        return err
    }

    req.SetBasicAuth(c.token, c.secret)
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("API error: %d", resp.StatusCode)
    }

    return nil
}
```

**File: `internal/providers/domeneshop/provider.go`**
```go
package domeneshop

import (
    "context"
    "fmt"
    "strings"

    "github.com/descoped/dddns/internal/providers"
)

// DomeneshopProvider implements DNSProvider for Domeneshop
type DomeneshopProvider struct {
    client *Client
    config *Config
}

// Config for Domeneshop
type Config struct {
    providers.ProviderConfig
    DomainID string `mapstructure:"domain_id" yaml:"domain_id"`

    // Credentials (decrypted from vault)
    Token  string
    Secret string
}

// NewDomeneshopProvider creates a new Domeneshop provider
func NewDomeneshopProvider(config map[string]interface{}, vault crypto.ProviderVault) (providers.DNSProvider, error) {
    cfg := &Config{}
    if err := mapstructure.Decode(config, cfg); err != nil {
        return nil, err
    }

    // Decrypt credentials if vault provided
    if vault != nil {
        creds, err := vault.DecryptCredentials("domeneshop", config["credentials_vault"].(string))
        if err != nil {
            return nil, err
        }
        cfg.Token = creds["token"]
        cfg.Secret = creds["secret"]
    }

    client := NewClient(cfg.Token, cfg.Secret)

    return &DomeneshopProvider{
        client: client,
        config: cfg,
    }, nil
}

func (d *DomeneshopProvider) GetCurrentIP(ctx context.Context) (string, error) {
    // Extract subdomain from hostname
    subdomain := d.extractSubdomain()

    record, err := d.client.GetARecord(d.config.DomainID, subdomain)
    if err != nil {
        return "", err
    }

    if record == nil {
        return "", fmt.Errorf("A record not found for %s", d.config.Hostname)
    }

    return record.Data, nil
}

func (d *DomeneshopProvider) UpdateIP(ctx context.Context, newIP string, dryRun bool) error {
    if dryRun {
        fmt.Printf("[DRY RUN] Would update %s to %s\n", d.config.Hostname, newIP)
        return nil
    }

    subdomain := d.extractSubdomain()

    // Get existing record to get ID
    existing, _ := d.client.GetARecord(d.config.DomainID, subdomain)

    record := &DNSRecord{
        Host: subdomain,
        Type: "A",
        Data: newIP,
        TTL:  int(d.config.TTL),
    }

    if existing != nil {
        record.ID = existing.ID
    }

    return d.client.UpdateARecord(d.config.DomainID, record)
}

func (d *DomeneshopProvider) extractSubdomain() string {
    // Extract subdomain from full hostname
    // e.g., "home.example.no" -> "home"
    parts := strings.Split(d.config.Hostname, ".")
    if len(parts) > 2 {
        return strings.Join(parts[:len(parts)-2], ".")
    }
    return "@" // Root domain
}
```

### Step 4: Update Configuration System

**File: `internal/config/config_v2.go`**
```go
package config

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/spf13/viper"
    "gopkg.in/yaml.v3"
)

// ConfigV2 represents the new multi-target configuration
type ConfigV2 struct {
    Version        string                    `yaml:"version"`
    IPCacheFile    string                    `yaml:"ip_cache_file"`
    SkipProxyCheck bool                      `yaml:"skip_proxy_check"`
    Targets        map[string]TargetConfig   `yaml:"targets"`
    DefaultTargets []string                  `yaml:"default_targets"`
}

// TargetConfig represents a single DNS target
type TargetConfig struct {
    Provider         string                 `yaml:"provider"`
    Hostname         string                 `yaml:"hostname"`
    TTL              int64                  `yaml:"ttl"`

    // Provider-specific fields (stored as map for flexibility)
    ProviderConfig   map[string]interface{} `yaml:",inline"`
}

// LoadV2 loads v2 configuration
func LoadV2() (*ConfigV2, error) {
    // Check config version
    if viper.GetString("version") != "2" {
        // Try to migrate from v1
        v1Config, err := Load()
        if err != nil {
            return nil, err
        }
        return MigrateV1ToV2(v1Config)
    }

    cfg := &ConfigV2{}
    if err := viper.Unmarshal(cfg); err != nil {
        return nil, err
    }

    return cfg, nil
}

// GetTarget retrieves a specific target configuration
func (c *ConfigV2) GetTarget(name string) (TargetConfig, error) {
    target, exists := c.Targets[name]
    if !exists {
        return TargetConfig{}, fmt.Errorf("target %s not found", name)
    }
    return target, nil
}

// GetTargetsToUpdate returns the list of targets to update
func (c *ConfigV2) GetTargetsToUpdate(specified []string) []string {
    if len(specified) > 0 {
        return specified
    }
    if len(c.DefaultTargets) > 0 {
        return c.DefaultTargets
    }
    // If no defaults, update all
    targets := make([]string, 0, len(c.Targets))
    for name := range c.Targets {
        targets = append(targets, name)
    }
    return targets
}
```

**File: `internal/config/migration.go`**
```go
package config

import (
    "fmt"
    "os"
)

// MigrateV1ToV2 migrates v1 config to v2 format
func MigrateV1ToV2(v1 *Config) (*ConfigV2, error) {
    v2 := &ConfigV2{
        Version:        "2",
        IPCacheFile:    v1.IPCacheFile,
        SkipProxyCheck: v1.SkipProxy,
        Targets: map[string]TargetConfig{
            "default": {
                Provider: "aws",
                Hostname: v1.Hostname,
                TTL:      v1.TTL,
                ProviderConfig: map[string]interface{}{
                    "aws_region":      v1.AWSRegion,
                    "hosted_zone_id":  v1.HostedZoneID,
                    "aws_access_key":  v1.AWSAccessKey,
                    "aws_secret_key":  v1.AWSSecretKey,
                },
            },
        },
        DefaultTargets: []string{"default"},
    }

    return v2, nil
}

// SaveMigration saves the migrated configuration
func SaveMigration(v2 *ConfigV2, path string) error {
    // Backup existing config
    backupPath := path + ".v1.backup"
    if err := os.Rename(path, backupPath); err != nil {
        return fmt.Errorf("failed to backup v1 config: %w", err)
    }

    // Save v2 config
    data, err := yaml.Marshal(v2)
    if err != nil {
        return fmt.Errorf("failed to marshal v2 config: %w", err)
    }

    if err := os.WriteFile(path, data, 0600); err != nil {
        // Restore backup on failure
        _ = os.Rename(backupPath, path)
        return fmt.Errorf("failed to save v2 config: %w", err)
    }

    fmt.Printf("✓ Migrated configuration to v2 format\n")
    fmt.Printf("✓ Backup saved to %s\n", backupPath)

    return nil
}
```

### Step 5: Update Crypto for Provider Vaults

**File: `internal/crypto/provider_vault.go`**
```go
package crypto

import (
    "crypto/sha256"
    "encoding/json"
    "fmt"
)

// ProviderVault manages provider-specific credential encryption
type ProviderVault struct {
    deviceKey []byte
}

// NewProviderVault creates a new provider vault
func NewProviderVault() (*ProviderVault, error) {
    deviceKey, err := GetDeviceKey()
    if err != nil {
        return nil, err
    }

    return &ProviderVault{
        deviceKey: deviceKey,
    }, nil
}

// EncryptCredentials encrypts provider-specific credentials
func (v *ProviderVault) EncryptCredentials(provider string, credentials map[string]string) (string, error) {
    // Serialize credentials
    jsonData, err := json.Marshal(credentials)
    if err != nil {
        return "", err
    }

    // Create provider-specific key
    providerKey := v.deriveProviderKey(provider)

    // Encrypt using existing AES-GCM implementation
    return encryptWithKey(providerKey, jsonData)
}

// DecryptCredentials decrypts provider-specific credentials
func (v *ProviderVault) DecryptCredentials(provider string, vault string) (map[string]string, error) {
    // Create provider-specific key
    providerKey := v.deriveProviderKey(provider)

    // Decrypt
    jsonData, err := decryptWithKey(providerKey, vault)
    if err != nil {
        return nil, err
    }

    // Deserialize
    var credentials map[string]string
    if err := json.Unmarshal(jsonData, &credentials); err != nil {
        return nil, err
    }

    return credentials, nil
}

// deriveProviderKey derives a provider-specific encryption key
func (v *ProviderVault) deriveProviderKey(provider string) []byte {
    // Combine device key with provider-specific salt
    salt := fmt.Sprintf("dddns-%s-vault-2025", provider)
    combined := append(v.deviceKey, []byte(salt)...)

    // Derive provider-specific key
    hash := sha256.Sum256(combined)
    return hash[:]
}

// MigrateToProviderVault migrates v1 secure config to provider vaults
func MigrateToProviderVault(v1Vault string) (map[string]string, error) {
    // Decrypt v1 AWS credentials
    accessKey, secretKey, err := DecryptCredentials(v1Vault)
    if err != nil {
        return nil, err
    }

    // Create new provider vault
    vault, err := NewProviderVault()
    if err != nil {
        return nil, err
    }

    // Encrypt for AWS provider
    awsCreds := map[string]string{
        "access_key": accessKey,
        "secret_key": secretKey,
    }

    awsVault, err := vault.EncryptCredentials("aws", awsCreds)
    if err != nil {
        return nil, err
    }

    return map[string]string{
        "aws": awsVault,
    }, nil
}
```

### Step 6: Update Command Implementation

**File: `cmd/update.go` (Modified)**
```go
package cmd

import (
    "context"
    "fmt"
    "log"
    "sync"

    "github.com/descoped/dddns/internal/config"
    "github.com/descoped/dddns/internal/providers"
    "github.com/spf13/cobra"
)

var (
    targetNames []string
    updateAll   bool
)

func init() {
    updateCmd.Flags().StringSliceVar(&targetNames, "target", []string{}, "Specific targets to update")
    updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update all configured targets")
}

func runUpdate(_ *cobra.Command, _ []string) error {
    // Load v2 configuration
    cfg, err := config.LoadV2()
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // Determine targets to update
    targets := cfg.GetTargetsToUpdate(targetNames)
    if updateAll {
        targets = getAllTargets(cfg)
    }

    // Get current IP once
    currentIP, err := getPublicIP()
    if err != nil {
        return err
    }

    // Update targets (parallel if multiple)
    if len(targets) == 1 {
        return updateTarget(cfg, targets[0], currentIP)
    }

    return updateTargetsParallel(cfg, targets, currentIP)
}

func updateTarget(cfg *config.ConfigV2, targetName string, currentIP string) error {
    target, err := cfg.GetTarget(targetName)
    if err != nil {
        return err
    }

    // Create provider
    factory := providers.NewFactory()
    provider, err := factory.CreateProvider(target.Provider, target.ProviderConfig, nil)
    if err != nil {
        return fmt.Errorf("failed to create provider %s: %w", target.Provider, err)
    }

    // Check if experimental
    if provider.IsExperimental() {
        log.Printf("⚠️  WARNING: Provider '%s' is experimental and may not work correctly", target.Provider)
    }

    // Get current DNS IP
    ctx := context.Background()
    dnsIP, err := provider.GetCurrentIP(ctx)
    if err != nil {
        logInfo("Warning: could not get current DNS record for %s: %v", targetName, err)
    }

    // Check if update needed
    if currentIP == dnsIP && !forceUpdate {
        logInfo("Target %s: IP unchanged (%s), skipping", targetName, currentIP)
        return nil
    }

    // Update DNS
    if dryRun {
        log.Printf("[DRY RUN] Would update %s: %s -> %s", targetName, dnsIP, currentIP)
    } else {
        logInfo("Updating %s: %s -> %s", targetName, dnsIP, currentIP)
        if err := provider.UpdateIP(ctx, currentIP, false); err != nil {
            return fmt.Errorf("failed to update %s: %w", targetName, err)
        }
        log.Printf("✓ Successfully updated %s to %s", targetName, currentIP)
    }

    return nil
}

func updateTargetsParallel(cfg *config.ConfigV2, targets []string, currentIP string) error {
    var wg sync.WaitGroup
    errors := make(chan error, len(targets))

    for _, target := range targets {
        wg.Add(1)
        go func(t string) {
            defer wg.Done()
            if err := updateTarget(cfg, t, currentIP); err != nil {
                errors <- fmt.Errorf("%s: %w", t, err)
            }
        }(target)
    }

    wg.Wait()
    close(errors)

    // Collect errors
    var allErrors []error
    for err := range errors {
        allErrors = append(allErrors, err)
    }

    if len(allErrors) > 0 {
        log.Printf("⚠️  %d targets failed to update:", len(allErrors))
        for _, err := range allErrors {
            log.Printf("  - %v", err)
        }
        return fmt.Errorf("%d targets failed", len(allErrors))
    }

    return nil
}
```

## Testing Plan

### Unit Tests

1. **Provider Interface Tests** (`internal/providers/provider_test.go`)
   - Interface compliance
   - Factory creation
   - Error handling

2. **Domeneshop Client Tests** (`internal/providers/domeneshop/client_test.go`)
   - Mock HTTP responses
   - Authentication
   - Error scenarios

3. **Configuration Tests** (`internal/config/config_v2_test.go`)
   - V1 to V2 migration
   - Target selection
   - Validation

4. **Crypto Tests** (`internal/crypto/provider_vault_test.go`)
   - Provider-specific encryption
   - Vault migration
   - Key derivation

### Integration Tests

1. **End-to-End Update** (`test/integration/update_test.go`)
   - Mock providers
   - Multi-target updates
   - Error recovery

2. **Migration Test** (`test/integration/migration_test.go`)
   - V1 config migration
   - Secure config migration
   - Backward compatibility

## Rollout Strategy

### Phase 1: Foundation (Days 1-3)
- Provider interface and factory
- Route53 refactor
- Basic tests

### Phase 2: Domeneshop (Days 4-5)
- Full implementation
- Testing with real account
- Documentation

### Phase 3: Multi-Target (Days 6-7)
- Config v2 implementation
- Migration logic
- Parallel updates

### Phase 4: Experimental (Days 8-9)
- GCP skeleton
- Azure skeleton
- Experimental warnings

### Phase 5: Polish (Days 10-14)
- Complete testing
- Documentation
- Performance optimization
- Release preparation

## Success Criteria

1. **Zero Breaking Changes**
   - Existing configs continue working
   - Commands maintain compatibility
   - Automatic migration

2. **Provider Support**
   - Route53 fully working (refactored)
   - Domeneshop fully tested
   - GCP/Azure marked experimental

3. **Performance**
   - Memory usage <20MB
   - Parallel updates efficient
   - Fast startup time

4. **Security**
   - Provider-specific vaults
   - Secure migration
   - No credential leaks

## Next Steps

1. Review and approve design
2. Create feature branch
3. Begin Phase 1 implementation
4. Set up test Domeneshop account
5. Coordinate testing on UDM device