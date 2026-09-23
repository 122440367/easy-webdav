## Purpose

定义管理面板与网页文件浏览器的登录、会话、退出行为，管理员与普通用户两种角色的访问边界，以及通过明文 HTTP 访问时的安全警告。

## ADDED Requirements

### Requirement: Session login
系统 SHALL 提供基于用户名与密码的登录接口，登录成功后 MUST 通过 HttpOnly、SameSite=Lax 的 Cookie 建立会话；在通过 HTTPS 访问时 Cookie MUST 带 Secure 标记。

#### Scenario: Successful login
- **WHEN** 用户提交正确的用户名与密码
- **THEN** 系统返回 `200`，设置会话 Cookie，并返回当前用户的基本信息与角色

#### Scenario: Wrong credentials
- **WHEN** 用户提交错误的用户名或密码
- **THEN** 系统返回 `401`，错误信息 MUST NOT 区分"用户不存在"与"密码错误"

#### Scenario: Disabled user login
- **WHEN** 已被禁用的用户提交正确的用户名与密码
- **THEN** 系统返回 `401`，不建立会话

### Requirement: Login rate limiting
系统 SHALL 对同一来源 IP 的登录失败进行限速：连续 10 次失败后，该 IP 在 5 分钟内的登录请求 MUST 返回 `429`。

#### Scenario: Brute force throttled
- **WHEN** 同一 IP 在短时间内连续 10 次登录失败后再次尝试
- **THEN** 系统返回 `429` 并附带 `Retry-After` 头

### Requirement: Session lifecycle
会话 SHALL 在 7 天无活动后过期；同一用户 MUST 能同时持有多个会话（多设备登录）；用户 MUST 能主动退出使当前会话立即失效；用户密码被修改或用户被禁用时，该用户的所有会话 MUST 立即失效。

#### Scenario: Logout invalidates session
- **WHEN** 已登录用户调用退出接口
- **THEN** 系统清除 Cookie，此后携带旧 Cookie 的请求返回 `401`

#### Scenario: Password change revokes sessions
- **WHEN** 管理员修改某用户的密码
- **THEN** 该用户所有已存在的会话失效，下一次请求返回 `401`

### Requirement: Role-based access
系统 SHALL 区分 `admin` 与 `user` 两种角色。管理类接口（用户管理、系统设置、全局用量、跨用户文件浏览）MUST 仅允许 `admin` 访问；文件浏览接口允许两种角色访问，但普通用户只能访问自己的根目录。

#### Scenario: User calls admin API
- **WHEN** 角色为 `user` 的会话请求用户管理接口
- **THEN** 系统返回 `403`

#### Scenario: Unauthenticated access to protected page
- **WHEN** 未登录的浏览器请求面板内的受保护页面
- **THEN** 系统跳转到登录页，登录成功后返回原页面

### Requirement: CSRF protection for state-changing requests
系统 SHALL 对所有会话 Cookie 认证的非幂等请求进行 CSRF 防护，来自其他站点的跨站表单提交 MUST 被拒绝。

#### Scenario: Cross-site POST rejected
- **WHEN** 请求携带有效会话 Cookie 但缺少合法的 CSRF 令牌或来源校验失败
- **THEN** 系统返回 `403` 且不执行操作

### Requirement: Self-service password change
任何已登录用户 SHALL 能修改自己的密码，且 MUST 提供当前密码进行验证。

#### Scenario: Change own password
- **WHEN** 用户提交正确的当前密码与合法的新密码
- **THEN** 系统更新密码，当前会话保持有效，其他会话失效

### Requirement: Insecure transport warning
系统 SHALL 允许通过明文 HTTP 使用面板与 WebDAV。当请求协议为 HTTP 且来源不是回环地址时，登录页与面板 MUST 显示醒目的不安全连接警告，说明密码将以明文传输并建议启用 TLS 或反向代理；警告 MUST NOT 阻止任何功能。协议判断 MUST 考虑受信任代理传来的 `X-Forwarded-Proto`。

#### Scenario: Warning shown over plain HTTP
- **WHEN** 用户从局域网另一台机器通过 `http://` 打开登录页
- **THEN** 页面显示不安全连接警告，登录仍可正常完成

#### Scenario: No warning behind TLS proxy
- **WHEN** 服务配置了受信任代理，代理以 HTTPS 对外并转发 `X-Forwarded-Proto: https`
- **THEN** 页面不显示警告

#### Scenario: No warning on localhost
- **WHEN** 用户通过 `http://127.0.0.1:8080` 访问
- **THEN** 页面不显示警告
