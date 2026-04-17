# Future DNS Provider Support Roadmap

## Overview
This document outlines additional DNS providers that should be considered for future implementation in dddns, beyond the initial set (AWS Route53, Domeneshop, Google Cloud DNS, Azure DNS).

## Priority Matrix

### Tier 1: High Priority (Strong User Demand)

#### 1. Cloudflare DNS
- **Priority**: CRITICAL
- **Rationale**:
  - Largest CDN/DNS provider with massive free tier
  - Most requested by community
  - Excellent API documentation
  - Fast global anycast network
- **API Type**: REST API v4
- **Authentication**: API Token (scoped) or Global API Key
- **Complexity**: Low
- **Documentation**: https://developers.cloudflare.com/api/
- **Special Features**:
  - Proxied vs DNS-only records
  - Free SSL certificates
  - DDoS protection

#### 2. Namecheap
- **Priority**: HIGH
- **Rationale**:
  - Very popular domain registrar
  - Large individual/small business user base
  - Test account available (per user)
- **API Type**: XML-based API
- **Authentication**: API Key + Username + Client IP whitelisting
- **Complexity**: Medium (XML parsing required)
- **Documentation**: https://www.namecheap.com/support/api/
- **Special Considerations**:
  - Requires IP whitelisting
  - XML responses (not JSON)

#### 3. GoDaddy
- **Priority**: HIGH
- **Rationale**:
  - Largest domain registrar by market share
  - Huge non-technical user base
  - Would benefit many home users
- **API Type**: REST API
- **Authentication**: API Key + Secret (OAuth-like)
- **Complexity**: Low
- **Documentation**: https://developer.godaddy.com/
- **Rate Limits**: 60 requests/minute

### Tier 2: Medium Priority (Developer Focused)

#### 4. DigitalOcean DNS
- **Priority**: MEDIUM
- **Rationale**:
  - Popular with developers
  - Free DNS service with droplets
  - Clean, well-documented API
- **API Type**: REST API v2
- **Authentication**: Bearer token
- **Complexity**: Low
- **Documentation**: https://docs.digitalocean.com/reference/api/
- **Benefits**:
  - No domain purchase required
  - Integrated with infrastructure

#### 5. Linode DNS
- **Priority**: MEDIUM
- **Rationale**:
  - Developer-friendly
  - Good API documentation
  - Growing user base
- **API Type**: REST API v4
- **Authentication**: Personal Access Token
- **Complexity**: Low
- **Documentation**: https://www.linode.com/api/v4/
- **Benefits**: Free DNS management

#### 6. Hetzner DNS
- **Priority**: MEDIUM
- **Rationale**:
  - Very popular in Europe
  - Competitive pricing
  - Simple API
- **API Type**: REST API
- **Authentication**: API Token
- **Complexity**: Low
- **Documentation**: https://dns.hetzner.com/api-docs
- **Regional Focus**: Europe

### Tier 3: Nice to Have (Specialized/Regional)

#### 7. Gandi
- **Priority**: LOW
- **Rationale**:
  - Popular in Europe
  - Privacy-focused
  - Clean API
- **API Type**: REST API
- **Authentication**: API Key
- **Complexity**: Low
- **Documentation**: https://doc.livedns.gandi.net/

#### 8. Name.com
- **Priority**: LOW
- **Rationale**:
  - Major registrar
  - Good API support
- **API Type**: REST API v4
- **Authentication**: Username + Token
- **Complexity**: Low
- **Documentation**: https://www.name.com/api-docs

#### 9. Porkbun
- **Priority**: LOW
- **Rationale**:
  - Growing popularity
  - Developer-friendly
  - Competitive pricing
- **API Type**: REST API
- **Authentication**: API Key + Secret Key
- **Complexity**: Low
- **Documentation**: https://porkbun.com/api/json/v3/documentation

#### 10. DuckDNS
- **Priority**: LOW
- **Rationale**:
  - Free dynamic DNS service
  - Simple HTTP GET updates
  - Popular for home automation
- **API Type**: HTTP GET
- **Authentication**: Token in URL
- **Complexity**: Very Low
- **Documentation**: https://www.duckdns.org/spec.jsp
- **Special Note**: Specifically designed for dynamic DNS

## Implementation Considerations

### Common Patterns

Most providers follow similar patterns that can be abstracted:

