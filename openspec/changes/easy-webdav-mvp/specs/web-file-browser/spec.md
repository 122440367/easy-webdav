## Purpose

定义登录用户在浏览器中直接浏览、上传、下载、整理与预览文件的行为，管理员跨用户浏览的行为，以及面板的语言与主题，使不挂载 WebDAV 客户端也能完成日常操作。

## ADDED Requirements

### Requirement: Directory listing
系统 SHALL 为已登录用户提供其根目录内任意子目录的列表，包含名称、类型、大小、修改时间；列表 MUST 支持按名称、大小、修改时间排序，并 MUST 通过面包屑导航返回上级目录。以 `.` 开头的文件与文件夹 MUST 默认隐藏，并提供"显示隐藏文件"开关，开关状态在同一浏览器内 MUST 保持。列表 MUST 一次返回目录全部条目，前端负责大列表的渲染性能。

#### Scenario: Browse subdirectory
- **WHEN** 用户点击一个文件夹
- **THEN** 页面显示该文件夹内容，地址栏路径同步更新，刷新页面后仍停留在该目录

#### Scenario: Empty directory
- **WHEN** 用户进入空目录
- **THEN** 页面显示空状态提示，并提供"上传"与"新建文件夹"入口

#### Scenario: Hidden files toggled
- **WHEN** 目录中存在 `.DS_Store` 与 `._photo.jpg`，用户未开启显示隐藏文件
- **THEN** 列表不显示这两项；开启开关后显示，刷新页面后仍为开启状态

### Requirement: Chunked upload
系统 SHALL 支持通过按钮选择与拖拽两种方式上传单个或多个文件，MUST 显示每个文件的进度，且 MUST 支持上传整个文件夹时保留目录结构。文件 MUST 以固定大小分块传输；单个分块失败时前端 MUST 在同一页面会话内自动重试该块，MUST NOT 要求整个文件重传。刷新或关闭页面后未完成的上传 MUST 被视为放弃，服务端 MUST 清理其临时数据。上传 MUST 受用户权限与配额约束。

#### Scenario: Drag and drop multiple files
- **WHEN** 用户将三个文件拖到当前目录区域
- **THEN** 三个文件依次或并行上传，各自显示进度，完成后列表自动刷新

#### Scenario: Chunk retry within session
- **WHEN** 上传一个 100 MB 文件的过程中某一分块因网络抖动失败
- **THEN** 前端自动重试该分块，其余已完成分块不重传，最终文件完整

#### Scenario: Abandoned upload cleaned
- **WHEN** 用户在上传中途关闭页面
- **THEN** 目标目录中不出现该文件，服务端临时数据在 24 小时内被清理，用量统计不包含临时数据

#### Scenario: Upload conflict
- **WHEN** 上传的文件名与当前目录中已有文件重名
- **THEN** 系统提示用户选择"覆盖"、"跳过"或"重命名保留两者"，默认不覆盖

#### Scenario: Upload rejected by quota
- **WHEN** 上传会使用户占用超过配额
- **THEN** 系统在上传前或上传中止时给出明确的配额不足提示，不留下部分写入的文件

### Requirement: Download
系统 SHALL 支持单文件下载；多选文件或文件夹时 SHALL 打包为 ZIP 流式下载，MUST NOT 先在服务端生成完整临时文件。

#### Scenario: Download folder as zip
- **WHEN** 用户选中一个文件夹并点击下载
- **THEN** 浏览器开始下载一个以该文件夹命名的 ZIP，内容包含完整目录结构

### Requirement: File and folder operations
`readwrite` 用户 SHALL 能新建文件夹、重命名、移动、复制与删除文件或文件夹；删除 MUST 有二次确认且为永久删除，不提供回收站；`read` 用户 MUST 看不到这些操作入口且相应接口返回 `403`。

#### Scenario: Rename file
- **WHEN** 用户将 `a.txt` 重命名为 `b.txt`
- **THEN** 列表中显示 `b.txt`，通过 WebDAV 访问也反映此更改

#### Scenario: Delete with confirmation
- **WHEN** 用户点击删除文件夹
- **THEN** 系统显示包含文件夹名称的确认对话框，确认后才永久删除

