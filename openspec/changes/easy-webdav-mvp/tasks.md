## 1. 仓库骨架与工具链

- [x] 1.1 `git init`，默认分支 `main`；`go mod init github.com/lecritus/easy-webdav`，`go.mod` 声明 Go 1.25；创建 design.md D9 中的目录结构，添加 `cmd/easy-webdav/main.go` 空壳，`go build` 通过
- [x] 1.2 添加 `.gitignore`、`LICENSE`（MIT）、`README.md` 占位、`Makefile`（build/test/lint/web 目标；Docker 相关目标检测无 Docker 时跳过并提示）
- [x] 1.3 初始化 `web/` Vite + Vue 3 + TypeScript 工程，引入 Naive UI、vue-i18n、Pinia、vue-router；`package.json` 声明 Node 22 以上；`npm run build` 产出到 `internal/web/dist`
- [x] 1.4 实现 `internal/web`：用 `embed` 打包 `dist`，提供 SPA 回退与 `<base href>` 注入；`go build` 后单二进制能返回 index.html
- [x] 1.5 添加 `.github/workflows/ci.yml`：`go vet`、`golangci-lint`、`go test`、前端 build 与体积检查、Conventional Commits 标题校验

## 2. 配置与启动（bootstrap-config）

- [x] 2.1 实现 `internal/config`：默认值、YAML 配置文件、`EW_*` 环境变量、`--*` flag 的四层合并，记录每项来源
- [x] 2.2 实现配置文件查找：`--config`/`EW_CONFIG` 显式路径优先，否则依次查找 `./config.yaml`、`<data-dir>/config.yaml`，都没有则跳过；显式路径不存在或 YAML 非法（含未知字段）时拒绝启动
- [x] 2.3 实现配置校验（监听地址、路径、TLS 文件存在性、`EW_TRUSTED_PROXIES` CIDR）与 `--print-config`（密钥脱敏）
- [x] 2.4 实现数据目录与存储目录自动创建，不可写时以清晰错误退出
- [x] 2.5 实现 `--version` 输出（ldflags 注入版本、commit、构建时间）
- [x] 2.6 单元测试：优先级覆盖、查找顺序、非法值报错、print-config 脱敏

## 3. 存储层（SQLite）

- [x] 3.1 引入 `modernc.org/sqlite`，实现 `internal/store` 打开数据库、WAL 模式、内嵌 SQL 迁移按版本执行
- [x] 3.2 定义并迁移表：`users`（id、username、password_hash、role、root_dir、permission、quota、disabled、created_at）、`sessions`、`usage`（root_dir、bytes、updated_at）、`settings`（key、value）
- [x] 3.3 实现 users/sessions/usage/settings 的仓库方法
- [x] 3.4 单元测试：迁移幂等、用户名唯一约束、会话过期查询

## 4. 认证与会话（admin-auth）

- [x] 4.1 实现 argon2id 密码哈希与校验，密码最短 8 位策略
- [x] 4.2 实现会话创建、校验、7 天空闲过期、退出、按用户批量吊销、同一用户多会话；Cookie 属性按 spec（HttpOnly、SameSite=Lax、HTTPS 时 Secure）
- [x] 4.3 实现请求上下文中间件：真实 IP（仅信任 `EW_TRUSTED_PROXIES` 内来源的 `X-Forwarded-For`）、有效协议（TLS 或受信任的 `X-Forwarded-Proto`）、是否回环来源
- [x] 4.4 实现按 IP 的登录失败限速（10 次 / 5 分钟，`429` + `Retry-After`）
- [x] 4.5 实现 CSRF 防护中间件（非幂等请求要求自定义头或双提交 token）
- [x] 4.6 实现统一错误响应格式 `{code, message, details}` 与错误码常量表
- [x] 4.7 实现 API：`POST /api/v1/auth/login`、`POST /api/v1/auth/logout`、`GET /api/v1/auth/me`（含 `insecure` 标志）、`POST /api/v1/auth/password`
- [x] 4.8 实现角色中间件：admin-only 路由返回 `403`
- [x] 4.9 实现首次运行引导：`GET /api/v1/setup/status`（含 `insecure` 标志）、`POST /api/v1/setup`（创建 admin 并建根目录 `<username>`），存在管理员后锁定；支持 `EW_ADMIN_USER/PASSWORD` 启动时创建
- [x] 4.10 单元与 handler 测试：登录成功/失败/禁用、限速、退出后 401、改密吊销会话、setup 锁定、CSRF 拒绝、伪造代理头被忽略、insecure 标志三种场景

