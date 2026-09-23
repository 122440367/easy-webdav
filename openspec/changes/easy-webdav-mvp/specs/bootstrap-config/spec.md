## Purpose

定义服务如何在没有任何配置的情况下启动、配置来源的优先级与配置文件的查找位置、数据目录的布局，以及首次运行时如何创建管理员账号，保证"下载即用"。

## ADDED Requirements

### Requirement: Zero-config startup
系统 SHALL 在不提供任何配置文件、环境变量或命令行参数的情况下成功启动，并使用内置默认值：监听 `0.0.0.0:8080`，数据目录为当前工作目录下的 `./data`，存储根目录为 `./data/files`。

#### Scenario: Start with no configuration
- **WHEN** 用户在空目录中直接执行二进制且不带任何参数
- **THEN** 系统创建 `./data` 与 `./data/files`，在 8080 端口开始监听，并在标准输出打印访问地址

#### Scenario: Data directory not writable
- **WHEN** 数据目录无法创建或不可写
- **THEN** 系统以非零退出码退出，并在标准错误输出明确指出失败的路径与原因

### Requirement: Configuration sources and precedence
系统 SHALL 按以下优先级合并配置，后者覆盖前者：内置默认值 < 配置文件 < 环境变量 < 命令行参数。环境变量 MUST 使用 `EW_` 前缀（例如 `EW_LISTEN`、`EW_DATA_DIR`、`EW_STORAGE_DIR`、`EW_BASE_URL`），命令行参数 MUST 使用长选项形式（例如 `--listen`、`--data-dir`、`--storage-dir`）。

#### Scenario: Environment variable overrides config file
- **WHEN** 配置文件将监听地址设为 `:9000`，同时环境变量 `EW_LISTEN=:9100`
- **THEN** 系统在 9100 端口监听

#### Scenario: Command-line flag overrides environment variable
- **WHEN** 环境变量 `EW_LISTEN=:9100`，同时命令行参数为 `--listen :9200`
- **THEN** 系统在 9200 端口监听

#### Scenario: Invalid configuration value
- **WHEN** 任一配置项的值不合法（例如监听地址格式错误）
- **THEN** 系统拒绝启动，并输出指出该配置项名称与来源的错误信息

### Requirement: Configuration file discovery
配置文件 SHALL 为 YAML 格式。未通过 `--config` 或 `EW_CONFIG` 指定路径时，系统 MUST 依次查找 `./config.yaml` 与 `<data-dir>/config.yaml`，使用找到的第一个；均不存在时 MUST 静默跳过配置文件层。系统 MUST NOT 自动生成配置文件。

#### Scenario: No config file present
- **WHEN** 两个默认位置均无 `config.yaml` 且未显式指定
- **THEN** 系统正常启动，仅使用默认值、环境变量与命令行参数，且不在磁盘上创建任何配置文件

#### Scenario: Explicit config path takes priority
- **WHEN** `./config.yaml` 存在，同时命令行指定 `--config /etc/easy-webdav/config.yaml`
- **THEN** 系统只读取显式指定的文件

#### Scenario: Explicit config path missing
- **WHEN** `--config` 指向一个不存在的文件
- **THEN** 系统拒绝启动并报告该路径

#### Scenario: Malformed config file
- **WHEN** 配置文件不是合法 YAML 或包含未知字段
- **THEN** 系统拒绝启动，并输出文件路径与出错位置

### Requirement: Effective configuration is inspectable
系统 SHALL 提供 `--print-config` 命令行参数，输出最终合并后的配置及每一项的来源，且 MUST 对密钥类字段脱敏。

#### Scenario: Print effective configuration
- **WHEN** 用户执行 `easy-webdav --print-config`
- **THEN** 系统输出所有配置项、生效值及来源（default/file/env/flag），随后退出且不启动服务

### Requirement: First-run admin setup
当系统中不存在任何管理员账号时，系统 SHALL 进入首次运行引导状态：面板首页 MUST 显示创建管理员的表单，且 MUST 拒绝所有 WebDAV 请求与除引导以外的 API 请求。首次创建的管理员与其他用户一样拥有根目录，默认为其用户名，权限为 `readwrite`。

#### Scenario: First visit creates admin
- **WHEN** 系统无管理员且用户在浏览器打开面板首页
- **THEN** 系统显示创建管理员表单；提交合法的用户名与密码后创建管理员并自动登录，其根目录 `<username>` 已在存储根目录下创建

#### Scenario: Setup endpoint locked after admin exists
- **WHEN** 已存在管理员且有人再次请求创建管理员的引导接口
- **THEN** 系统返回 `403` 并不创建任何账号

#### Scenario: Admin bootstrapped from environment
- **WHEN** 系统无管理员且启动时设置了 `EW_ADMIN_USER` 与 `EW_ADMIN_PASSWORD`
- **THEN** 系统在启动时用这两个值创建管理员并跳过网页引导；日志 MUST NOT 打印密码

### Requirement: Password policy at setup
系统 SHALL 要求管理员及所有用户密码长度不少于 8 个字符。

#### Scenario: Weak password rejected
- **WHEN** 用户在首次引导中提交少于 8 个字符的密码
- **THEN** 系统拒绝创建并提示密码长度要求
