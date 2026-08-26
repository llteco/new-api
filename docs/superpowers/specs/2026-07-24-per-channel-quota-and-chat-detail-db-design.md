# 分渠道额度 + 对话详情独立库 设计文档

日期：2026-07-24
分支：`feature/per-channel-quota-and-chat-detail-db`

## 概述

两个独立但相关的功能，均围绕单个 API-Key（令牌）：

1. **分渠道额度**：将令牌额度细化为「每令牌 × 每渠道」独立额度，支持周期重置。
2. **对话详情独立库**：为指定令牌把完整请求/响应 JSON 存入独立数据库（独立 DSN/密码），仅管理员可见。

---

## 功能一：分渠道额度

### 模式语义（互斥模式）

令牌二选一：
- **总额度模式**（现状）：使用令牌 `remain_quota`。
- **分渠道模式**（新）：令牌 `remain_quota` 不再生效；每个渠道有独立 `remain_quota`。

由令牌上新字段 `ChannelQuotaMode bool` 控制。

### 数据模型

**Token 新增字段**（`model/token.go`）：
- `ChannelQuotaMode bool` `gorm:"default:0"`

**新表 `token_channel_quotas`**（`model/token_channel_quota.go`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int PK | |
| `token_id` | int | 索引 |
| `channel_id` | int | 索引 |
| `remain_quota` | int | `default:0` |
| `used_quota` | int | `default:0` |
| `reset_quota` | int | `default:0`，周期重置时恢复到此值 |

约束：`unique(token_id, channel_id)`。

> 不在行上存储 `reset_period` / `next_reset_time`。周期与重置时机由令牌自身的 `reset_period` + `next_reset_time` 统一驱动（共享周期，重置时各行恢复到各自的 `reset_quota`）。

未配置该渠道（无行）时：**不限额、不记录 used_quota**（不自动建行）。

### 周期重置

扩展现有 `model.MaybeResetTokenQuota(token *Token)`（`model/token_reset.go`）：

```
若 token.ChannelQuotaMode：
  1. 复用现有逻辑判断 token.NextResetTime 是否到点（period 来自令牌）。
  2. 到点后：对该令牌所有 token_channel_quotas 行批量执行
     remain_quota = reset_quota, used_quota = 0。
  3. 推进 token.NextResetTime（沿用 CalcNextTokenResetTime）。
  4. 失效该令牌的渠道额度缓存。
否则：走现有总额度重置逻辑。
```

调用方不变（仍在中继前调用 `MaybeResetTokenQuota`）。

### 计费注入点

改动集中在 service 层，不触及 40+ 渠道适配器。渠道在中继前已由 `middleware.Distribute()` 解析，`relayInfo.ChannelId` 在预扣费时可用。

**1. 预扣 `PreConsumeTokenQuota(relayInfo, quota)`**（`service/quota.go:387`）：

```
加载 token。
若 token.ChannelQuotaMode：
  查 TokenChannelQuota(tokenId, relayInfo.ChannelId)。
  行存在：
    若 remain_quota < quota → 返回「分渠道额度不足」错误（403, skip-retry）。
    model.DecreaseTokenChannelQuota(tokenId, channelId, quota)。
    返回特殊信号：本次扣减命中渠道账户。
  行不存在（未配置渠道）：
    不校验、不扣减 token 侧（视为该渠道不限额）。
    返回信号：未命中（token 侧零变更）。
否则：现有逻辑。
```

**2. `BillingSession`**（`service/billing_session.go`）：
- 新增字段：`channelQuotaMode bool`、`channelId int`、`channelConsumed int`。
- `preConsume` 调用 `PreConsumeTokenQuota` 后，根据返回信号设置上述字段。
- `Settle(actualQuota)`：
  - `channelQuotaMode && channelConsumed > 0` 时，delta 调整路由到 `DecreaseTokenChannelQuota` / `IncreaseTokenChannelQuota`。
  - 未配置渠道（无行）场景：token 侧不调整，仅资金侧（`funding.Settle`）正常结算。
- `Refund`：对称地退还到渠道账户（仅当 `channelConsumed > 0`）。

**3. 信任旁路**：`channelQuotaMode` 为真时强制禁用信任旁路（`shouldTrust` 返回 false），保证分渠道记账精确。