## 5. 用户管理与运行时设置（user-management）

- [x] 5.1 实现用户名校验、根目录路径规范化与逃逸检测（拒绝 `..`、绝对路径、编码绕过、空字符串、`.`）
- [x] 5.2 实现 API：`GET/POST /api/v1/users`、`GET/PUT/DELETE /api/v1/users/{id}`、`POST /api/v1/users/{id}/password`、启用/禁用
- [x] 5.3 创建用户时自动创建根目录并从运行时设置取默认配额与权限；删除用户时保留文件；禁止删除或禁用最后一个管理员
- [x] 5.4 修改密码/禁用/删除时吊销该用户会话并清空 Basic 认证缓存
- [x] 5.5 实现运行时设置：`GET /api/v1/settings`（所有已登录用户）、`PUT /api/v1/settings`（admin），进程内缓存即时生效，三项：`default_quota`、`default_permission`、`site_name`
- [x] 5.6 测试：重名 `409`、路径穿越与空根目录 `400`、最后管理员 `409`、权限变更即时生效、默认设置应用到新用户、非 admin 改设置 `403`

## 6. WebDAV 服务（webdav-server）

- [x] 6.1 复制 `golang.org/x/net/webdav` 源码到 `internal/webdav/xnet/`，保留 LICENSE 与 PATENTS，写 `UPSTREAM.md` 记录 x/net 版本；替换 import 后 `go build` 与上游自带测试通过
- [x] 6.2 补丁一：MOVE/COPY 缺 `Overwrite` 头时按 RFC 4918 默认 `T`；补丁二：PROPFIND 中集合 href 追加尾斜杠；补丁三：对集合的 PROPPATCH 返回 `207` 且不支持属性回 `403` 状态；每个补丁独立提交并在 `UPSTREAM.md` 登记上游 issue 编号
- [x] 6.3 实现 Basic 认证中间件：解析凭据、查用户、校验哈希、禁用用户拒绝、`401` + `WWW-Authenticate`，接入登录限速
- [x] 6.4 实现 Basic 认证结果 60 秒内存缓存（key 为哈希），并提供按用户清空
- [x] 6.5 实现 `restrictedFS`：包装 `webdav.Dir`，路径规范化与根目录限制；符号链接逐段 `Lstat` 并 `EvalSymlinks`，真实路径在根内则跟随，否则返回不存在，链式链接完整展开；`read` 用户写方法返回权限错误；打开后二次 `Stat` 比对防 TOCTOU
- [x] 6.6 实现 LockSystem 包装：全局 `MemLS`，锁路径前置用户根目录绝对路径
- [x] 6.7 组装每请求 `webdav.Handler`，挂载到 `/dav/`；OPTIONS 补 `MS-Author-Via: DAV`；`Depth: infinity` PROPFIND 返回 `403`
- [x] 6.8 校验 COPY/MOVE 的 Destination 头在根目录内，否则 `403`
- [x] 6.9 确认 GET 支持 Range/ETag/条件请求（`http.ServeContent` 路径），补充缺失部分
- [x] 6.10 集成测试：用 Go 客户端覆盖 OPTIONS、PROPFIND（含尾斜杠）、PUT/GET 往返、read 用户 `403`、LOCK 冲突 `423`、Range `206`、越界路径 `404`、根内符号链接可访问、根外符号链接 `404`、链式链接、缺 Overwrite 头的 MOVE 成功、目录 PROPPATCH `207`
- [ ] 6.11 CI 中加入 `litmus` 与 `rclone` 兼容性测试（Docker 仅在 CI）；发布前手动验证 Windows 资源管理器、macOS 访达、davfs2、一款移动端 App，并把结果记入 README 兼容性表

