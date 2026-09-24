## Context

仓库为空，本设计从零定义项目骨架。动机见 proposal.md 的 Why，行为契约见 `specs/` 下七个能力的规范。

约束来自用户的两个硬性要求："开箱即用"意味着无外部依赖、无必填配置；"部署方便"意味着单文件与单容器两种交付形态都必须一等支持。技术栈已由用户确定为 Go 单二进制 + 内嵌前端。目标用户为 1 到 50 人的个人与小团队，单实例即可。

工具链约束：`go.mod` 声明 Go 1.25，前端要求 Node 22 LTS 以上；开发机上没有 Docker，因此所有依赖 Docker 的测试与构建只在 CI 执行，本地开发流程不得依赖 Docker。

WebDAV 协议本身的难点集中在 LOCK 语义与各系统客户端的怪癖。调研确认：`golang.org/x/net/webdav` 仍随 x/net 发布但功能上多年未动，包文档明确写明"未针对恶意客户端加固"，且有多个长期未修的兼容 issue（访达 MOVE 缺 `Overwrite` 头返回 412、集合 href 缺尾斜杠、Windows Mini-Redirector 对目录 PROPPATCH 返回 500）。`emersion/go-webdav` 只有 Class 1，访达会以只读挂载。SFTPGo 用的 `drakkan/webdav` 修了这些问题但维护者改动为 AGPL，与 MIT 项目不兼容。

## Goals / Non-Goals

**Goals:**
- 协议层不重写：以 `golang.org/x/net/webdav` 源码为基础打补丁，只修已知兼容问题，其余不动；认证、路径隔离、权限、配额四个关注点作为外层中间件。
- 一个进程、一个数据目录、一个端口，覆盖面板、API 与 WebDAV。
- 数据模型简单到能用 SQLite 单文件承载，且可用纯 Go 驱动避免 CGO，使交叉编译零成本。
- 前端构建产物通过 `embed` 进二进制，前端与后端在同一仓库、同一版本号发布。
- 所有安全边界（路径、权限、配额）在后端统一入口强制执行，前端只做展示与交互。

**Non-Goals:**
- 不做插件系统或多存储后端抽象。存储层直接使用本地文件系统，但通过一个窄接口隔离，为后续 S3 留口。
- 不做集群或多实例。SQLite 与本地锁表意味着单实例。
- 不做前端 SSR，不追求 SEO。
- 不做 WebDAV 锁的持久化。进程重启即清空锁，与 Apache mod_dav 默认行为一致。
- 不做回收站、缩略图、Prometheus 指标、API Token、LDAP、跨刷新的断点续传。

## Decisions

### D1. 协议库：fork `x/net/webdav` 进仓库并打补丁

- 做法：将 `golang.org/x/net/webdav` 源码（约 3000 行，BSD-3）复制到 `internal/webdav/xnet/`，保留上游 LICENSE 与 PATENTS。直接在源码上修复三个已知问题：（1）MOVE/COPY 缺 `Overwrite` 头时按 RFC 4918 默认视为 `T`；（2）PROPFIND 响应中集合的 href 追加尾斜杠；（3）对集合的 PROPPATCH 返回 `207` 并对不支持的属性回 `403` 状态而非整体 `500`。同时在 OPTIONS 响应补 `MS-Author-Via: DAV`，并对 `Depth: infinity` 的 PROPFIND 返回 `403`。
- 备选：A 中间件外挂修补，放弃原因是尾斜杠与 PROPPATCH 的问题在 Handler 内部生成响应体时发生，外层难以干净地改写 XML；C 用 `drakkan/webdav`，放弃原因是 AGPL 传染；D 自研，放弃原因是收益低风险高；Rust 或 Node 方案在 proposal 阶段已排除。
- 已知代价：失去上游自动更新，需要手动跟进安全修复；上游变动极少，成本可接受。补丁思路可以参考 `drakkan/webdav`，但代码必须自行实现以避开 AGPL。

### D2. 用户隔离：每请求构造带根目录的 `webdav.FileSystem`