1. **REST API Providers** (Majority)
   - Standard HTTP client
   - JSON request/response
   - Bearer token or API key auth
   - Similar CRUD operations

2. **Special Cases**
   - **Namecheap**: XML-based, needs XML parser
   - **DuckDNS**: Simple HTTP GET, minimal implementation
   - **Cloudflare**: Additional proxy settings

### Suggested Implementation Order

1. **Phase 1** (After initial release):
   - Cloudflare (critical priority)
   - Namecheap (test access available)

2. **Phase 2**:
   - GoDaddy (large user base)
   - DigitalOcean (developer community)

3. **Phase 3**:
   - Linode
   - Hetzner (if European user demand)

4. **Community Contributed**:
   - Remaining providers based on demand
   - Accept PRs following provider template

## Provider Template Requirements

For community contributions, each provider should:

1. **Implement Core Interface**
   ```go
   type DNSProvider interface {
       GetCurrentIP(ctx context.Context) (string, error)
       UpdateIP(ctx context.Context, newIP string, dryRun bool) error
       ValidateConfig() error
       GetProviderName() string
       IsExperimental() bool
   }
   ```

2. **Include Documentation**
   - Setup guide with screenshots
   - API credential generation steps
   - Minimum required permissions
   - Rate limit information

3. **Provide Tests**
   - Unit tests with mocked responses
   - Integration test instructions
   - Example configuration

4. **Security Requirements**
   - Support credential vault encryption
   - No hardcoded credentials
   - Secure credential storage

## Provider Comparison Table

| Provider | Users | API Quality | Free Tier | Rate Limits | Complexity |
|----------|-------|-------------|-----------|-------------|------------|
| Cloudflare | ⭐⭐⭐⭐⭐ | Excellent | Yes | 1200/5min | Low |
| Namecheap | ⭐⭐⭐⭐ | Good | No | 50/hour | Medium |
| GoDaddy | ⭐⭐⭐⭐⭐ | Good | No | 60/min | Low |
| DigitalOcean | ⭐⭐⭐ | Excellent | Yes* | 5000/hour | Low |
| Linode | ⭐⭐⭐ | Excellent | Yes* | 800/hour | Low |
| Hetzner | ⭐⭐⭐ | Good | Yes* | 3600/hour | Low |
| Gandi | ⭐⭐ | Good | No | 300/5min | Low |
| Name.com | ⭐⭐ | Good | No | None listed | Low |
| Porkbun | ⭐⭐ | Good | No | None listed | Low |
| DuckDNS | ⭐⭐ | Basic | Yes | None | Very Low |

*Free with service subscription

## Community Input

### How to Request a Provider

Users can request new provider support by:

1. Opening a GitHub issue with:
   - Provider name
   - API documentation link
   - Use case/reason
   - Willingness to test

2. Voting on existing provider requests

3. Contributing implementation (PR)

### Provider Acceptance Criteria

New providers should meet these criteria:

1. **Demand**: At least 10 user requests OR
2. **Market Share**: >1% of domain registrations OR
3. **Strategic**: Fills a geographic/feature gap
4. **Quality**: Has stable, documented API
5. **Maintainable**: Provider unlikely to disappear

## Notes for Implementation

### Cloudflare Specific
- Must handle proxied vs DNS-only selection
- Zone ID lookup may be needed
- Scoped API tokens preferred over global key

### Namecheap Specific
- XML parsing library needed
- IP whitelisting requirement (document clearly)
- Sandbox environment available for testing

### GoDaddy Specific
- OTE (test) environment available
- Shopper ID might be required
- Different endpoints for different TLDs

### DuckDNS Specific
- Extremely simple - just HTTP GET
- Could be implemented as "simplified" provider
- Popular for IoT/home automation

## Maintenance Considerations

1. **Provider Stability**: Choose providers with stable APIs
2. **Documentation**: Each provider needs clear setup docs
3. **Testing**: Integration tests should use provider sandboxes where available
4. **Deprecation**: Plan for provider sunset/changes
5. **Community**: Enable community contributions for long-tail providers

## Conclusion

The provider architecture should remain flexible enough to accommodate these future additions. Priority should be given to:

1. Providers with large user bases (Cloudflare, GoDaddy)
2. Providers we can test (Namecheap)
3. Providers with excellent APIs (DigitalOcean, Linode)
4. Community-requested providers

The goal is to cover 80% of users with 5-6 well-maintained providers, while allowing community contributions for specialized needs.