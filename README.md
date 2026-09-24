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

## Command line / 命令行

`easy-webdav [serve] [flags]` starts the server; the flags above are equivalent to the `version` and `config print` subcommands. WebDAV and the panel need nothing else.

```sh
easy-webdav serve              # 与服务端等价 / same as running the server
easy-webdav version            # 等价 --version
easy-webdav config print       # 等价 --print-config（密码脱敏）
easy-webdav healthcheck        # 探活，容器 HEALTHCHECK 使用
```

Offline account management runs without the server and is how an operator recovers a lost administrator password:

```sh
easy-webdav admin create --username admin --admin
easy-webdav admin list
easy-webdav admin reset-password --username admin
```

| Command | Flags | Exit codes |
|---|---|---|
| `admin create` | `--username`、`--password`、`--admin`、`--role admin\|user`、`--root-dir`、`--permission read\|readwrite`、`--quota`、`--config`、`--data-dir`、`--storage-dir` | `0` 成功、`1` 冲突/弱密码等运行错误、`2` 用法或校验错误 |
| `admin reset-password` | `--username`、`--password`、`--config`、`--data-dir`、`--storage-dir`（同时吊销该用户全部会话） | 同上 |
| `admin list` | `--config`、`--data-dir`、`--storage-dir` | `0` 成功、`2` 配置错误 |

密码来源优先级：`--password` > `EW_ADMIN_PASSWORD` > 交互式提示（终端上不回显）。非交互环境（管道或 CI）从标准输入读取一行，例如 `echo "secret123" | easy-webdav admin create --username ci`。未提供 `--permission`/`--quota` 时沿用面板里的运行时默认值，`--root-dir` 缺省等于用户名。

## Docker

```sh
docker run --rm -p 8080:8080 -v easy-webdav-data:/data ghcr.io/122440367/easy-webdav:latest
```

Use `deploy/docker-compose.yml` or `deploy/easy-webdav.service` for Compose and systemd deployments.

The image is distroless (no shell, no package manager), runs as uid `1000:1000` by default, keeps `/data` as the only writable volume and uses `easy-webdav healthcheck` for its `HEALTHCHECK`. To match a host account, start it as root once and let the binary hand the data directory over:

```sh
docker run --rm -p 8080:8080 --user 0:0 \
  -e EW_PUID=1001 -e EW_PGID=1001 \
  -v easy-webdav-data:/data ghcr.io/122440367/easy-webdav:latest
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

`EW_BASE_URL` only changes the `<base href>` injected into the panel; the reverse proxy is what adds or strips a prefix. `deploy/nginx.example.conf` contains both layouts, including the WebDAV block (unbuffered bodies, no size limit, long timeouts) that a sub-path deployment needs. Set `EW_TRUSTED_PROXIES` to the proxy address so real client IPs and https detection work.

## Clients / 客户端

WebDAV URL: `http://host:8080/dav/`. Use it with Windows Explorer, macOS Finder, Linux `davfs2`, or `rclone`. Basic authentication should use TLS or a trusted private network.

## Compatibility / 兼容性

| Client | Status |
|---|---|
| rclone WebDAV | Automated in CI (create / upload / rename / delete) |
| litmus (DAV compliance) | `basic`, `copymove` and `http` gated in CI; `locks` and `props` report documented limitations |
| Browser file manager | Supported |
| Windows 10/11 Explorer | Pending manual verification before the first release |
| macOS Finder | Pending manual verification before the first release |
| davfs2 | Pending manual verification before the first release |
| Mobile app (iOS Files / Solid Explorer) | Pending manual verification before the first release |

Protocol-level behaviour those clients depend on is covered by Go integration tests today: `MOVE` without an `Overwrite` header, collection `href` trailing slashes, `PROPPATCH` on collections returning `207`, `OPTIONS` advertising `DAV: 1, 2` with `MS-Author-Via`, `Range` requests, lock conflicts (`423`) and symlink containment.

### litmus results / litmus 结果

CI compiles litmus 0.13 on the runner and runs it against the built binary. Groups the specification covers are enforced; the remaining failures are deliberate design choices or upstream `x/net/webdav` limitations, listed here with their root cause:

