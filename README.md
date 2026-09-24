# easy-webdav

轻量、单二进制的 WebDAV 服务与网页文件管理面板。/ A small single-binary WebDAV server with a browser file manager.

## Quick start / 快速开始

```sh
./easy-webdav
```

Open `http://127.0.0.1:8080`. On first run create the administrator account. The default data directory is `./data`, with files under `./data/files`.

```sh
./easy-webdav --version        # 版本号 / version, commit, build time
./easy-webdav --print-config   # 打印生效配置（密码已脱敏）/ effective config
./easy-webdav healthcheck      # 探测本地 /healthz，退出码即结果 / container health probe
```

## Docker

```sh
docker run --rm -p 8080:8080 -v easy-webdav-data:/data ghcr.io/lecritus/easy-webdav:latest
```

Use `deploy/docker-compose.yml` or `deploy/easy-webdav.service` for Compose and systemd deployments.

The image is distroless (no shell, no package manager), runs as uid `1000:1000` by default, keeps `/data` as the only writable volume and uses `easy-webdav healthcheck` for its `HEALTHCHECK`. To match a host account, start it as root once and let the binary hand the data directory over:

```sh
docker run --rm -p 8080:8080 --user 0:0 \
  -e EW_PUID=1001 -e EW_PGID=1001 \
  -v easy-webdav-data:/data ghcr.io/lecritus/easy-webdav:latest
```

`EW_PUID`/`EW_PGID` only take effect when the process starts as root: it chowns `/data` (the directory and its immediate children) to that account, drops privileges and then starts serving.

## Configuration / 配置

Precedence is defaults < `config.yaml` < `EW_*` environment variables < command-line flags.

| Setting | Default | Environment / flag |
|---|---|---|
| Listen address | `0.0.0.0:8080` | `EW_LISTEN` / `--listen` |
| Data directory | `./data` | `EW_DATA_DIR` / `--data-dir` |
| Storage directory | `./data/files` | `EW_STORAGE_DIR` / `--storage-dir` |
| Base URL | `/` | `EW_BASE_URL` / `--base-url` |
| TLS certificate/key | unset | `EW_TLS_CERT`, `EW_TLS_KEY` |
| Trusted proxies | empty | `EW_TRUSTED_PROXIES` (CIDR list) |
| Log format/level | `text` / `info` | `EW_LOG_FORMAT`, `EW_LOG_LEVEL` |
| Access log | `errors-and-writes` | `EW_ACCESS_LOG` (`all`, `off`) |
| Bootstrap administrator | unset | `EW_ADMIN_USER`, `EW_ADMIN_PASSWORD` |
| Container uid/gid | unset | `EW_PUID`, `EW_PGID` (root start only) |

Example:

```yaml
listen: 127.0.0.1:8080
data_dir: /var/lib/easy-webdav
storage_dir: /srv/files
base_url: /files/
trusted_proxies:
  - 127.0.0.1/32
log_format: json
access_log: errors-and-writes
```

When `EW_BASE_URL=/files/` is combined with the sample nginx configuration in `deploy/nginx.example.conf`, the panel, the JSON API and `/dav/` are all reachable below that prefix.

## Clients / 客户端

WebDAV URL: `http://host:8080/dav/`. Use it with Windows Explorer, macOS Finder, Linux `davfs2`, or `rclone`. Basic authentication should use TLS or a trusted private network.

## Compatibility / 兼容性

| Client | Status |
|---|---|
| rclone WebDAV | Automated in CI (create / upload / rename / delete) |
| litmus (DAV compliance) | Automated in CI (`basic`, `copymove`, `props`, `locks`, `http`) |
| Browser file manager | Supported |
| Windows 10/11 Explorer | Pending manual verification before the first release |
| macOS Finder | Pending manual verification before the first release |
| davfs2 | Pending manual verification before the first release |
| Mobile app (iOS Files / Solid Explorer) | Pending manual verification before the first release |

Protocol-level behaviour those clients depend on is covered by Go integration tests today: `MOVE` without an `Overwrite` header, collection `href` trailing slashes, `PROPPATCH` on collections returning `207`, `OPTIONS` advertising `DAV: 1, 2` with `MS-Author-Via`, `Range` requests, lock conflicts (`423`) and symlink containment.

## Development / 开发

```sh
go test ./...          # Go unit + integration tests
npm --prefix web test  # frontend unit tests
npm --prefix web run build
```

Local development never requires Docker; the Docker-based compatibility tests and image builds only run in CI.

## Security / 安全建议

Use TLS directly or terminate TLS at a reverse proxy. Keep the data directory private and configure `EW_TRUSTED_PROXIES` only for actual proxy networks. Basic authentication sends the password with every WebDAV request, so the panel shows a warning whenever the request arrives over plain HTTP without a configured TLS termination.
