## Why

现有 WebDAV 方案要么只有协议没有面板（nginx、Apache mod_dav、dufs），要么面板太重、配置项过多（Nextcloud、SFTPGo）。个人和小团队想要的是"下载一个文件、启动、打开浏览器就能用"的 WebDAV 服务，同时能在网页里管用户、看文件、控配额。目前没有一个专注于此的轻量开源项目，这是 easy-webdav 的定位。

目标用户是个人自用 NAS/VPS 与小团队或家庭，规模 1 到 50 个用户。50 人以上、需要对接企业目录的场景不在目标内，但接口层留口。

## What Changes

这是一个从零开始的开源项目，本变更定义第一个可用版本（MVP）。

- 新增 Go 单二进制服务：内嵌前端静态资源，启动即提供 WebDAV 端点和管理面板，无外部依赖。
- 新增零配置启动与首次运行引导：默认数据目录、默认端口、首次访问面板时创建管理员账号；可通过 YAML 配置文件、环境变量或命令行参数覆盖配置，配置文件按固定位置查找、不自动生成。
- 新增多用户管理：管理员在面板中创建、编辑、禁用、删除用户，为每个用户指定根目录（目录隔离）、读或读写权限。管理员自己同样拥有根目录，可挂载 WebDAV。
- 新增面板运行时设置：新用户默认配额、默认权限、站点名称，管理员在面板修改即时生效。
- 新增 WebDAV 协议服务：基于 HTTP Basic 认证按用户校验，每个用户只能看到并操作自己的根目录，支持 Windows 资源管理器、macOS 访达、Linux 客户端和常见移动端 App 挂载。HTTP 下允许使用，面板显示安全警告。
- 新增网页文件浏览器：登录用户在浏览器内浏览目录、分块上传（含拖拽、多文件、文件夹结构、会话内失败重试）、下载、新建文件夹、重命名、移动、复制、删除、预览图片/文本/PDF/音视频。默认隐藏点开头的文件。
- 新增管理员跨用户浏览：管理员可从用户管理页进入任意用户的根目录进行读写操作，页面显示明确提示。
- 新增中英双语界面与明暗主题：跟随浏览器语言和系统主题，均可手动切换。
- 新增配额与用量统计：按用户设置磁盘配额，写入超限时拒绝并返回标准错误；面板展示每个用户与整体的占用情况。
- 新增结构化日志：输出到标准输出，文本或 JSON 格式，级别可调，访问日志默认只记录错误与写操作。
- 新增部署交付物：Docker 镜像（`docker run` 一行启动）、`docker-compose.yml` 示例、systemd 单元示例、各平台预编译二进制、GitHub Actions 自动发布。

## Capabilities

### New Capabilities

- `bootstrap-config`: 零配置启动、配置来源与优先级（默认值 < 配置文件 < 环境变量 < 命令行参数）、配置文件查找位置、数据目录布局、首次运行创建管理员。
- `admin-auth`: 管理面板与网页文件浏览器的登录、会话、退出，管理员与普通用户的角色区分，HTTP 访问时的安全警告。
- `user-management`: 用户的创建、编辑、禁用、删除；根目录指定与目录隔离；读/读写权限；面板运行时设置。
- `webdav-server`: WebDAV 协议端点、Basic 认证、按用户的根目录与权限执行、锁支持、符号链接策略、主流客户端兼容性。
- `web-file-browser`: 浏览器内的文件浏览、分块上传、下载、目录操作、文件预览、管理员跨用户浏览、界面语言与主题。
- `quota-usage`: 用户配额设置与写入时的强制执行、用量统计与面板展示。
- `deployment`: 单二进制交付、Docker 镜像与 compose 示例、systemd 示例、日志、健康检查端点、发布流程。

### Modified Capabilities

无。项目为全新项目，`openspec/specs/` 下尚无任何规范。

## Impact

- 仓库：`github.com/lecritus/easy-webdav`，默认分支 `main`，MIT 许可。新建整个仓库结构，包含 Go 后端（`cmd/`、`internal/`）、前端工程（`web/`）、Dockerfile、CI 工作流。
- 依赖：Go 1.25 以上；协议层将 `golang.org/x/net/webdav` 源码复制进仓库（BSD-3 许可）并直接打补丁修复已知客户端兼容问题，不作为外部依赖引用；SQLite 使用纯 Go 驱动 `modernc.org/sqlite`，避免 CGO；前端 Vue 3 + TypeScript + Vite + Naive UI + vue-i18n，Node 22 以上构建，产物通过 `embed` 打进二进制。
- 对外接口：
  - `/dav/` WebDAV 端点（Basic 认证）。
  - `/api/v1/` 管理与文件浏览 JSON API（会话 Cookie 认证）。
  - `/` 管理面板与文件浏览器页面。
  - `/healthz` 健康检查。
- 运行环境：Linux、macOS、Windows 三平台 amd64/arm64 二进制；Docker 镜像 `ghcr.io/lecritus/easy-webdav`，基于 distroless 或 scratch。
- 开发环境约束：本地不依赖 Docker，Docker 相关的兼容性测试与镜像构建只在 CI 中执行。
- 非目标（本次不做）：分享链接、回收站、多存储后端（S3 等）、OAuth/LDAP 登录、单用户多虚拟目录、文件版本历史、在线编辑、图片缩略图、Prometheus 指标、API Token、跨页面刷新的断点续传、用户自助注册。这些留给后续变更。