- 做法：认证中间件解析用户后，将 `webdav.Dir(<storageRoot>/<userRoot>)` 包装进一个 `restrictedFS`，该 wrapper 负责：（a）拒绝解析后逃逸根目录的路径；（b）对每个路径段做 `Lstat`，遇到符号链接时 `EvalSymlinks` 解析到真实路径，真实路径不在用户根目录内则返回不存在，链式链接全部展开后再判断；（c）对 `read` 用户的所有写方法直接返回权限错误。`webdav.Handler` 每次请求新建，成本可忽略。
- 符号链接策略选择"允许跟随但限制在根内"而非全部拒绝，是为了保留 NAS 用户用链接把外部目录挂进用户空间的常见用法，同时阻止逃逸。WebDAV 协议本身不能创建符号链接，所以风险面只在拥有文件系统访问权的管理员。
- 备选：单一全局 FileSystem 靠 URL 前缀 `/dav/<user>/` 区分。放弃原因：暴露用户名、Destination 头跨用户复制更难拦截、客户端挂载路径更长。
- 锁系统：全局共享一个 `webdav.NewMemLS()`，但锁 token 的资源路径前置用户根目录绝对路径，避免不同用户同名文件互相锁定；共享根目录的用户自然共享锁，符合预期。
- 根目录约束：用户根目录必须是存储根目录的非空真子路径。这既避免任何用户看到其他用户文件夹，也保证存储根目录下的系统内部目录（见 D4 上传临时目录）对所有用户不可见。

### D3. 存储：SQLite（`modernc.org/sqlite`，纯 Go）

- 存放：用户表、会话表、用量表、系统设置表。单文件 `data/easy-webdav.db`，WAL 模式。
- 选择理由：零外部依赖；纯 Go 驱动避免 CGO，使 `CGO_ENABLED=0` 交叉编译到六个平台无需工具链。`modernc.org/sqlite` 生态验证充分，主流迁移与 ORM 工具默认支持。
- 备选：`ncruces/go-sqlite3` 依赖更少、二进制略小，但生态相对新；bbolt 缺少 SQL 与迁移便利；`mattn/go-sqlite3` 需要 CGO。
- 迁移：内嵌 SQL 迁移文件，启动时按版本号顺序执行。

### D4. 配额：写入前预检 + 写入中计量 + 后台全量校正

- 用量缓存在 `usage` 表中按根目录路径（而非用户）记账，天然处理共享根目录，也天然处理管理员在他人目录中的写入（D12）。用量按文件表观大小累计，目录与临时数据计 0。
- WebDAV PUT 流程：（1）若 `Content-Length` 已知，先判断 `used + delta > quota` 则直接 `507`；（2）写入临时文件，写入流用计数 writer 包装，超限即中止并删除临时文件；（3）成功后 rename 到目标路径，再原子更新用量。临时文件方式同时满足"不留半成品"与 graceful shutdown 的要求。
- 面板分块上传流程（见 D7）：每个上传会话在临时目录下有独立子目录，分块顺序追加到单个临时文件；预检在会话创建时按声明总大小做一次，计量在每块写入时做；最后一块到达后 rename 到目标路径。
- 临时目录位置：必须与存储目录同文件系统，否则最终 rename 会跨盘失败。候选位置为存储根目录下的隐藏目录（如 `<storage-dir>/.ew-tmp/`），依赖 D2 的"用户根目录必须是非空真子路径"约束保证其对用户不可见。具体目录名与分块大小见 Open Questions，实现时按推荐值执行。
- COPY/MOVE 入根目录同样走预检；MOVE 在同一根目录内净增量为 0。
- 后台任务：启动时与每 24 小时 `filepath.WalkDir` 重算全部根目录，更新缓存，同时清理超过 24 小时的上传临时目录；面板提供手动触发。
- 备选：每次写入都 Walk 计算。放弃原因：大目录下 O(n) 代价不可接受。

### D5. 认证分离：面板用会话 Cookie，WebDAV 用 Basic

