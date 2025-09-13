# UDM Installation Analysis

## Current State Analysis

### What's Already Handled:
1. **Boot persistence** - Using unifios-utilities `/data/on_boot.d/` scripts
2. **Cron recreation** - Boot script recreates cron job after each reboot
3. **Binary persistence** - Stored in `/data/dddns/` which survives reboots/updates
4. **Config persistence** - Stored in `/data/.dddns/` which persists

### How It Survives Reboots/Updates:
- **Reboots**: The `/data/on_boot.d/20-dddns.sh` script runs on every boot and:
  - Recreates the symlink in `/usr/local/bin/`
  - Recreates the cron job in `/etc/cron.d/`
  - Restarts cron service
- **Firmware Updates**: The `/data/` partition persists through firmware updates, so binary and config remain intact. Only the boot script execution ensures cron is recreated.

## Remaining Tasks for Streamlined UDM Installation

### Key Issues Found:

1. **GitHub URLs still reference old org** - Scripts have hardcoded `descoped/dddns` which is correct now
2. **Config template mismatch** - The config template in scripts doesn't match actual dddns config structure
3. **Missing --quiet flag** - Cron should use `dddns update --quiet` to reduce log verbosity
4. **Version command output** - Scripts expect specific version output format
5. **AWS credentials handling** - Scripts mention AWS CLI profiles but dddns uses different config

### Recommended Fixes:

1. **Update config template** in both scripts to match actual structure:
   - Remove `aws_profile` (no longer used)
   - Fix field names to match current config
   - Add secure config option

2. **Add --quiet to cron**:
   ```bash
   */30 * * * * root /usr/local/bin/dddns update --quiet >> /var/log/dddns.log 2>&1
   ```

3. **Simplify installation** - Consider having just one install script since they're very similar

4. **Add logrotate** - The scripts mention log rotation but don't implement it

5. **Fix config structure** to match actual implementation:
   ```yaml
   # Current implementation expects:
   aws:
     region: "us-east-1"
     access_key_id: ""
     secret_access_key: ""

   dns:
     hosted_zone_id: ""
     hostname: ""
     ttl: 300

   operations:
     ip_cache_file: "/data/.dddns/last-ip.txt"
     skip_proxy_check: false
   ```

### Script Improvements Needed:

1. **install-udm.sh**:
   - Update config template structure
   - Add --quiet flag to cron job
   - Remove references to aws_profile
   - Add support for secure config

2. **install-dddns-udm.sh**:
   - Update config template structure
   - Add --quiet flag to cron job
   - Consider merging with install-udm.sh to avoid duplication

3. **Boot script (`20-dddns.sh`)**:
   - Add --quiet flag to cron command
   - Add basic log rotation check
   - Consider adding health check

### Persistence Strategy Summary:

The current approach using unifios-utilities is solid:

1. **Binary**: `/data/dddns/dddns` - persists across all updates
2. **Config**: `/data/.dddns/config.yaml` - persists across all updates
3. **Boot Hook**: `/data/on_boot.d/20-dddns.sh` - runs on every boot
4. **Cron Job**: `/etc/cron.d/dddns` - recreated on every boot

This ensures dddns will:
- ✅ Survive device reboots
- ✅ Survive minor firmware updates
- ✅ Survive major firmware updates (may need to reinstall unifios-utilities)
- ✅ Run automatically every 30 minutes via cron

### Next Steps:

1. Update both install scripts with corrected config templates
2. Add --quiet flag to reduce log verbosity
3. Test installation on actual UDM hardware
4. Consider adding health monitoring/alerting
5. Document the secure config option for AWS credentials

The scripts are well-designed for UDM persistence using the unifios-utilities pattern. The main work needed is updating them to match the current dddns implementation.