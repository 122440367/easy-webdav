## Purpose

定义管理员如何在面板中管理用户账号、每个用户的根目录隔离与读写权限如何被声明和强制执行，以及管理员可在面板修改的运行时默认设置。

## ADDED Requirements

### Requirement: User CRUD
管理员 SHALL 能创建、查看、编辑、禁用、启用与删除用户。用户名 MUST 全局唯一，仅允许字母、数字、下划线、连字符和点，长度 1 到 64。系统 MUST NOT 提供用户自助注册，所有账号由管理员创建。

#### Scenario: Create user
- **WHEN** 管理员提交合法的用户名、密码、根目录与权限
- **THEN** 系统创建用户并返回其信息（不含密码），面板用户列表立即显示该用户

#### Scenario: Duplicate username
- **WHEN** 管理员使用已存在的用户名创建用户
- **THEN** 系统返回 `409` 并提示用户名已存在

#### Scenario: Delete user
- **WHEN** 管理员删除一个用户
- **THEN** 该用户的所有会话失效，WebDAV 认证立即失败，其根目录下的文件 MUST 保留不被删除

#### Scenario: Last admin cannot be removed
- **WHEN** 管理员尝试删除或禁用系统中唯一的管理员账号
- **THEN** 系统返回 `409` 并拒绝操作

### Requirement: Per-user root directory
每个用户 SHALL 有一个根目录，管理员角色也不例外。根目录以相对于存储根目录的路径表示，未指定时默认为 `<username>`。系统 MUST 在创建用户时自动创建该目录。根目录路径 MUST 经过规范化，MUST NOT 为空或 `.`（即不得以存储根目录本身为根），且 MUST NOT 逃逸出存储根目录。

#### Scenario: Default root directory
- **WHEN** 管理员创建用户 `alice` 且未填写根目录
- **THEN** 系统将根目录设为 `alice`，并在存储根目录下创建该文件夹

#### Scenario: Admin has own root directory
- **WHEN** 管理员账号通过 WebDAV 挂载或在面板打开"我的文件"
- **THEN** 看到的是其自身根目录内容，而非存储根目录下的所有用户文件夹

#### Scenario: Path traversal rejected
- **WHEN** 管理员将根目录填写为 `../outside` 或包含 `..` 段的任何路径
- **THEN** 系统返回 `400` 并拒绝保存

#### Scenario: Storage root as user root rejected
- **WHEN** 管理员将根目录填写为空字符串、`.` 或 `/`
- **THEN** 系统返回 `400` 并拒绝保存

#### Scenario: Shared root directory
- **WHEN** 两个用户被指定相同的根目录
- **THEN** 两者都能访问同一目录内容，各自的权限独立生效

### Requirement: Access permission
每个用户 SHALL 有权限级别 `read` 或 `readwrite`。`read` 用户对其根目录内任何写入类操作（上传、新建、重命名、移动、复制、删除、加锁）MUST 被拒绝。

#### Scenario: Read-only user attempts upload
- **WHEN** 权限为 `read` 的用户通过 WebDAV 或网页尝试上传文件
- **THEN** 系统返回 `403`，文件系统无任何变化

#### Scenario: Permission change takes effect immediately
- **WHEN** 管理员将某用户从 `readwrite` 改为 `read`
- **THEN** 该用户随后的写入请求立即被拒绝，无需重新登录

### Requirement: Directory isolation
用户 SHALL 只能看到并操作自己根目录内的内容。任何试图访问根目录之外路径的请求 MUST 返回 `404` 或 `403`，且 MUST NOT 泄露目标是否存在。

#### Scenario: Access outside root
- **WHEN** 用户请求形如 `/dav/../other-user/file` 或经编码绕过的路径
- **THEN** 系统返回 `404`，不读取也不列出目标

### Requirement: Admin password reset
管理员 SHALL 能为任意用户重设密码，且不需要提供该用户的旧密码。

#### Scenario: Admin resets user password
- **WHEN** 管理员为用户提交新密码
- **THEN** 系统更新密码并使该用户所有会话失效

### Requirement: Runtime settings
管理员 SHALL 能在面板中修改以下运行时设置，修改 MUST 立即生效且不需要重启：新建用户的默认配额（默认 0，即不限）、新建用户的默认权限（默认 `readwrite`）、站点名称（默认 `easy-webdav`，显示在页面标题与登录页）。影响进程生命周期的项（监听地址、路径、TLS、代理、日志）MUST 只能通过配置修改。

#### Scenario: Default quota applied to new user
- **WHEN** 管理员将默认配额设为 5 GB，随后创建用户且未填写配额
- **THEN** 新用户的配额为 5 GB

#### Scenario: Site name shown
- **WHEN** 管理员将站点名称改为"家庭网盘"
- **THEN** 登录页与浏览器标签标题立即显示"家庭网盘"

#### Scenario: Non-admin cannot change settings
- **WHEN** 普通用户请求修改运行时设置
- **THEN** 系统返回 `403`
