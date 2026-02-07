# Configuration Guide

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SECRET_KEY` | ✅ | - | Base64 encoded JSON from Remnawave Panel |
| `NODE_PORT` | ❌ | 2222 | Main API server port |
| `DISABLE_HASHED_SET_CHECK` | ❌ | false | Disable config change detection |

## SECRET_KEY Structure

The `SECRET_KEY` is a Base64 encoded JSON containing:

```json
{
  "nodeCertPem": "-----BEGIN CERTIFICATE-----...",
  "nodeKeyPem": "-----BEGIN PRIVATE KEY-----...",
  "caCertPem": "-----BEGIN CERTIFICATE-----...",
  "jwtPublicKey": "-----BEGIN PUBLIC KEY-----..."
}
```

You get this from the Remnawave Panel when adding a new node.

## Ports

| Port | Description |
|------|-------------|
| `NODE_PORT` (default 2222) | Main API (mTLS) - only port needed |

Unlike the Node.js version, no additional ports are required:
- ❌ Port 61000 (Xray gRPC) - Not needed, Xray is embedded
- ❌ Port 61001 (Internal API) - Not needed, merged into main API
- ❌ Port 61002 (Supervisord) - Not needed, no process management required

## Docker Usage

```bash
docker run -d \
  -e SECRET_KEY="your_base64_key" \
  -e NODE_PORT="2222" \
  -p 2222:2222 \
  ghcr.io/Mikimiya/remnawave-node:latest
```

## Systemd Service

```ini
[Unit]
Description=Remnawave Node Go
After=network.target

[Service]
Type=simple
EnvironmentFile=/etc/remnawave-node/env
ExecStart=/usr/local/bin/remnawave-node
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```
