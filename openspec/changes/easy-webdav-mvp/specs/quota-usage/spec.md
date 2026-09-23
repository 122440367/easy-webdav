## Purpose

定义按用户设置磁盘配额、写入时的强制执行，以及用户与全局用量统计在面板中的展示。

## ADDED Requirements

### Requirement: Per-user quota
管理员 SHALL 能为每个用户设置配额，单位为字节，面板以人类可读单位（MB、GB）输入与显示；`0` 或未设置表示不限制。

#### Scenario: Set quota
- **WHEN** 管理员将用户配额设为 `10 GB`
- **THEN** 用户列表显示该用户配额为 10 GB 及当前占用百分比

### Requirement: Quota enforcement on write
当写入操作（WebDAV PUT/COPY/MOVE 入根目录、网页上传）会导致用户占用超过配额时，系统 MUST 拒绝该操作，WebDAV MUST 返回 `507 Insufficient Storage`，网页 API MUST 返回 `413` 并附带剩余空间信息。已存在的文件被覆盖时，MUST 按新旧大小之差计算。

#### Scenario: PUT exceeding quota
- **WHEN** 用户剩余 100 MB，通过 WebDAV PUT 一个 200 MB 的文件
- **THEN** 系统返回 `507`，磁盘上不留下不完整文件

#### Scenario: Overwrite within quota
- **WHEN** 用户剩余 100 MB，用 150 MB 的新版本覆盖一个已有 100 MB 的文件
- **THEN** 写入成功，因为净增量为 50 MB

#### Scenario: Unknown content length
- **WHEN** 上传请求未声明长度（分块传输）
- **THEN** 系统在写入过程中持续计量，超限时中止并清理，返回 `507`

### Requirement: Usage accounting
系统 SHALL 维护每个用户根目录的占用字节数，按文件表观大小累计，目录本身与上传临时数据计 0；写入与删除后 MUST 在 5 秒内反映到面板；系统 SHALL 在启动时和每 24 小时对占用进行一次全量重算，以纠正外部直接修改文件造成的偏差。管理员 MUST 能手动触发重算。

#### Scenario: Usage updates after upload
- **WHEN** 用户上传一个 5 MB 文件
- **THEN** 面板中该用户的占用增加约 5 MB

#### Scenario: Manual recalculation
- **WHEN** 管理员在面板点击"重新计算用量"
- **THEN** 系统遍历所有用户根目录并更新占用，完成后显示结果

#### Scenario: Shared root directory accounting
- **WHEN** 两个用户共享同一根目录
- **THEN** 两者显示相同的占用值，任一用户的配额均以该值判断

### Requirement: Usage dashboard
面板 SHALL 为管理员展示全局概览：用户总数、活跃用户数、总占用、存储所在磁盘的剩余空间；SHALL 为每个用户展示自己的占用、配额与百分比。

#### Scenario: Admin overview
- **WHEN** 管理员打开面板首页
- **THEN** 页面显示上述四项全局指标以及占用最多的用户列表

#### Scenario: User sees own usage
- **WHEN** 普通用户登录
- **THEN** 页面顶部或侧栏显示其占用/配额进度条，配额为不限时只显示占用