**4. 旧路径** `PreConsumeQuota`（`service/pre_consume_quota.go`）经同一 `PreConsumeTokenQuota` 函数，自动覆盖；但其基于 `c.GetInt("token_quota")` 的信任判断在分渠道模式下需跳过（令牌 `remain_quota` 不代表可用额度）。

**新增 model 函数**（`model/token_channel_quota.go`）：
- `GetTokenChannelQuota(tokenId, channelId) (*TokenChannelQuota, error)`
- `DecreaseTokenChannelQuota(tokenId, channelId, quota) error`（`remain_quota - ?, used_quota + ?`）
- `IncreaseTokenChannelQuota(tokenId, channelId, quota) error`
- `ResetAllTokenChannelQuotas(tokenId) error`（重置用）
- `GetAllTokenChannelQuotas(tokenId) ([]TokenChannelQuota, error)`
- `UpsertTokenChannelQuotas(tokenId, rows []TokenChannelQuota) error`（管理接口批量写）
- `DeleteTokenChannelQuota(tokenId, channelId) error`

> 计费安全：`DecreaseTokenChannelQuota` 复用现有 `gorm.Expr` 原子减法模式；负数入参在入口校验拒绝。quota 列为 32 位整数，沿用既有饱和约束（`common.QuotaFromFloat` 等不直接相关，因为分渠道额度由管理员整数配置，无浮点转换路径）。

### 缓存（Redis）

分渠道额度同样支持 Redis 加速（可选，二期）：
- key：`token_channel_quota:{tokenId}:{channelId}`
- 仿 `cacheIncrTokenQuota` / `cacheDecrTokenQuota`。
- 一期可不接入 Redis，仅 DB（功能正确性优先；如启用则与令牌缓存一同失效）。

### API

- `GET /api/token/{id}/channel_quotas` → 列出该令牌所有渠道额度行（含渠道名）。
- `PUT /api/token/{id}/channel_quotas` → 全量覆盖该令牌的渠道额度配置（管理员/令牌所有者）。
- 在令牌 `Update` 的字段列表加入 `channel_quota_mode`。

权限：与令牌编辑一致（管理员或令牌所有者）。

### 前端（`web/default`）

- 令牌编辑抽屉：新增「分渠道额度」开关。开启后：
  - 隐藏/禁用令牌总额度相关输入（互斥）。
  - 展示子表格：渠道选择器 + 每渠道额度输入 + `reset_quota`（默认=该渠道额度）。
  - 周期设置沿用令牌已有的 `reset_period` 选择器。
- i18n：所有新增文案走 `t()`，补全 zh/en/fr/ru/ja/vi。

### 数据库兼容

- 新表 AutoMigrate 加入 `migrateDB()`。
- `unique(token_id, channel_id)` 在三种库均支持。
- 不使用库专有类型。

---

## 功能二：对话详情独立库

### 触发与范围

- 按令牌开关：`Token.ChatLogEnabled bool`。
- 记录内容：完整请求 JSON + 完整响应 JSON；流式响应在服务端拼接为完整 JSON 后存；单条体积超上限截断并标记 `truncated`。
- 仅记录令牌已开启、且 `CHAT_LOG_SQL_DSN` 已配置的情形。

### 数据模型

**Token 新增字段**：
- `ChatLogEnabled bool` `gorm:"default:0"`

**新环境变量**：
- `CHAT_LOG_SQL_DSN`：独立库连接串（含独立账号密码）。未配置 → 功能静默禁用（即使令牌开启也跳过，启动打 warning）。
- 复用 `chooseDB`，支持 SQLite / MySQL / PostgreSQL。

**新句柄**：`model.CHATLOG_DB`（仿 `LOG_DB`）。
- `model.InitChatLogDB()`（仿 `InitLogDB`），在 `main.go` 启动流程中调用。
- `model.migrateChatLogDB()`（仿 `migrateLOGDB`），AutoMigrate `ChatLog`。

