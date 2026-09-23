## Purpose

定义项目的交付形态与部署契约：单二进制、Docker 镜像、systemd 示例、日志、健康检查、反向代理兼容与发布流程，保证"部署方便"。

## ADDED Requirements

### Requirement: Single static binary
项目 SHALL 为 Linux、macOS、Windows 的 amd64 与 arm64 各发布一个静态链接的可执行文件，前端资源 MUST 内嵌其中，运行时 MUST NOT 依赖外部运行时、动态库或额外文件。

#### Scenario: Run on clean machine
- **WHEN** 用户将二进制复制到一台未安装任何依赖的机器上并执行
- **THEN** 服务正常启动，面板可访问

#### Scenario: Version flag
- **WHEN** 用户执行 `easy-webdav --version`
- **THEN** 输出版本号、Git 提交与构建时间

### Requirement: Docker image
项目 SHALL 发布多架构（amd64、arm64）Docker 镜像至 `ghcr.io/lecritus/easy-webdav`。容器 MUST 以非 root 用户运行，数据目录 MUST 为 `/data`，默认监听 `8080`；镜像压缩后大小 MUST 小于 30 MB。

#### Scenario: One-line docker run
- **WHEN** 用户执行 `docker run -d -p 8080:8080 -v ./data:/data ghcr.io/lecritus/easy-webdav`
- **THEN** 容器启动，浏览器访问 `http://localhost:8080` 显示首次运行引导

#### Scenario: PUID/PGID support
- **WHEN** 用户设置环境变量 `PUID=1000` 与 `PGID=1000`
- **THEN** 容器内进程以该 uid/gid 运行，挂载卷中新建的文件归属该 uid/gid

### Requirement: Compose example
仓库 SHALL 提供 `docker-compose.yml` 示例，仅包含服务本身与一个数据卷，用户执行 `docker compose up -d` 即可启动。

#### Scenario: Compose up
- **WHEN** 用户在仓库根目录执行 `docker compose up -d`
- **THEN** 服务在 8080 端口可用，数据持久化到命名卷或本地目录

### Requirement: Bare-metal service example
仓库 SHALL 提供 systemd 单元文件示例，以专用非 root 用户运行二进制，并将数据目录指向 `/var/lib/easy-webdav`。Windows 与 macOS 的服务化 MUST 在文档中给出推荐做法，不提供文件。

#### Scenario: Install as systemd service
- **WHEN** 用户按 README 步骤复制二进制与单元文件并执行 `systemctl enable --now easy-webdav`
- **THEN** 服务随系统启动，日志可通过 `journalctl -u easy-webdav` 查看

### Requirement: Logging
系统 SHALL 将日志写到标准输出与标准错误，MUST NOT 自行写日志文件或轮转。默认格式为文本、级别为 info；`EW_LOG_FORMAT=json` MUST 切换为每行一个 JSON 对象；`EW_LOG_LEVEL` MUST 接受 `debug`、`info`、`warn`、`error`。访问日志默认 MUST 只记录响应状态为 4xx/5xx 的请求与所有写操作（PUT、DELETE、MKCOL、COPY、MOVE、PROPPATCH、LOCK、UNLOCK 及对应的面板 API）；`EW_ACCESS_LOG=all` MUST 记录全部请求；`EW_ACCESS_LOG=off` MUST 关闭访问日志。日志 MUST NOT 包含密码、会话令牌或 Authorization 头内容。

#### Scenario: Default access log filters reads
- **WHEN** 客户端成功执行一次 PROPFIND 与一次 PUT
- **THEN** 日志只出现 PUT 记录

#### Scenario: JSON format
- **WHEN** 设置 `EW_LOG_FORMAT=json`
- **THEN** 每行日志为可被 JSON 解析的对象，含时间、级别、消息字段

#### Scenario: Credentials never logged
- **WHEN** 一次失败的 Basic 认证请求被记录
- **THEN** 日志包含用户名与来源 IP，不包含密码或 Authorization 头原文

### Requirement: Health check
系统 SHALL 提供 `GET /healthz`，无需认证，服务正常时返回 `200` 与纯文本 `ok`，数据库或存储目录不可用时返回 `503`。Docker 镜像 MUST 声明基于该端点的 HEALTHCHECK。

#### Scenario: Healthy
- **WHEN** 服务正常运行时请求 `/healthz`
- **THEN** 返回 `200` 与 `ok`

#### Scenario: Storage unavailable
- **WHEN** 存储目录被卸载或不可写
- **THEN** `/healthz` 返回 `503`

### Requirement: Reverse proxy and sub-path support
系统 SHALL 在反向代理后正常工作：MUST 仅信任 `EW_TRUSTED_PROXIES`（CIDR 列表，默认空）中来源的代理头（`X-Forwarded-For`、`X-Forwarded-Proto`）以获取真实 IP 与协议；SHALL 支持通过 `EW_BASE_URL` 配置子路径（例如 `/webdav`），面板与 WebDAV 端点在该子路径下正确工作。

#### Scenario: Behind nginx with sub-path
- **WHEN** 服务配置 `EW_BASE_URL=/webdav` 并由 nginx 代理 `/webdav/` 到服务
- **THEN** 面板在 `https://host/webdav/` 可用，WebDAV 客户端可挂载 `https://host/webdav/dav/`

#### Scenario: Proxy headers only trusted from configured sources
- **WHEN** 未配置信任代理，客户端直接请求并伪造 `X-Forwarded-For`
- **THEN** 系统忽略该头，登录限速以真实连接 IP 计算

### Requirement: Optional built-in TLS
系统 SHALL 支持通过配置指定证书与私钥文件直接提供 HTTPS，未配置时仅提供 HTTP。

#### Scenario: Start with TLS
- **WHEN** 配置了 `EW_TLS_CERT` 与 `EW_TLS_KEY`
- **THEN** 服务以 HTTPS 监听，会话 Cookie 带 Secure 标记

### Requirement: Graceful shutdown and data safety
收到 SIGINT 或 SIGTERM 时，系统 MUST 停止接受新连接，等待进行中的请求完成（上限 30 秒），然后退出；正在进行的上传若未完成 MUST NOT 留下半成品文件。

#### Scenario: Stop during upload
- **WHEN** 上传进行中时服务收到 SIGTERM
- **THEN** 服务等待该上传完成或超时后中止并清理临时文件，随后退出

### Requirement: Release automation
仓库 SHALL 通过 CI 在打 tag 时自动构建全部平台二进制与 Docker 镜像，生成校验和文件，并发布到 GitHub Releases 与容器镜像仓库。Release notes MUST 由提交记录按 Conventional Commits 类型自动生成。

#### Scenario: Tag triggers release
- **WHEN** 维护者推送 `v1.2.3` 标签
- **THEN** CI 产出 6 个平台的二进制、`checksums.txt`、多架构镜像 `:v1.2.3` 与 `:latest`，以及按 feat/fix 分组的 release notes