## 7. 配额与用量（quota-usage）

- [x] 7.1 实现 `internal/quota`：按根目录的用量缓存读写、原子增减，表观大小计量
- [x] 7.2 实现计数 writer 与 WebDAV PUT 的临时文件写入流程（临时文件 → rename），超限中止并清理
- [x] 7.3 在 WebDAV PUT/COPY/MOVE 与面板上传接入预检与计量：WebDAV 返回 `507`，API 返回 `413` 附剩余空间；覆盖时按差值计算
- [x] 7.4 实现启动时与每 24 小时的全量重算与过期上传临时目录清理，以及 `POST /api/v1/usage/recalculate`
- [x] 7.5 实现 `GET /api/v1/usage`（管理员全局概览含磁盘剩余）与 `GET /api/v1/usage/me`
- [x] 7.6 用户 API 支持设置 `quota` 字段（字节），0 表示不限
- [x] 7.7 测试：超限 `507` 且无残留文件、覆盖差值、chunked 传输中途超限、共享根目录共用用量、重算纠偏、临时数据不计入用量

## 8. 网页文件浏览 API（web-file-browser 后端）

- [x] 8.1 实现 `resolveTarget(session, user_id)`：缺省为自身根目录与权限；提供 `user_id` 时要求 admin 否则 `403`，基准切为目标用户根目录且权限为 `readwrite`；所有文件 handler 只依赖其结果
- [x] 8.2 实现 `GET /api/v1/files?path=`：一次返回目录全部条目（含隐藏文件），路径限制在目标根目录
- [x] 8.3 实现 `internal/upload` 分块上传三段接口：`POST /api/v1/uploads`（路径、总大小、冲突策略 overwrite/skip/rename，做配额预检）、`PUT /api/v1/uploads/{id}/chunks/{n}`（顺序写入临时文件并计量）、`POST /api/v1/uploads/{id}/complete`（校验大小后 rename）；临时目录位于存储根目录下的隐藏目录，超过 24 小时未完成的由 7.4 清理
- [x] 8.4 实现下载 `GET /api/v1/files/download?path=`：单文件带 `Content-Disposition: attachment` 与 `nosniff`；多路径或目录走流式 ZIP
- [x] 8.5 实现预览 `GET /api/v1/files/raw?path=`：仅白名单 MIME 允许 inline，HTML 强制 `text/plain`，文本类超过 2 MB 返回 `413`
- [x] 8.6 实现 mkdir、rename/move、copy、delete 接口，`read` 用户返回 `403`
- [x] 8.7 测试：路径穿越、HTML 不 inline、大文本 `413`、ZIP 目录结构完整、read 用户 `403`、冲突策略三种分支、分块乱序/缺块/重复块、`user_id` 非 admin `403`、admin 在 read 用户目录写入成功且配额记在目标用户

## 9. 前端面板