**新表 `chat_logs`**（独立库，`model/chat_log.go`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | int PK | |
| `token_id` | int | 索引 |
| `user_id` | int | 索引 |
| `channel_id` | int | 索引 |
| `model_name` | varchar(128) | |
| `request_id` | varchar(64) | 索引 |
| `request_body` | longtext | 截断后 |
| `response_body` | longtext | 截断后（流式已拼接） |
| `is_stream` | bool | |
| `truncated` | bool | 请求或响应超限截断 |
| `status_code` | int | 上游响应码 |
| `use_time` | int | 秒 |
| `created_at` | bigint | 索引 |

### 采集点（协议无关）

在 relay 入口（`controller/relay.go` 的统一处理处），当 `token.ChatLogEnabled && CHATLOG_DB != nil` 时：

1. **请求体**：复用 `common.BodyStorage.Bytes()`（中继已读取的请求体缓冲，避免二次读）。
2. **响应体**：安装 `gin.ResponseWriter` 包装器缓冲全部写出字节，上限 `ChatLogMaxBodyBytes`（默认 256KB，可经 Option 配置）。超限停止缓冲、置 `truncated=true`。
3. 请求结束（`defer`）异步落库：`gopool.Go` 写入 `CHATLOG_DB`。

> 流式与非流式统一：所有响应字节都经过 writer 包装器，天然覆盖。避免在每个渠道适配器里单独拼接。

**上限配置**：`setting` 新增 `chat_log_max_body_bytes`（默认 262144）。请求/响应各自截断。

### 访问（仅管理员）

新路由（挂在 admin 鉴权中间件之后）：
- `GET /api/chat_logs?token_id=&model_name=&start=&end=&page=&page_size=` → 分页列表（不含 body，仅元数据）。
- `GET /api/chat_logs/:id` → 单条详情（含 request_body / response_body）。

权限：仅管理员角色（root / admin）。普通用户、令牌所有者均不可见（按需求「仅供管理员访问」）。

### 前端（`web/default`，管理员页）

- 「对话详情」管理员页面：表格（时间、令牌、模型、渠道、流式、截断标记），筛选 + 分页。
- 点击行打开详情抽屉：JSON 查看器展示请求/响应（折叠/展开）。
- i18n 文案补齐。

### 数据库兼容

- 独立库 AutoMigrate，三种库通用。
- 大文本列采用方言感知：`migrateChatLogDB` 中在 MySQL 上用 `longtext`，PostgreSQL/SQLite 上用 `text`（后两者 text 无限长；MySQL longtext 4GB）。通过 `common.UsingLogDatabase`（或针对 chatlog 库的等价判断）分支执行原生 `ALTER`/建表列定义，避免 PG 拒绝未知类型名 `longtext`。

---

## 跨功能：Token 模型与迁移

`Token` 结构新增两个 bool：
- `ChannelQuotaMode bool`
- `ChatLogEnabled bool`

均加入 `migrateDB()` 的 AutoMigrate（GORM 自动加列，三种库兼容）。`Token.Update()` 的 `Select` 字段列表加入二者。

---

## 测试策略

**后端**（`github.com/stretchr/testify/require` + `assert`）：
- `model/token_channel_quota_test.go`：增删改查、原子扣减、重置批量、unique 约束。
- `model/token_reset_test.go` 扩展：分渠道模式的周期重置分支。
- `service/quota_test.go`：`PreConsumeTokenQuota` 分渠道分支（行存在/不存在/不足）、`BillingSession.Settle`/`Refund` 渠道路由。
- `model/chat_log_test.go`：落库、截断、独立库句柄。
- 计费安全：分渠道扣减不产生负数（入口拒绝负入参）。

**前端**：沿用现有模式（web/default 无强制单测框架时，类型 + 构建校验）。

---

## 非目标（YAGNI）

- 分渠道额度的 Redis 缓存（二期，一期 DB 即可保证正确）。
- 对话详情的全文检索/导出（先做浏览）。
- 分渠道额度的导入/批量模板。
- classic 主题同步（仅 default）。

---

## 风险与回退

- `ChannelQuotaMode` 默认 false，`ChatLogEnabled` 默认 false → 对存量令牌零行为变化。
- `CHAT_LOG_SQL_DSN` 默认未设 → 功能二完全静默。
- ResponseWriter 包装器仅在「令牌开启且库已配置」时安装，不影响其他请求性能。
- 互斥模式：令牌一旦切回总额度模式，分渠道行保留（不删），便于切回；UI 提示。