- WebDAV 客户端只支持 Basic/Digest，且 Digest 与现代密码哈希不兼容（需要明文或 HA1 存储），因此 WebDAV 用 Basic。明文 HTTP 下允许使用，面板通过 D6 的协议检测显示警告，不阻断，因为多数自用场景是内网或反代后（此时到服务本身就是 HTTP）。
- 面板用 Cookie 会话，会话 ID 存 SQLite，`HttpOnly + SameSite=Lax`，HTTPS 时加 `Secure`。同一用户允许多个并发会话。所有非 GET 的 API 请求要求 `X-Requested-With` 或双提交 CSRF token 头，Cookie 的 SameSite=Lax 作为第二道防线。
- 密码哈希：argon2id，参数取 OWASP 推荐值。WebDAV 每请求都要验密，argon2id 每次几十毫秒会成为瓶颈，因此对 Basic 认证结果做一个短期（60 秒）内存缓存，key 为 `sha256(username + ":" + password)`，密码修改或用户禁用时主动清空该用户条目。
- 登录限速：内存中按 IP 的滑动窗口计数器，面板登录与 WebDAV 认证共用。真实 IP 只从 `EW_TRUSTED_PROXIES` 内来源的 `X-Forwarded-For` 取，默认不信任任何代理。
- `/api/` 不接受 Basic 认证，脚本化需求由 CLI 子命令承担（见 Open Questions）。

### D6. HTTP 路由布局与请求上下文

```
/                 → 内嵌前端 SPA（index.html，未匹配路径回退到 index.html）
/api/v1/...       → JSON API，Cookie 会话
/dav/             → WebDAV，Basic 认证
/healthz          → 健康检查
/assets/...       → 前端静态资源，带长缓存与内容哈希文件名
```

- 所有路由挂在 `EW_BASE_URL` 前缀之下；前端构建时 `base` 设为相对路径 `./`，运行时通过注入到 `index.html` 的 `<base href>` 获得前缀，避免为不同子路径重新构建。
- 请求上下文中间件统一计算：真实客户端 IP、有效协议（TLS 直连或受信任代理的 `X-Forwarded-Proto`）、是否回环来源。`GET /api/v1/auth/me` 与 setup status 接口返回 `insecure: true/false`，前端据此显示不安全连接警告。
- API 错误统一格式 `{"code": "USER_EXISTS", "message": "...", "details": {}}`；`code` 稳定，前端按 code 查 i18n 文案，`message` 为英文回退。

### D7. 前端：Vue 3 + TypeScript + Vite + Naive UI

- UI 库选 Naive UI：Vue 3 原生、TypeScript 类型完整、tree-shaking 良好、内置明暗主题切换。备选 Element Plus 体积更大，Tailwind 自写组件在表格、对话框、表单校验上耗时过多。
- i18n：vue-i18n，内置 `zh-CN` 与 `en`，首次按 `navigator.languages` 匹配，无匹配回退 `en`；手动选择存 localStorage。后端不做 i18n。
- 主题：Naive UI 的 `darkTheme` 配合 `prefers-color-scheme` 监听，手动覆盖存 localStorage。
- 状态：Pinia 管理当前用户与会话；文件浏览器状态以 URL 为准（路径即状态），保证刷新与分享路径可用。管理员跨用户浏览时 URL 携带目标用户标识。
- 大目录：API 一次返回全部条目，前端用虚拟滚动渲染，排序在前端完成。
- 上传：固定大小分块，前端维护上传会话，块级并发上限 3，块失败自动重试 3 次后标记该文件失败；页面刷新即放弃，不做跨刷新续传。文件夹上传通过 `webkitdirectory` 与拖拽 `DataTransferItem.webkitGetAsEntry()` 保留结构。API 形态为三段：创建会话（声明路径、总大小、冲突策略）、上传块（会话 id + 块序号）、完成（服务端校验大小后 rename）。
- 隐藏文件：默认过滤 `.` 开头条目，开关存 localStorage；过滤在前端做，API 始终返回全部条目。
- 预览：图片、音视频用原生标签；PDF 用浏览器内置查看器于 iframe，`Content-Disposition: inline` 仅对这几类白名单 MIME 允许；文本与代码上限 2 MB，用轻量高亮库按语言按需加载。HTML 一律当作文本。不做缩略图。