### Requirement: File preview
系统 SHALL 在浏览器内预览以下类型：图片（png、jpg、gif、webp、svg）、文本与代码（按扩展名高亮，仅限 2 MB 以内）、PDF、音频与视频（浏览器原生支持的格式）。其他类型显示文件信息并提供下载。列表 MUST NOT 生成图片缩略图，统一使用类型图标。

#### Scenario: Preview image
- **WHEN** 用户点击一张图片
- **THEN** 页面以覆盖层显示该图片，支持左右切换同目录中的其他图片

#### Scenario: Large text file not previewed
- **WHEN** 用户点击一个 5 MB 的 `.log` 文件
- **THEN** 页面显示文件信息与下载按钮，并提示文件过大无法预览

#### Scenario: Preview unsupported type
- **WHEN** 用户点击 `.exe` 文件
- **THEN** 页面显示文件名、大小、修改时间与下载按钮，不尝试渲染

### Requirement: Path safety in browser API
文件浏览 API 接收的所有路径 MUST 经过规范化并限制在目标根目录内；下载与预览响应 MUST 设置 `Content-Disposition` 与 `X-Content-Type-Options: nosniff`，HTML 类文件 MUST 以附件或纯文本方式返回，不得在面板同源下直接渲染。

#### Scenario: HTML file not rendered inline
- **WHEN** 用户预览一个 `.html` 文件
- **THEN** 系统以纯文本高亮显示源码，不执行其中脚本

### Requirement: Admin browsing of other users' directories
管理员 SHALL 能从用户管理页进入任意用户的根目录，并以 `readwrite` 权限执行本规范定义的全部文件操作，不受该用户自身权限级别限制。进入后页面顶部 MUST 持续显示"正在查看 <username> 的文件"提示条与退出入口；管理员默认的"我的文件"页面 MUST 仍为其自身根目录。在他人目录中的写入 MUST 按该目录的用量与该用户的配额计算。普通用户调用该能力 MUST 返回 `403`。

#### Scenario: Admin enters user directory
- **WHEN** 管理员在用户列表点击 alice 的"浏览文件"
- **THEN** 文件浏览器切换到 alice 的根目录，顶部显示提示条，面包屑根为 alice 的根目录

#### Scenario: Admin uploads into user directory
- **WHEN** 管理员在 alice 的目录中上传一个 100 MB 文件
- **THEN** 文件出现在 alice 的目录，alice 的用量增加 100 MB；若超过 alice 的配额则被拒绝

#### Scenario: Admin browses read-only user's directory
- **WHEN** alice 的权限为 `read`，管理员进入其目录并删除一个文件
- **THEN** 删除成功

#### Scenario: Non-admin attempts cross-user browsing
- **WHEN** 普通用户在请求中指定其他用户的标识
- **THEN** 系统返回 `403`

### Requirement: Localization
面板 SHALL 提供简体中文与英文两种界面语言，首次访问时 MUST 根据浏览器语言自动选择，无匹配时使用英文；用户 MUST 能手动切换，选择在同一浏览器内保持。API 错误响应 MUST 携带稳定的机器可读错误码，前端据此显示本地化文案。

#### Scenario: Browser language detected
- **WHEN** 浏览器首选语言为 `zh-CN`
- **THEN** 登录页与面板以简体中文显示

#### Scenario: Manual switch persists
- **WHEN** 用户将语言切换为英文并刷新页面
- **THEN** 页面仍为英文

### Requirement: Theme
面板 SHALL 提供明暗两种主题，默认跟随操作系统设置，用户 MUST 能手动固定为明或暗，选择在同一浏览器内保持。

#### Scenario: Follows system dark mode
- **WHEN** 操作系统处于深色模式且用户未手动选择
- **THEN** 面板以深色主题显示

#### Scenario: Manual override persists
- **WHEN** 用户手动选择浅色主题并刷新页面
- **THEN** 面板保持浅色，不随系统变化

### Requirement: Responsive layout
面板 SHALL 在桌面浏览器与移动浏览器上均可用，在宽度 375px 的屏幕上核心操作（浏览、上传、下载、删除）MUST 可用。

#### Scenario: Mobile browsing
- **WHEN** 用户在手机浏览器打开面板
- **THEN** 列表以适合窄屏的布局显示，操作菜单可通过长按或菜单按钮访问