| Group | Result | Notes |
|---|---|---|
| `basic` | 16/16 | gated |
| `copymove` | 13/13 | gated, including the Finder `Overwrite` patch |
| `http` | gated | protocol details such as `HEAD`, `ETag` and `Range` |
| `locks` | 30/34 | both `owner_modify` cases patch a dead property; `lock_shared` answers `501`; `fail_complex_cond_put` passes because `If` conditions carrying an ETag are not evaluated |
| `props` | 10/14 | no dead-property storage (see design patch 3) and `propfind_invalid2` expects a `400` for an invalid namespace declaration that `x/net` tolerates |

Known limitations, none of which affect the supported clients (Windows Explorer, Finder, davfs2, rclone, mobile apps):

- **Dead properties.** `PROPPATCH` answers `207` with a `403` propstat instead of storing arbitrary properties. That is what the Windows Mini-Redirector needs in order to continue, and no supported client reads the values back.
- **Shared locks.** `LOCK` with `<shared/>` returns `501 Not Implemented`; only exclusive write locks are implemented, matching the specification. Writing to a locked resource without the token still returns `423`.
- **`If` header conditions.** Only lock tokens are evaluated. ETag and `Not` conditions inside `If` are ignored because the upstream in-memory lock system does not implement them; the project's own conformance tests pin the behaviour that is promised (see `internal/webdav/conformance_test.go`).

## Release checklist / 发布前检查清单

CI already covers unit tests, the DAV conformance cases and the Docker-based `litmus`/`rclone` runs. Before tagging a release, walk through the items below on a machine with a browser, Docker and the target clients, and record the outcome in the compatibility table above.

**桌面与移动客户端（WebDAV 端点，读写四项操作：建文件夹、上传、重命名、删除）**

- [ ] Windows 10/11：资源管理器「映射网络驱动器」，地址用 `\\host@SSL@443\dav`（http 用 `\\host@8080\dav`）；检查 `.` 开头文件、Office 另存与中文文件名
- [ ] macOS 访达：`⌘K` 连接同一地址；确认访达发送的不带 `Overwrite` 头的 MOVE 成功、`._` 元数据文件不报错
- [ ] Linux davfs2：`mount -t davfs https://host/dav/ /mnt/x`；确认大文件上传与 `df` 容量显示
- [ ] 移动端：iOS「文件」或 Android Solid Explorer 添加 WebDAV 账号；确认浏览、上传照片、重命名
- [ ] `rclone`：`rclone config create …` 后执行 `lsd`/`copy`/`move`/`delete` 全部成功

**部署形态**

- [ ] 干净机器直接运行二进制：首次访问进入引导页，创建管理员后 `/files` 可用
- [ ] `docker run --rm -p 8080:8080 -v easy-webdav-data:/data ghcr.io/<owner>/easy-webdav:<tag>`，容器 `healthy`
- [ ] `docker compose up -d`（`deploy/docker-compose.yml`）可用；`EW_PUID`/`EW_PGID` 以 root 启动时 `/data` 归属正确
- [ ] systemd 单元（`deploy/easy-webdav.service`）启动正常，`systemctl restart` 后数据与用量保留
- [ ] nginx 子路径部署（`deploy/nginx.example.conf` Layout B）：面板、API 与 `/files/dav/` 都可用
- [ ] 镜像体积 < 30 MB（`docker images`），并记录实际数值
- [ ] 日志三种模式：默认只记 4xx/5xx 与写操作、`EW_ACCESS_LOG=all`、`EW_ACCESS_LOG=off`；确认不含密码、`Authorization` 与 Cookie

**发布产物**

- [ ] 打 tag 后 GitHub Release 包含六平台二进制、`checksums.txt` 与自动生成的 release notes
- [ ] `ghcr.io/<owner>/easy-webdav` 推送了 `vX.Y.Z`、`vX.Y` 与 `latest` 三个标签，多架构清单同时包含 `linux/amd64` 与 `linux/arm64`
- [ ] `:edge` 由 main 分支构建推送（`.github/workflows/edge.yml`）

## Development / 开发

```sh
go test ./...          # Go unit + integration tests
npm --prefix web test  # frontend unit tests
npm --prefix web run build
```

Local development never requires Docker; the Docker-based compatibility tests and image builds only run in CI.

## Security / 安全建议

Use TLS directly or terminate TLS at a reverse proxy. Keep the data directory private and configure `EW_TRUSTED_PROXIES` only for actual proxy networks. Basic authentication sends the password with every WebDAV request, so the panel shows a warning whenever the request arrives over plain HTTP without a configured TLS termination.