### D8. ZIP 下载流式实现

使用标准库 `archive/zip` 直接写入 `http.ResponseWriter`，配合 `Flush`；不设置 `Content-Length`，使用 chunked 传输。遍历时对每个文件先做配额无关的路径校验。

### D9. 项目目录结构

```
cmd/easy-webdav/         main.go：命令入口、组装、启动
internal/config/         默认值、YAML 文件查找与解析、env、flag 合并与 --print-config
internal/store/          SQLite 打开、迁移、各表的仓库方法
internal/auth/           密码哈希、会话、Basic 缓存、限速
internal/webdav/xnet/    x/net/webdav 源码副本与补丁，含 UPSTREAM.md
internal/webdav/         restrictedFS、LockSystem 包装、Handler 组装
internal/quota/          用量记账、计数 writer、后台重算、临时目录清理
internal/upload/         分块上传会话管理
internal/api/            JSON API handlers（users、files、usage、setup、auth、settings）
internal/web/            embed 前端产物、SPA 回退、base href 注入
internal/server/         路由、中间件链、请求上下文、graceful shutdown、healthz
internal/logging/        slog 初始化、访问日志中间件
web/                     Vite 前端工程
deploy/                  Dockerfile、docker-compose.yml、easy-webdav.service、示例 nginx 配置
.github/workflows/       ci.yml（lint、test、build、commit 规范）、release.yml（goreleaser）
```

### D10. 发布：GoReleaser + GitHub Actions + ghcr.io

- 仓库 `github.com/122440367/easy-webdav`，默认分支 `main`，MIT 许可。
- `goreleaser` 一次产出六平台二进制、checksums、多架构镜像推 `ghcr.io/122440367/easy-webdav`；release notes 按 Conventional Commits 类型自动分组，不手写 CHANGELOG。CI 校验 PR 标题与提交信息符合规范。
- Dockerfile 基于 `gcr.io/distroless/static` 或 `scratch`，包含 CA 证书与时区数据。PUID/PGID 实现方式见 Open Questions。
- 本地 Makefile 中 Docker 相关目标（镜像构建、litmus、rclone 测试）检测到无 Docker 时跳过并提示，不阻塞其他目标。

### D11. 日志

- 标准库 `slog`。启动时按 `EW_LOG_FORMAT` 选择 `TextHandler` 或 `JSONHandler`，按 `EW_LOG_LEVEL` 设级别，只写 stdout/stderr，轮转交给 systemd/Docker。
- 访问日志为一个中间件，在响应完成后按 `EW_ACCESS_LOG` 策略决定是否记录：`errors-and-writes`（默认）、`all`、`off`。记录字段：方法、路径、状态、耗时、字节数、用户名、来源 IP。Authorization 头与 Cookie 一律不记录。

### D12. 管理员跨用户浏览

- 文件 API 全部接受可选的 `user_id` 查询参数。缺省时以当前会话用户的根目录为基准；提供时要求当前用户为 admin，否则 `403`，基准切换为目标用户的根目录且权限强制为 `readwrite`。
- 权限与配额解析在同一处完成：`resolveTarget(session, user_id) → (rootDir, permission, quota)`，所有文件 handler 只依赖这个结果，不再关心是谁在操作。用量按根目录记账（D4），因此不需要额外逻辑。
- 前端在用户管理页提供入口，进入后路由变为 `/files/u/<user_id>/<path>`，顶栏渲染提示条与退出按钮。

### D13. 运行时设置

- `settings` 表键值存储三项：`default_quota`、`default_permission`、`site_name`。`GET /api/v1/settings` 对所有已登录用户开放（前端需要站点名称），`PUT /api/v1/settings` 仅 admin。
- 进程内缓存一份，写入时同步更新，无需重启。创建用户时未提供的字段从这里取默认值。

## Risks / Trade-offs

