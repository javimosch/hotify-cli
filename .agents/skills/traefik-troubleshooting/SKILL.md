# Traefik Troubleshooting Skill

## Overview
This skill covers common Traefik configuration issues and solutions when deploying apps via hotify-cli, particularly around ACME SSL certificate generation and dynamic configuration.

## Common Issues and Solutions

### ACME "Domain Not Defined" Error

**Symptom**: Traefik logs show error like:
```
error="error while parsing rule Host(app.example.com): app.example.com is not defined"
```

**Root Cause**: The domain is not properly specified in the Traefik dynamic configuration for ACME to recognize it.

**Solution**: Update `/etc/traefik/dynamic.yml` to include explicit domain specification in TLS configuration:

```yaml
http:
  routers:
    app:
      rule: "Host(`app.example.com`)"
      service: app
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
        domains:
          - main: app.example.com
  services:
    app:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:PORT"
```

**Key Points**:
- Use backticks around domain name in Host() rule
- Add explicit `domains:` section under `tls:`
- Specify the main domain explicitly

### ACME Challenge Conflicts

**Symptom**: Traefik fails to generate SSL certificates, showing errors about challenge validation.

**Root Cause**: Having both DNS and HTTP challenges configured in Traefik's ACME configuration can cause conflicts.

**Solution**: Use only one challenge type in `/etc/traefik/traefik.yml`:

**HTTP Challenge (simpler, default)**:
```yaml
certificatesResolvers:
  letsencrypt:
    acme:
      email: admin@example.com
      storage: /etc/traefik/acme.json
      httpChallenge:
        entryPoint: web
```

**DNS Challenge (requires Cloudflare token)**:
```yaml
certificatesResolvers:
  letsencrypt:
    acme:
      email: admin@example.com
      storage: /etc/traefik/acme.json
      dnsChallenge:
        provider: cloudflare
        delayBeforeCheck: 30
```

**Recommendation**: Start with HTTP challenge. Only use DNS challenge if HTTP port 80 is blocked or you need wildcard certificates.

### Traefik Service Reload Not Supported

**Symptom**: `sudo systemctl reload traefik` fails with "reload is not supported for this unit" error.

**Root Cause**: The Traefik systemd service is not configured to support reload signals.

**Solution**: Use restart instead of reload:
```bash
sudo systemctl restart traefik
```

**Note**: This causes brief downtime (typically 2-5 seconds).

### Dynamic Configuration Not Picked Up

**Symptom**: Changes to `/etc/traefik/dynamic.yml` don't take effect.

**Root Cause**: Traefik needs to be restarted to pick up dynamic configuration changes, even with `watch: true` enabled.

**Solution**: Always restart Traefik after modifying dynamic configuration:
```bash
sudo systemctl restart traefik
```

### DNS Record Already Exists

**Symptom**: Cloudflare API returns error when trying to create DNS record.

**Root Cause**: The DNS record already exists from a previous setup attempt.

**Solution**: 
1. Check if record exists in Cloudflare dashboard
2. If it exists and points to correct IP, skip DNS setup
3. If it exists but points to wrong IP, update it via Cloudflare dashboard or API

## Troubleshooting Workflow

1. **Check Traefik Status**:
   ```bash
   sudo systemctl status traefik
   ```

2. **Check Traefik Logs**:
   ```bash
   sudo journalctl -u traefik -f
   ```

3. **Verify Dynamic Configuration**:
   ```bash
   cat /etc/traefik/dynamic.yml
   ```

4. **Verify Main Configuration**:
   ```bash
   cat /etc/traefik/traefik.yml
   ```

5. **Test DNS Resolution**:
   ```bash
   nslookup app.example.com
   ```

6. **Test HTTP Access**:
   ```bash
   curl -I http://app.example.com
   ```

7. **Test HTTPS Access**:
   ```bash
   curl -I https://app.example.com
   ```

8. **Restart Traefik**:
   ```bash
   sudo systemctl restart traefik
   ```

## hotify-cli Specific Considerations

### Current Limitations

1. **ACME Challenge Type**: hotify-cli currently doesn't allow choosing between HTTP and DNS challenges. Default is DNS challenge with Cloudflare.

2. **Dynamic Configuration**: hotify-cli generates dynamic configuration but may not include explicit domain specification for ACME.

3. **Service Restart**: hotify-cli attempts reload but should fall back to restart when reload is not supported.

### Recommended Improvements for v2.3.0

1. Add `--challenge-type` flag to `setup-traefik` command (http/dns)
2. Include explicit domain specification in generated dynamic configuration
3. Detect when reload is not supported and use restart instead
4. Add DNS record existence check before attempting creation
5. Add configuration validation before applying changes

## Testing Checklist

- [ ] Traefik starts without errors
- [ ] No ACME domain errors in logs
- [ ] HTTP to HTTPS redirect works
- [ ] HTTPS access returns valid SSL certificate
- [ ] Application is accessible via domain
- [ ] SSL certificate auto-renews before expiry

## Common Commands

```bash
# Restart Traefik
sudo systemctl restart traefik

# Check Traefik status
sudo systemctl status traefik

# View Traefik logs
sudo journalctl -u traefik -f

# View current configuration
cat /etc/traefik/traefik.yml
cat /etc/traefik/dynamic.yml

# Test domain resolution
nslookup app.example.com

# Test HTTP access
curl -I http://app.example.com

# Test HTTPS access
curl -I https://app.example.com
```

## Related Files

- `/etc/traefik/traefik.yml` - Main Traefik configuration
- `/etc/traefik/dynamic.yml` - Dynamic routing configuration
- `/etc/traefik/acme.json` - SSL certificate storage
- `/etc/traefik/cloudflare.env` - Cloudflare API token
- `/etc/systemd/system/traefik.service` - Systemd service definition
