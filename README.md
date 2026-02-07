# Remnawave Node Go

Go implementation of [remnawave/node](https://github.com/remnawave/node). Single binary, single port, embedded Xray-core, zero dependencies.It was created using Claude Opus 4.5.

## Quick Start

### Script Install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Mikimiya/remnawave-node/main/install.sh)
```

Other commands: `update` | `update-geo` | `uninstall` — append to the command above.

### Docker

```bash
docker run -d --name remnawave-node --restart always \
  -e SECRET_KEY="<YOUR_SECRET_KEY>" \
  -p 2222:2222 \
  ghcr.io/Mikimiya/remnawave-node:latest
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SECRET_KEY` | ✅ | - | Node key from Remnawave panel |
| `NODE_PORT` | No | `2222` | API port |
| `PORT_MAP` | No | - | NAT port mapping. Format: `443:10000,80:10001` |

## Post-install

```bash
remnawave-node-config set-secret-key <key>       # Set SECRET_KEY
remnawave-node-config set-port-map <mapping>     # Set port mapping (NAT)
remnawave-node-config show                       # Show config
systemctl restart remnawave-node                 # Restart service
journalctl -u remnawave-node -f                  # View logs
```

## Why Go?

| | Node.js | Go |
|---|---|---|
| Runtime | Node.js + Xray + Supervisord | Single binary |
| Ports | 4 | 1 |
| Memory | ~150MB | ~30MB |
| NAT Port Mapping | ❌ | ✅ |

## Disclaimer

For personal learning and research only. Do not use for illegal activities. The author bears no responsibility.

## License

AGPL-3.0