## Purpose

定义 WebDAV 协议端点的认证、按用户的根目录与权限执行、锁支持、符号链接策略，以及对主流桌面与移动客户端的兼容性要求。

## ADDED Requirements

### Requirement: WebDAV endpoint
系统 SHALL 在 `/dav/` 路径提供 WebDAV Class 1 与 Class 2 服务，支持 OPTIONS、GET、HEAD、PUT、DELETE、MKCOL、COPY、MOVE、PROPFIND、PROPPATCH、LOCK、UNLOCK 方法。用户的根目录 MUST 映射为 `/dav/`，用户看不到自己的用户名或其他用户的路径。

#### Scenario: OPTIONS advertises DAV classes
- **WHEN** 客户端向 `/dav/` 发送 OPTIONS
- **THEN** 响应包含 `DAV: 1, 2` 头以及支持的方法列表

#### Scenario: PROPFIND lists root
- **WHEN** 已认证用户向 `/dav/` 发送 `Depth: 1` 的 PROPFIND
- **THEN** 响应为 `207 Multi-Status`，列出该用户根目录下的直接子项，集合类型资源的 href MUST 以 `/` 结尾

#### Scenario: Upload and download round-trip
- **WHEN** 用户 PUT 一个文件到 `/dav/a/b.txt`（父目录已存在），随后 GET 同一路径
- **THEN** PUT 返回 `201`，GET 返回 `200` 且内容与上传内容字节一致

### Requirement: Basic authentication per user
WebDAV 端点 SHALL 使用 HTTP Basic 认证，凭据为用户管理中的用户名与密码。未认证或凭据错误的请求 MUST 返回 `401` 并附带 `WWW-Authenticate: Basic realm="easy-webdav"`。被禁用的用户 MUST 认证失败。明文 HTTP 下 MUST 允许认证。

#### Scenario: Missing credentials
- **WHEN** 客户端不带 Authorization 头请求 `/dav/`
- **THEN** 系统返回 `401` 与 `WWW-Authenticate` 头

#### Scenario: Wrong password
- **WHEN** 客户端使用错误密码
- **THEN** 系统返回 `401`，且连续失败受与面板登录相同的限速策略约束

### Requirement: Permission enforcement over WebDAV
`read` 权限用户的 PUT、DELETE、MKCOL、COPY、MOVE、PROPPATCH、LOCK 请求 MUST 返回 `403`。所有用户的所有请求 MUST 被限制在其根目录内。

#### Scenario: Read-only MKCOL
- **WHEN** `read` 用户发送 MKCOL
- **THEN** 系统返回 `403` 且不创建目录

#### Scenario: COPY across root boundary
- **WHEN** 用户发送 COPY，其 Destination 头指向经规范化后位于根目录之外的路径
- **THEN** 系统返回 `403` 或 `502`，不产生任何文件

### Requirement: Locking
系统 SHALL 支持 LOCK 与 UNLOCK，包括独占写锁与超时；锁 MUST 在服务重启后不要求持久化。持有锁的资源在其他会话尝试写入时 MUST 返回 `423 Locked`。

#### Scenario: Lock then write from another client
- **WHEN** 客户端 A 对文件加独占锁，客户端 B 不带锁令牌尝试 PUT 同一文件
- **THEN** B 收到 `423`，A 携带锁令牌的 PUT 成功

### Requirement: Client compatibility
系统 SHALL 能被以下客户端以读写方式成功挂载并完成"创建文件夹、上传、重命名、删除"四项操作：Windows 10/11 资源管理器映射网络驱动器、macOS 访达"连接服务器"、Linux davfs2、rclone、以及至少一款移动端客户端（如 iOS Files 或 Android Solid Explorer）。已知需要处理的客户端行为包括：访达发送不带 `Overwrite` 头的 MOVE、Windows Mini-Redirector 对集合发送 PROPPATCH、Windows 要求 OPTIONS 响应含 `MS-Author-Via: DAV`。

#### Scenario: Windows Explorer mount
- **WHEN** Windows 用户通过"映射网络驱动器"连接 `http(s)://host:port/dav/`
- **THEN** 驱动器成功挂载，可以创建文件夹、拖入文件、重命名与删除

#### Scenario: macOS Finder mount
- **WHEN** macOS 用户通过访达"连接服务器"连接同一地址
- **THEN** 卷成功挂载并可读写，`._` 元数据文件不会导致操作失败

#### Scenario: Finder rename without Overwrite header
- **WHEN** 访达对一个文件发送 MOVE 且不带 `Overwrite` 头，目标不存在
- **THEN** 系统返回 `201`，重命名成功

#### Scenario: Windows PROPPATCH on collection
- **WHEN** Windows 客户端对目录发送 PROPPATCH 设置 `Win32LastModifiedTime` 等属性
- **THEN** 系统返回 `207` 而非 `500`，目录操作继续正常

### Requirement: Range and conditional requests
GET SHALL 支持 `Range` 请求并返回 `206`；GET 与 HEAD SHALL 返回 `ETag` 与 `Last-Modified`，并正确处理 `If-None-Match` 与 `If-Modified-Since`。

#### Scenario: Partial download
- **WHEN** 客户端带 `Range: bytes=0-99` 请求一个 1000 字节文件
- **THEN** 系统返回 `206`，正文长度为 100，`Content-Range` 正确

### Requirement: Large upload streaming
系统 SHALL 以流式方式处理 PUT 请求，MUST NOT 将整个请求体缓存在内存中；单文件大小上限仅受磁盘空间与用户配额限制。

#### Scenario: Upload larger than available memory
- **WHEN** 用户上传一个远大于进程可用内存的文件
- **THEN** 上传成功，进程常驻内存不随文件大小线性增长

### Requirement: Symlink and internal file handling
系统 SHALL 允许跟随用户根目录内的符号链接，但链接解析后的真实路径 MUST 仍位于该用户根目录内；解析后逃逸出根目录的链接 MUST 视为不存在。存储根目录中的系统内部文件与目录（数据库、配置、上传临时目录）MUST 位于任何用户根目录之外，不可通过 WebDAV 访问。

#### Scenario: Symlink inside root is followed
- **WHEN** 用户根目录内的 `docs` 是指向同一根目录下 `archive/docs` 的符号链接，用户对 `/dav/docs/` 发送 PROPFIND
- **THEN** 系统返回 `archive/docs` 的内容

#### Scenario: Symlink escape blocked
- **WHEN** 用户根目录内存在指向根目录之外的符号链接，用户请求该链接路径
- **THEN** 系统返回 `404`，不读取目标内容

#### Scenario: Symlink chain resolved fully
- **WHEN** 链接 A 指向根内的链接 B，B 指向根外
- **THEN** 系统返回 `404`