- [x] 9.1 搭建路由与布局：登录页、首次引导页、文件浏览器（`/files/<path>` 与 `/files/u/<user_id>/<path>`）、用户管理（admin）、概览（admin）、系统设置（admin）、账户设置；未登录跳转登录并回跳
- [x] 9.2 接入 vue-i18n：`zh-CN` 与 `en` 语言包、按浏览器语言自动选择、回退英文、手动切换存 localStorage；错误码到文案的映射表
- [x] 9.3 接入 Naive UI 主题：跟随 `prefers-color-scheme`，手动切换存 localStorage
- [x] 9.4 实现首次引导页与登录页，按 `insecure` 标志显示不安全连接警告，显示站点名称
- [x] 9.5 实现文件浏览器：列表虚拟滚动、前端排序、面包屑、URL 即路径、空状态、隐藏文件开关（存 localStorage）
- [x] 9.6 实现分块上传：按钮与拖拽、多文件、文件夹结构、块级并发 3、块失败重试 3 次、进度条、冲突对话框、配额不足提示
- [x] 9.7 实现下载（单文件与多选 ZIP）、新建文件夹、重命名、移动、复制、带确认的永久删除；`read` 用户隐藏写操作
- [x] 9.8 实现预览覆盖层：图片（同目录左右切换）、文本/代码高亮（2 MB 内）、PDF、音视频、不支持类型与过大文本的信息卡片
- [x] 9.9 实现用户管理页：列表（含配额与占用百分比）、创建/编辑表单（默认值来自运行时设置）、重设密码、启用/禁用/删除、最后管理员操作禁用、"浏览文件"入口
- [x] 9.10 实现管理员跨用户浏览：进入 `/files/u/<user_id>/` 后顶部持续显示"正在查看 <username> 的文件"提示条与退出按钮，操作按 `readwrite` 展示
- [x] 9.11 实现概览页（全局指标、占用 Top 用户、重算按钮）与用户侧的用量进度条
- [x] 9.12 实现系统设置页（默认配额、默认权限、站点名称）与账户设置页（修改自己的密码）
- [x] 9.13 响应式适配：375px 宽度下核心操作可用，移动端操作菜单
- [x] 9.14 Vitest 单测覆盖 store、上传状态机、错误码映射；前端体积检查通过（gzip < 500 KB）并集成到 CI

## 10. 服务器组装与部署（deployment）

- [x] 10.1 实现 `internal/logging`：slog 初始化（`EW_LOG_FORMAT` text/json、`EW_LOG_LEVEL`），访问日志中间件（默认只记 4xx/5xx 与写操作，`EW_ACCESS_LOG=all|off`），不记录 Authorization 与 Cookie
- [x] 10.2 实现 `internal/server`：路由、中间件链、`EW_BASE_URL` 子路径前缀
- [x] 10.3 实现 `/healthz`（检查数据库 ping 与存储目录可写）
- [x] 10.4 实现可选 TLS（`EW_TLS_CERT/KEY`）与 graceful shutdown（30 秒上限，清理临时文件）
- [x] 10.5 编写 `deploy/Dockerfile`（多阶段构建，distroless/scratch，非 root，`/data`，HEALTHCHECK）与 PUID/PGID 支持（实现方式按 design Open Questions 推荐）
- [x] 10.6 编写 `deploy/docker-compose.yml`、`deploy/easy-webdav.service`（systemd，专用用户，`/var/lib/easy-webdav`）与 `deploy/nginx.example.conf`（子路径反代示例）
- [x] 10.7 配置 GoReleaser：六平台二进制、checksums、多架构镜像推 `ghcr.io/lecritus/easy-webdav`、按 Conventional Commits 生成 release notes；`.github/workflows/release.yml` 在 tag 时触发
- [ ] 10.8 端到端验证（CI 或有 Docker 的机器）：干净机器运行二进制、`docker run` 一行启动、`docker compose up -d`、systemd 单元启动、nginx 子路径下面板与 WebDAV 可用、镜像 < 30 MB、日志三种模式输出正确

## 11. 文档与发布

- [x] 11.1 编写 README（中英双语）：一句话介绍、三种启动方式（二进制/docker run/compose/systemd）、配置项表（含日志与代理项）、配置文件示例、客户端挂载指南与兼容性表、安全建议（TLS 与不安全警告说明）
- [x] 11.2 编写 CONTRIBUTING.md（Conventional Commits、本地开发无需 Docker、fork 补丁流程）与 issue/PR 模板
- [ ] 11.3 打 `v0.1.0` tag 并确认 release 产物完整