- [fork `x/net/webdav` 后失去上游自动更新] → `UPSTREAM.md` 记录复制时的 x/net 版本号与每个补丁的说明和上游 issue 编号；补丁各自独立提交，便于重放到新版本；集成测试覆盖 Windows/macOS/davfs2/rclone 的真实交互，CI 中用 `litmus` 与 `rclone` 自动化，桌面客户端在发布前手动验证。
- [Basic 认证在无 TLS 时明文传输密码] → 面板在非回环 HTTP 访问时显示醒目警告；文档提供内置 TLS 与反代示例。
- [Basic 认证缓存被撞库] → 缓存 key 为哈希，且缓存命中不绕过限速统计；60 秒 TTL 限制窗口。
- [SQLite 单实例限制横向扩展] → 明确定位为个人与小团队工具；接口层留出 `store` 抽象，后续可换 Postgres。
- [Windows 资源管理器对大文件与大目录响应慢] → 这是 MiniRedir 的固有限制，面板连接引导与文档中推荐 rclone 或 RaiDrive 作为替代客户端。
- [用量缓存与磁盘真实状态漂移（用户绕过服务直接改文件）] → 每日全量重算 + 手动重算；配额判断允许短暂误差，不追求强一致。
- [临时文件写入后 rename 在 Windows 上遇到目标被占用会失败] → 重试数次后返回 `423` 或 `409`，并在响应中说明。
- [上传临时目录位于存储根目录下，管理员误配用户根目录为存储根会暴露它] → D2 在用户管理 API 中硬性拒绝空或 `.` 的根目录。
- [符号链接跟随带来 TOCTOU 竞争] → 解析后使用真实路径打开文件，并在打开后再次 `Stat` 比对；风险面仅限拥有文件系统访问权的管理员。
- [ZIP 流式下载中途出错无法回传状态码] → 已开始传输后只能断开连接；在遍历前做一次快速的权限与存在性校验，把大部分失败前置。
- [前端 UI 库与 i18n 体积影响二进制大小] → 按需引入，语言包按需加载，目标前端产物 gzip 后小于 500 KB；CI 中加体积检查。
- [本地无 Docker 导致兼容性问题只能在 CI 发现] → CI 在每个 PR 上跑 litmus 与 rclone；Go 集成测试用内置客户端覆盖已知客户端行为（缺 Overwrite 头的 MOVE、目录 PROPPATCH 等），本地即可回归。

## Migration Plan

全新项目，无迁移。发布节奏：`v0.1.0` 完成本变更所有任务后打 tag，触发首个 release。数据库迁移机制从第一版就启用，以便后续版本平滑升级。回滚策略：用户回退到旧二进制即可，SQLite 迁移只做向前兼容的添加列，不做破坏性变更，直到 1.0。

## Open Questions

以下问题在访谈中未拍板，不改变规范与任务结构，实现时按括号内推荐值执行并在代码注释或文档中标注：

- fork 的维护方式（推荐：`UPSTREAM.md` 记录版本与补丁清单，补丁独立提交，不写同步脚本）。
- 分块上传的块大小与临时目录名（推荐：8 MB，`<storage-dir>/.ew-tmp/<upload-id>/`）。
- CLI 形态与管理员忘记密码的恢复手段（推荐：子命令 `serve`、`admin reset-password`、`admin create`、`version`、`config print`，`--version` 与 `--print-config` 保留为别名）。
- 脚本化管理的认证方式（推荐：MVP 不支持，由 CLI `admin` 子命令覆盖；API Token 留后续）。
- 面板"连接"引导弹窗的内容深度（推荐：显示 WebDAV URL、复制按钮、按操作系统分标签的挂载步骤、rclone 与 `net use` 片段）。
- Docker 基础镜像与 PUID/PGID 实现（推荐：`distroless/static`，二进制检测到以 root 运行且设置了 PUID/PGID 时 chown `/data` 后 setuid/setgid 并 exec 自身；未设置时以 uid 1000 运行）。
- 测试策略细节（推荐：Go 表格测试 + httptest，前端 Vitest 单测，不做 Playwright）。
- 镜像标签策略（推荐：tag 时 `vX.Y.Z`、`vX.Y`、`latest`；main 推送时 `edge`）。
- 是否为 WebDAV 增加 Digest 认证作为可选项。需要以可逆方式存储 HA1，安全性折衷明显，默认不做，留待用户反馈。
