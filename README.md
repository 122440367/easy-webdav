# easy-webdav

轻量、单二进制的 WebDAV 服务与网页文件管理面板。/ A small single-binary WebDAV server with a browser file manager.

## Quick start / 快速开始

```sh
./easy-webdav
```

Open `http://127.0.0.1:8080`. On first run create the administrator account. The default data directory is `./data`, with files under `./data/files`.

## Docker

```sh
docker run --rm -p 8080:8080 -v easy-webdav-data:/data ghcr.io/lecritus/easy-webdav:latest
```

Use `deploy/docker-compose.yml` or `deploy/easy-webdav.service` for Compose and systemd deployments.

## Configuration / 配置

Precedence is defaults < `config.yaml` < `EW_*` environment variables < command-line flags.

| Setting | Default | Environment / flag |
|---|---|---|
| Listen address | `0.0.0.0:8080` | `EW_LISTEN` / `--listen` |
| Data directory | `./data` | `EW_DATA_DIR` / `--data-dir` |
| Storage directory | `./data/files` | `EW_STORAGE_DIR` / `--storage-dir` |
| Base URL | `/` | `EW_BASE_URL` / `--base-url` |
| TLS certificate/key | unset | `EW_TLS_CERT`, `EW_TLS_KEY` |
| Trusted proxies | empty | `EW_TRUSTED_PROXIES` |
| Log format/level | `text` / `info` | `EW_LOG_FORMAT`, `EW_LOG_LEVEL` |
| Access log | `errors-and-writes` | `EW_ACCESS_LOG` |

Example:

```yaml
listen: 127.0.0.1:8080
data_dir: /var/lib/easy-webdav
storage_dir: /srv/files
base_url: /files/
```

## Clients / 客户端

WebDAV URL: `http://host:8080/dav/`. Use it with Windows Explorer, macOS Finder, Linux `davfs2`, or `rclone`. Basic authentication should use TLS or a trusted private network.

## Compatibility / 兼容性

| Client | Status |
|---|---|
| Browser file manager | Supported |
| rclone WebDAV | CI target |
| Windows Explorer | Manual verification required |
| macOS Finder | Manual verification required |
| davfs2 | Manual verification required |

## Security / 安全建议

Use TLS directly or terminate TLS at a reverse proxy. Keep the data directory private and configure `EW_TRUSTED_PROXIES` only for actual proxy networks.
