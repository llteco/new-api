# 分渠道额度 + 对话详情独立库 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为单个 API-Key 实现分渠道独立额度（互斥模式 + 周期重置），以及为指定令牌把完整请求/响应 JSON 存入独立数据库（管理员可见）。

**Architecture:**
- 功能一：新增 `Token.ChannelQuotaMode` 开关 + `token_channel_quotas` 表；计费逻辑集中在 `PreConsumeTokenQuota` 和 `BillingSession`，不动 40+ 渠道适配器；周期重置复用令牌的 `reset_period`。
- 功能二：新增 `Token.ChatLogEnabled` 开关 + `CHAT_LOG_SQL_DSN` 独立库句柄；协议无关的 `gin.ResponseWriter` 包装器统一捕获流式/非流式响应；管理员独占路由。
- 两功能独立可发，共享 Token 模型迁移。

**Tech Stack:** Go 1.22 + Gin + GORM v2；前端 React 19 + TypeScript + Rsbuild + Tailwind + i18next。数据库须 SQLite/MySQL/PostgreSQL 三库兼容。

设计文档：`docs/superpowers/specs/2026-07-24-per-channel-quota-and-chat-detail-db-design.md`

---

## 文件结构

### 功能一（分渠道额度）

| 操作 | 文件 | 职责 |
|------|------|------|
| 新建 | `model/token_channel_quota.go` | `TokenChannelQuota` 模型 + 增删改查 + 原子扣减 |
| 新建 | `model/token_channel_quota_test.go` | model 层测试 |
| 修改 | `model/token.go` | `Token` 加 `ChannelQuotaMode`；`Update()` Select 列表加该字段 |
| 修改 | `model/main.go` | `migrateDB()` / `migrateDBFast()` 注册 `TokenChannelQuota{}` |
| 修改 | `model/token_reset.go` | `MaybeResetTokenQuota` 增加分渠道重置分支 |
| 修改 | `service/quota.go` | `PreConsumeTokenQuota` 增加分渠道分支（返回是否命中） |
| 修改 | `service/pre_consume_quota.go` | 旧路径信任判断跳过分渠道模式 |
| 修改 | `service/billing_session.go` | `BillingSession` 加渠道字段；`Settle`/`Refund` 路由到渠道账户 |
| 修改 | `controller/token.go` | 新增 `GetTokenChannelQuotas` / `UpdateTokenChannelQuotas` handler |
| 修改 | `router/api-router.go` | 注册渠道额度路由 |
| 修改 | `web/default/src/features/tokens/**` | 令牌编辑 UI：分渠道开关 + 子表格 |
| 修改 | `web/default/src/i18n/locales/*.json` | 新文案翻译 |

### 功能二（对话详情库）

| 操作 | 文件 | 职责 |
|------|------|------|
| 新建 | `model/chat_log.go` | `ChatLog` 模型 + 落库 + 查询 |
| 新建 | `model/chat_log_test.go` | model 层测试 |
| 新建 | `service/chat_log_capture.go` | ResponseWriter 包装器 + 采集/落库编排 |
| 新建 | `controller/chat_log.go` | 管理员列表/详情 handler |
| 新建 | `router/chat-log-router.go` | 管理员路由注册 |
| 修改 | `model/main.go` | `CHATLOG_DB` 句柄 + `InitChatLogDB()` + `migrateChatLogDB()` |
| 修改 | `main.go` | 启动调用 `InitChatLogDB()` |
| 修改 | `model/token.go` | `Token` 加 `ChatLogEnabled` |
| 修改 | `controller/relay.go` | 中继入口安装采集器（条件式） |
| 修改 | `router/main.go` | 注册 chat-log 路由 |
| 修改 | `web/default/src/features/admin/**` | 管理员对话详情页 |
| 修改 | `web/default/src/i18n/locales/*.json` | 新文案翻译 |

---

# 阶段 A：分渠道额度

## Task A1：TokenChannelQuota 模型与迁移

**Files:**
- Create: `model/token_channel_quota.go`
- Modify: `model/token.go:14-35`（Token 结构加字段）、`model/token.go:296-311`（Update Select 列表）
- Modify: `model/main.go:271-302`（migrateDB）、`model/main.go:318-354`（migrateDBFast）
- Test: `model/token_channel_quota_test.go`

- [ ] **Step 1: 写失败的 model 测试**

新建 `model/token_channel_quota_test.go`：

```go
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenChannelQuota_CRUD(t *testing.T) {
	// 用现有测试 DB 初始化模式（参考 token_reset_test.go / controller 的 setupFlowControllerTestDB）
	db := setupTestDB(t) // 见 Step 3 提供的 helper

	tok := &Token{UserId: 1, Key: "sk-test-crud", RemainQuota: 0, Status: 1}
	require.NoError(t, tok.Insert())

	// Upsert
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 100, 5000))
	// Get
	row, err := GetTokenChannelQuota(tok.Id, 100)
	require.NoError(t, err)
	assert.Equal(t, 5000, row.RemainQuota)
	assert.Equal(t, 5000, row.ResetQuota)
	assert.Equal(t, 0, row.UsedQuota)

	// Decrease（原子）
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 100, 300))
	row, _ = GetTokenChannelQuota(tok.Id, 100)
	assert.Equal(t, 4700, row.RemainQuota)
	assert.Equal(t, 300, row.UsedQuota)

	// Increase（退还）
	require.NoError(t, IncreaseTokenChannelQuota(tok.Id, 100, 100))
	row, _ = GetTokenChannelQuota(tok.Id, 100)
	assert.Equal(t, 4800, row.RemainQuota)
	assert.Equal(t, 200, row.UsedQuota)

	// 未配置渠道返回 ErrNotFound（sql.ErrNoRows 包装）
	_, err = GetTokenChannelQuota(tok.Id, 999)
	assert.Error(t, err)

	// ResetAll
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 200, 1000))
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 200, 400))
	require.NoError(t, ResetAllTokenChannelQuotas(tok.Id))
	row100, _ := GetTokenChannelQuota(tok.Id, 100)
	row200, _ := GetTokenChannelQuota(tok.Id, 200)
	assert.Equal(t, 5000, row100.RemainQuota)
	assert.Equal(t, 0, row100.UsedQuota)
	assert.Equal(t, 1000, row200.RemainQuota)
	assert.Equal(t, 0, row200.UsedQuota)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./model/ -run TestTokenChannelQuota_CRUD -v`
Expected: 编译失败（函数未定义 / setupTestDB 未定义）。

- [ ] **Step 3: 实现 model 层**

新建 `model/token_channel_quota.go`：

```go
package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type TokenChannelQuota struct {
	Id          int   `json:"id" gorm:"primaryKey"`
	TokenId     int   `json:"token_id" gorm:"uniqueIndex:idx_token_channel,priority:1;index"`
	ChannelId   int   `json:"channel_id" gorm:"uniqueIndex:idx_token_channel,priority:2;index"`
	RemainQuota int   `json:"remain_quota" gorm:"default:0"`
	UsedQuota   int   `json:"used_quota" gorm:"default:0"`
	ResetQuota  int   `json:"reset_quota" gorm:"default:0"`
}

func (TokenChannelQuota) TableName() string {
	return "token_channel_quotas"
}

// GetTokenChannelQuota 读取单行；行不存在时返回包装错误（调用方据 errors.Is 判断）。
func GetTokenChannelQuota(tokenId, channelId int) (*TokenChannelQuota, error) {
	var row TokenChannelQuota
	err := DB.Where("token_id = ? AND channel_id = ?", tokenId, channelId).First(&row).Error
	return &row, err
}

// UpsertTokenChannelQuota 插入或更新一行，remain_quota 同步设为 reset_quota。
func UpsertTokenChannelQuota(tokenId, channelId, resetQuota int) error {
	row := TokenChannelQuota{
		TokenId: tokenId, ChannelId: channelId,
		RemainQuota: resetQuota, ResetQuota: resetQuota,
	}
	return DB.Where("token_id = ? AND channel_id = ?", tokenId, channelId).
		Assign(row).FirstOrCreate(&row).Error
}

// DecreaseTokenChannelQuota 原子扣减：remain_quota - quota, used_quota + quota。
func DecreaseTokenChannelQuota(tokenId, channelId, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数")
	}
	return DB.Model(&TokenChannelQuota{}).
		Where("token_id = ? AND channel_id = ?", tokenId, channelId).
		Updates(map[string]interface{}{
			"remain_quota": gorm.Expr("remain_quota - ?", quota),
			"used_quota":   gorm.Expr("used_quota + ?", quota),
		}).Error
}

// IncreaseTokenChannelQuota 原子退还。
func IncreaseTokenChannelQuota(tokenId, channelId, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数")
	}
	return DB.Model(&TokenChannelQuota{}).
		Where("token_id = ? AND channel_id = ?", tokenId, channelId).
		Updates(map[string]interface{}{
			"remain_quota": gorm.Expr("remain_quota + ?", quota),
			"used_quota":   gorm.Expr("used_quota - ?", quota),
		}).Error
}

// ResetAllTokenChannelQuotas 周期重置：该令牌所有行恢复到 reset_quota，清零 used。
func ResetAllTokenChannelQuotas(tokenId int) error {
	return DB.Model(&TokenChannelQuota{}).
		Where("token_id = ?", tokenId).
		Updates(map[string]interface{}{
			"remain_quota": gorm.Expr("reset_quota"),
			"used_quota":   0,
		}).Error
}

// GetAllTokenChannelQuotas 列出令牌所有渠道额度行（含渠道名，管理员视图）。
func GetAllTokenChannelQuotas(tokenId int) ([]TokenChannelQuota, error) {
	var rows []TokenChannelQuota
	err := DB.Where("token_id = ?", tokenId).Order("channel_id asc").Find(&rows).Error
	return rows, err
}

// ReplaceTokenChannelQuotas 全量覆盖（管理接口）：删除旧行后批量插入。
func ReplaceTokenChannelQuotas(tokenId int, items []TokenChannelQuota) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token_id = ?", tokenId).Delete(&TokenChannelQuota{}).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].Id = 0
			items[i].TokenId = tokenId
			items[i].RemainQuota = items[i].ResetQuota
			items[i].UsedQuota = 0
			if err := tx.Create(&items[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// 用于 common.SysError 之类引用，避免 import 未使用（如不需要可删）
var _ = common.SysLog
```

在 `model/token_channel_quota_test.go` 顶部补测试 DB helper（参考现有 `controller/usedata_flow_test.go` 的 `setupFlowControllerTestDB` 与 `controller/model_list_test.go` 的 `model.InitDB()` 模式；这里用内存 SQLite）：

```go
func setupTestDB(t *testing.T) *gorm.DB {
	// 复用项目现有测试初始化模式：设置 SQL_DSN 为 local 内存库后 InitDB。
	t.Helper()
	// 参考 model/token_reset_test.go —— 若项目已有全局 testMain，直接用 model.DB。
	// 此处假定测试在已初始化的 model.DB 上运行（与 token_reset_test.go 同一 fixture）。
	// 如果项目无统一 testMain，在此手动 Open 一个内存 sqlite 并赋给 model.DB。
	return DB
}
```

> 注：Step 3 的 helper 需适配项目现有测试 fixture。实现时先检查 `model/token_reset_test.go` 是否依赖全局 `model.DB`；若是，则本测试沿用同一 fixture，`setupTestDB` 仅 `return DB` 并确保 `TestMain` 已初始化（参考 `controller/model_list_test.go:88` 的 `model.InitDB()` 调用）。

- [ ] **Step 4: Token 模型加字段**

修改 `model/token.go`，在 `Token` 结构体 `CrossGroupRetry` 之后加：

```go
	ChannelQuotaMode bool           `json:"channel_quota_mode" gorm:"default:0"`
```

修改 `model/token.go` 的 `Update()` 方法 Select 列表（约 307-309 行），加入 `"channel_quota_mode"`：

```go
	err = DB.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
		"model_limits_enabled", "model_limits", "allow_ips", "group", "cross_group_retry",
		"reset_period", "reset_quota", "next_reset_time", "channel_quota_mode").Updates(token).Error
```

- [ ] **Step 5: 注册迁移**

修改 `model/main.go` 的 `migrateDB()`（约 271 行 AutoMigrate 调用列表）加入 `&TokenChannelQuota{}`：

```go
	err := DB.AutoMigrate(
		&Channel{},
		// ... 既有 ...
		&AuthzRole{},
		&TokenChannelQuota{},
	)
```

同样修改 `migrateDBFast()`（约 322 行 migrations 切片）加入：

```go
		{&TokenChannelQuota{}, "TokenChannelQuota"},
```

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./model/ -run TestTokenChannelQuota_CRUD -v`
Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add model/token_channel_quota.go model/token_channel_quota_test.go model/token.go model/main.go
git commit -m "feat(model): 分渠道额度表与 Token.ChannelQuotaMode 字段"
```

---

## Task A2：分渠道周期重置

**Files:**
- Modify: `model/token_reset.go:52-110`（MaybeResetTokenQuota）
- Test: `model/token_reset_test.go`（扩展）

- [ ] **Step 1: 写失败测试**

在 `model/token_reset_test.go` 末尾追加：

```go
func TestMaybeResetTokenQuota_ChannelMode(t *testing.T) {
	tok := &Token{
		UserId: 1, Key: "sk-reset-chan", Status: 1,
		ChannelQuotaMode: true,
		ResetPeriod:      TokenResetMonthly,
		NextResetTime:    time.Now().Add(-1 * time.Hour).Unix(),
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 10, 5000))
	require.NoError(t, UpsertTokenChannelQuota(tok.Id, 20, 8000))
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 10, 2000))
	require.NoError(t, DecreaseTokenChannelQuota(tok.Id, 20, 3000))

	require.True(t, MaybeResetTokenQuota(tok))

	r10, _ := GetTokenChannelQuota(tok.Id, 10)
	r20, _ := GetTokenChannelQuota(tok.Id, 20)
	assert.Equal(t, 5000, r10.RemainQuota)
	assert.Equal(t, 0, r10.UsedQuota)
	assert.Equal(t, 8000, r20.RemainQuota)
	assert.Equal(t, 0, r20.UsedQuota)
	// NextResetTime 推进到未来
	assert.Greater(t, tok.NextResetTime, time.Now().Unix())
}
```

确保文件顶部已 `import "github.com/stretchr/testify/require"` 与 `"github.com/stretchr/testify/assert"`（按需补充）。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./model/ -run TestMaybeResetTokenQuota_ChannelMode -v`
Expected: FAIL（重置未触达渠道行，UsedQuota 不为 0）。

- [ ] **Step 3: 实现重置分支**

修改 `model/token_reset.go` 的 `MaybeResetTokenQuota`。在 `period == TokenResetNever` 判断之后、加载 `fresh` 之前，插入分渠道模式分支。改造后的函数结构：

```go
func MaybeResetTokenQuota(token *Token) bool {
	if token == nil {
		return false
	}
	now := common.GetTimestamp()
	if token.NextResetTime <= 0 || token.NextResetTime > now {
		return false
	}
	period := NormalizeTokenResetPeriod(token.ResetPeriod)
	if period == TokenResetNever {
		return false
	}

	// ---- 分渠道模式 ----
	if token.ChannelQuotaMode {
		return maybeResetTokenChannelQuotas(token, period, now)
	}

	// ---- 既有总额度逻辑（保持不变）----
	var fresh Token
	if err := DB.Where("id = ?", token.Id).First(&fresh).Error; err != nil {
		return false
	}
	// ...（以下保持原样，直到函数末尾）...
}
```

在文件末尾新增：

```go
// maybeResetTokenChannelQuotas 分渠道模式的周期重置。
// 重置时机/周期沿用令牌自身字段；到点后该令牌所有渠道行恢复到各自的 reset_quota。
func maybeResetTokenChannelQuotas(token *Token, period string, now int64) bool {
	var fresh Token
	if err := DB.Where("id = ?", token.Id).First(&fresh).Error; err != nil {
		return false
	}
	// 用最新 NextResetTime 复判，避免并发重复重置
	if fresh.NextResetTime <= 0 || fresh.NextResetTime > now {
		token.NextResetTime = fresh.NextResetTime
		return false
	}

	resetCount := 0
	base := time.Unix(fresh.NextResetTime, 0)
	for fresh.NextResetTime > 0 && fresh.NextResetTime <= now {
		next := CalcNextTokenResetTime(base, period)
		if next <= 0 || next == fresh.NextResetTime {
			break
		}
		fresh.NextResetTime = next
		base = time.Unix(next, 0)
		resetCount++
	}
	if resetCount == 0 {
		return false
	}

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&TokenChannelQuota{}).
			Where("token_id = ?", fresh.Id).
			Updates(map[string]interface{}{
				"remain_quota": gorm.Expr("reset_quota"),
				"used_quota":   0,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&Token{}).Where("id = ?", fresh.Id).
			Update("next_reset_time", fresh.NextResetTime).Error
	}); err != nil {
		return false
	}

	_ = cacheDeleteToken(token.Key)
	token.NextResetTime = fresh.NextResetTime
	return true
}
```

> `gorm` 与 `time` 已在 token_reset.go 隐式可用？token_reset.go 当前只 import `time` 和 common。需新增 import `"gorm.io/gorm"`。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./model/ -run TestMaybeResetTokenQuota -v`
Expected: 两个 reset 测试均 PASS（新测试 + 既有 simulateResetLoop 不受影响）。

- [ ] **Step 5: 提交**

```bash
git add model/token_reset.go model/token_reset_test.go
git commit -m "feat(model): 分渠道额度周期重置分支"
```

---

## Task A3：预扣费分渠道分支

**Files:**
- Modify: `service/quota.go:387-409`（PreConsumeTokenQuota）
- Modify: `service/pre_consume_quota.go:33-79`（信任判断跳过分渠道）

- [ ] **Step 1: 写失败测试**

新建 `service/quota_channel_test.go`：

```go
package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreConsumeTokenQuota_ChannelMode_HitRow(t *testing.T) {
	tok := &model.Token{
		UserId: 1, Key: "sk-pc-chan", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 42, 1000))

	info := &common.RelayInfo{TokenId: tok.Id, TokenKey: tok.Key, ChannelId: 42}
	hit, err := PreConsumeTokenQuotaChannel(info, 300)
	require.NoError(t, err)
	assert.True(t, hit)

	row, _ := model.GetTokenChannelQuota(tok.Id, 42)
	assert.Equal(t, 700, row.RemainQuota)
	assert.Equal(t, 300, row.UsedQuota)
}

func TestPreConsumeTokenQuota_ChannelMode_Insufficient(t *testing.T) {
	tok := &model.Token{
		UserId: 1, Key: "sk-pc-chan-ins", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 42, 100))

	info := &common.RelayInfo{TokenId: tok.Id, TokenKey: tok.Key, ChannelId: 42}
	_, err := PreConsumeTokenQuotaChannel(info, 300)
	require.Error(t, err)
}

func TestPreConsumeTokenQuota_ChannelMode_NoRow_NoLimit(t *testing.T) {
	tok := &model.Token{
		UserId: 1, Key: "sk-pc-chan-norow", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	// 不为渠道 99 配置额度

	info := &common.RelayInfo{TokenId: tok.Id, TokenKey: tok.Key, ChannelId: 99}
	hit, err := PreConsumeTokenQuotaChannel(info, 300)
	require.NoError(t, err)
	assert.False(t, hit) // 未配置 → 不限额、不扣减
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./service/ -run TestPreConsumeTokenQuota_ChannelMode -v`
Expected: 编译失败（`PreConsumeTokenQuotaChannel` 未定义）。

- [ ] **Step 3: 实现分渠道预扣函数**

在 `service/quota.go` 中，保留原 `PreConsumeTokenQuota`（旧路径仍调用），新增带返回信号的版本：

```go
import (
	"errors"
	// 既有 import 保持

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	// ...
)

// PreConsumeTokenQuotaChannel 在分渠道模式下预扣。
// 返回 hit=true 表示命中渠道额度行并已扣减；hit=false 表示该渠道未配置（不限额、不扣减）。
func PreConsumeTokenQuotaChannel(relayInfo *relaycommon.RelayInfo, quota int) (hit bool, err error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return false, nil
	}
	token, err := model.GetTokenByKey(relayInfo.TokenKey, false)
	if err != nil {
		return false, err
	}
	if !token.ChannelQuotaMode {
		// 非分渠道模式，回退总额度逻辑（保持单一入口）
		return false, PreConsumeTokenQuota(relayInfo, quota)
	}
	row, rowErr := model.GetTokenChannelQuota(relayInfo.TokenId, relayInfo.ChannelId)
	if rowErr != nil {
		// 行不存在 → 未配置渠道：不限额、不扣减
		return false, nil
	}
	if row.RemainQuota < quota {
		return true, fmt.Errorf("分渠道额度不足, 渠道 %d 剩余: %s, 需要: %s",
			relayInfo.ChannelId, logger.FormatQuota(row.RemainQuota), logger.FormatQuota(quota))
	}
	if err := model.DecreaseTokenChannelQuota(relayInfo.TokenId, relayInfo.ChannelId, quota); err != nil {
		return true, err
	}
	return true, nil
}
```

> 原有 `PreConsumeTokenQuota` 函数体保持不变（BillingSession 旧调用点逐步迁移到新函数；本 Task 先提供新函数，Task A4 接入 BillingSession）。

- [ ] **Step 4: 旧路径信任判断跳过分渠道模式**

修改 `service/pre_consume_quota.go` 的 `PreConsumeQuota`，在读取 `token_quota` 前增加令牌模式判断。在 `relayInfo.UserQuota = userQuota`（约 47 行）之前插入：

```go
	// 分渠道模式下，令牌 remain_quota 不代表可用额度，禁用信任旁路、强制预扣
	tokenForMode, err := model.GetTokenByKey(relayInfo.TokenKey, false)
	if err == nil && tokenForMode.ChannelQuotaMode {
		// 跳过信任判断：直接进入下面的 preConsumedQuota > 0 预扣分支
		// （preConsumedQuota 保持入参值，不归零）
	} else if err == nil {
		// 既有信任判断逻辑（保留原 if userQuota > trustQuota 块）
		// 见下文重构说明
	}
```

> 重构说明：将原 `if userQuota > trustQuota { ... }` 整块用 `else if tokenForMode != nil && !tokenForMode.ChannelQuotaMode` 包裹，使得分渠道模式时 `preConsumedQuota` 不被归零、一定走预扣。`PreConsumeTokenQuota` 调用点改为调用 `PreConsumeTokenQuotaChannel`（Task A4 后 BillingSession 是主路径；此旧函数为兼容保留，仍需正确）。

具体地，把 `PreConsumeQuota` 中：
```go
	err := PreConsumeTokenQuota(relayInfo, preConsumedQuota)
```
改为：
```go
	_, err := PreConsumeTokenQuotaChannel(relayInfo, preConsumedQuota)
```

并在信任判断 `if userQuota > trustQuota {` 外层加 `tokenForMode.ChannelQuotaMode` 短路：分渠道模式时整个信任块不执行。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./service/ -run TestPreConsumeTokenQuota_ChannelMode -v`
Expected: 三个测试 PASS。

- [ ] **Step 6: 提交**

```bash
git add service/quota.go service/pre_consume_quota.go service/quota_channel_test.go
git commit -m "feat(service): 预扣费分渠道分支"
```

---

## Task A4：BillingSession 渠道路由

**Files:**
- Modify: `service/billing_session.go:25-36`（结构体）、`:41-79`（Settle）、`:82-123`（Refund）、`:152-178`（Reserve）、`:186-230`（preConsume）、`:282-315`（shouldTrust）

- [ ] **Step 1: 写失败测试**

新建 `service/billing_session_channel_test.go`：

```go
package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingSession_SettleChannelMode(t *testing.T) {
	tok := &model.Token{
		UserId: 1, Key: "sk-bs-chan", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 7, 1000))

	info := &common.RelayInfo{
		UserId: 1, TokenId: tok.Id, TokenKey: tok.Key, ChannelId: 7,
	}
	// 构造 BillingSession：预扣 300 后，实际消耗 400，补扣 100
	s := newBillingSessionForTest(info) // 测试 helper，见 Step 3

	// 预扣阶段
	require.NoError(t, s.preConsumeForTest(nil, 300))
	row, _ := model.GetTokenChannelQuota(tok.Id, 7)
	assert.Equal(t, 700, row.RemainQuota)

	// 结算：实际 400，补扣 100
	require.NoError(t, s.Settle(400))
	row, _ = model.GetTokenChannelQuota(tok.Id, 7)
	assert.Equal(t, 600, row.RemainQuota)
	assert.Equal(t, 400, row.UsedQuota)
}

func TestBillingSession_RefundChannelMode(t *testing.T) {
	tok := &model.Token{
		UserId: 1, Key: "sk-bs-chan-ref", Status: 1,
		ChannelQuotaMode: true,
	}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 7, 1000))

	info := &common.RelayInfo{
		UserId: 1, TokenId: tok.Id, TokenKey: tok.Key, ChannelId: 7,
	}
	s := newBillingSessionForTest(info)
	require.NoError(t, s.preConsumeForTest(nil, 300))
	row, _ := model.GetTokenChannelQuota(tok.Id, 7)
	assert.Equal(t, 700, row.RemainQuota)

	s.Refund(nil) // 失败退还
	// Refund 异步（gopool.Go），简单 sleep 等待
	// 改用同步退还断言：直接校验 NeedsRefund 后调 Refund，再查询
	// 为确定性，测试改为调用同步的 refundTokenChannelQuota 等价路径，
	// 或在 Refund 内对 channelConsumed 同步退还（见 Step 3 说明）
}
```

> Step 3 实现 `Refund` 时，渠道额度退还放在锁内同步执行（资金来源仍异步），使测试可确定性断言。若设计上必须异步，则测试用 `require.Eventually` 轮询。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./service/ -run TestBillingSession_SettleChannelMode -v`
Expected: 编译失败（`newBillingSessionForTest` 等未定义）。

- [ ] **Step 3: 实现 BillingSession 渠道字段与路由**

修改 `service/billing_session.go`：

(a) `BillingSession` 结构体新增字段（约 25-36 行）：

```go
type BillingSession struct {
	relayInfo        *relaycommon.RelayInfo
	funding          FundingSource
	preConsumedQuota int
	tokenConsumed    int
	extraReserved    int
	trusted          bool
	fundingSettled   bool
	settled          bool
	refunded         bool
	mu               sync.Mutex

	// 分渠道额度模式
	channelQuotaMode bool  // 令牌处于分渠道模式
	channelId        int   // 命中的渠道
	channelHit       bool  // 是否命中已配置的渠道行（true=已扣渠道账户）
	channelConsumed  int   // 渠道账户实际扣减量（用于退还/结算）
}
```

(b) `preConsume` 方法（约 186-230 行）：把对 `PreConsumeTokenQuota` 的调用替换为 `PreConsumeTokenQuotaChannel`，并记录渠道状态：

```go
	// ---- 1) 预扣令牌额度 ----
	if effectiveQuota > 0 {
		// 判断分渠道模式
		tok, _ := model.GetTokenByKey(s.relayInfo.TokenKey, false)
		if tok != nil && tok.ChannelQuotaMode {
			s.channelQuotaMode = true
			s.channelId = s.relayInfo.ChannelId
		}
		if s.channelQuotaMode {
			hit, err := PreConsumeTokenQuotaChannel(s.relayInfo, effectiveQuota)
			if err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			s.channelHit = hit
			if hit {
				s.tokenConsumed = effectiveQuota
				s.channelConsumed = effectiveQuota
			}
			// hit=false（未配置渠道）：不扣 token 侧，仅资金侧预扣
		} else {
			if err := PreConsumeTokenQuota(s.relayInfo, effectiveQuota); err != nil {
				return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			}
			s.tokenConsumed = effectiveQuota
		}
	}
```

> 资金来源预扣失败回滚处（原 `model.IncreaseTokenQuota`），改为：若 `channelConsumed > 0` 用 `model.IncreaseTokenChannelQuota`，否则原逻辑。

(c) `Settle` 方法（约 41-79 行）：第 2 步令牌额度调整改为按模式路由：

```go
	// 2) 调整令牌额度
	var tokenErr error
	if !s.relayInfo.IsPlayground {
		if s.channelQuotaMode && s.channelHit {
			// 分渠道模式
			if delta > 0 {
				tokenErr = model.DecreaseTokenChannelQuota(s.relayInfo.TokenId, s.channelId, delta)
			} else if delta < 0 {
				tokenErr = model.IncreaseTokenChannelQuota(s.relayInfo.TokenId, s.channelId, -delta)
			}
		} else if s.channelQuotaMode && !s.channelHit {
			// 分渠道模式但该渠道未配置：token 侧不调整
			tokenErr = nil
		} else {
			// 既有总额度逻辑
			if delta > 0 {
				tokenErr = model.DecreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, delta)
			} else {
				tokenErr = model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, -delta)
			}
		}
		if tokenErr != nil {
			common.SysLog(fmt.Sprintf("error adjusting token quota after funding settled (userId=%d, tokenId=%d, delta=%d): %s",
				s.relayInfo.UserId, s.relayInfo.TokenId, delta, tokenErr.Error()))
		}
	}
```

(d) `Refund` 方法（约 82-123 行）：异步闭包内令牌退还增加渠道分支。复制闭包变量时新增 `channelQuotaMode, channelId, channelHit, channelConsumed`；闭包内：

```go
		// 2) 退还令牌额度
		if tokenConsumed > 0 && !isPlayground {
			if channelQuotaMode && channelHit {
				if err := model.IncreaseTokenChannelQuota(tokenId, channelId, tokenConsumed); err != nil {
					common.SysLog("error refunding channel quota: " + err.Error())
				}
			} else if !channelQuotaMode {
				if err := model.IncreaseTokenQuota(tokenId, tokenKey, tokenConsumed); err != nil {
					common.SysLog("error refunding token quota: " + err.Error())
				}
			}
		}
```

(e) `Reserve` 方法（约 152-178 行）：`reserveToken` 调用点（约 168 行）之后，`s.tokenConsumed += delta` 改为按模式分别累加：

```go
	if err := s.reserveToken(delta); err != nil {
		s.rollbackFundingReserve(delta)
		return err
	}

	s.preConsumedQuota += delta
	if s.channelQuotaMode && s.channelHit {
		s.channelConsumed += delta
	} else if !s.channelQuotaMode {
		s.tokenConsumed += delta
	}
	// channelQuotaMode && !channelHit：token 侧无变化
	s.extraReserved += delta
	s.syncRelayInfo()
```

`reserveToken`（约 271-279 行）内部：分渠道模式调 `PreConsumeTokenQuotaChannel`：

```go
func (s *BillingSession) reserveToken(delta int) error {
	if delta <= 0 || s.relayInfo.IsPlayground {
		return nil
	}
	if s.channelQuotaMode {
		_, err := PreConsumeTokenQuotaChannel(s.relayInfo, delta)
		return err
	}
	if err := PreConsumeTokenQuota(s.relayInfo, delta); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return nil
}
```

(f) `shouldTrust`（约 282-315 行）：开头加分渠道短路：

```go
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 分渠道模式强制精确记账，禁用信任旁路
	if s.relayInfo != nil {
		if tok, _ := model.GetTokenByKey(s.relayInfo.TokenKey, false); tok != nil && tok.ChannelQuotaMode {
			return false
		}
	}
	// ... 既有逻辑 ...
}
```

> 性能：`shouldTrust` 每请求一次 `GetTokenByKey(false)`（命中 Redis 缓存），可接受。或更优：在 `NewBillingSession` 构造时一次性加载 token 到 session 字段复用。实现时优先在 preConsume 已加载的 token 上判断（preConsume 先于 shouldTrust？当前 preConsume 调 shouldTrust，故 shouldTrust 内需自行加载）。推荐：构造 session 时缓存 `channelQuotaMode`，避免重复查询——在 NewBillingSession 里 `tok, _ := model.GetTokenByKey(...); s.channelQuotaMode = tok != nil && tok.ChannelQuotaMode`。

(g) 测试 helper（同文件 `service/billing_session_channel_test.go` 或独立）：

```go
// newBillingSessionForTest 构造一个最小可用 BillingSession（资金来源用 WalletFunding，预扣前状态）。
func newBillingSessionForTest(info *common.RelayInfo) *BillingSession {
	tok, _ := model.GetTokenByKey(info.TokenKey, true)
	s := &BillingSession{
		relayInfo: info,
		funding:   &WalletFunding{userId: info.UserId},
	}
	if tok != nil {
		s.channelQuotaMode = tok.ChannelQuotaMode
		s.channelId = info.ChannelId
	}
	return s
}

func (s *BillingSession) preConsumeForTest(c interface{}, quota int) error {
	// 直接复用内部逻辑的最小封装；若 preConsume 签名是 (*gin.Context, int)，
	// 测试可构造一个最小 gin.Context 或将该逻辑抽为不依赖 context 的内部函数。
	// 简化：测试改为直接调用预扣 + 设置字段，不走完整 preConsume。
	return nil
}
```

> 实现注意：`preConsume` 依赖 `*gin.Context`。测试可通过构造 `gin.CreateTestContext(nil)` 或抽取一个不依赖 context 的 `preConsumeLocked(quota)` 内部函数供测试与 preConsume 共用。本 Task 实现时采用后者（抽取 `preConsumeLocked`，preConsume 仅做 context 透传）。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./service/ -run TestBillingSession_ -v`
Expected: PASS。

Run: `go build ./...`
Expected: 编译通过（确认所有 BillingSession 调用点签名兼容）。

- [ ] **Step 5: 提交**

```bash
git add service/billing_session.go service/billing_session_channel_test.go
git commit -m "feat(service): BillingSession 分渠道额度路由"
```

---

## Task A5：管理 API（渠道额度增删改查）

**Files:**
- Modify: `controller/token.go`（新增 handler）
- Modify: `router/api-router.go:232-243`（tokenRoute 注册子路由）

- [ ] **Step 1: 写失败测试**

新建 `controller/token_channel_quota_handler_test.go`：

```go
package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTokenChannelQuotas(t *testing.T) {
	// 初始化测试 DB 与路由（参考 controller/token_test.go 的 setup 模式）
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	tok := &model.Token{UserId: 1, Key: "sk-api-chan", Status: 1, ChannelQuotaMode: true}
	require.NoError(t, tok.Insert())
	require.NoError(t, model.UpsertTokenChannelQuota(tok.Id, 5, 2000))

	r.Use(func(c *gin.Context) { c.Set("id", 1); c.Set("role", 100); c.Next() })
	r.GET("/api/token/:id/channel_quotas", controller.GetTokenChannelQuotas)

	req := httptest.NewRequest(http.MethodGet, "/api/token/"+strconv.Itoa(tok.Id)+"/channel_quotas", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []model.TokenChannelQuota `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, 5, resp.Data[0].ChannelId)
}
```

> 复用 `controller/token_test.go` 现有的测试 setup（DB 初始化、auth 注入）。`strconv`、`controller` import 按需补。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./controller/ -run TestGetTokenChannelQuotas -v`
Expected: 编译失败（`controller.GetTokenChannelQuotas` 未定义）。

- [ ] **Step 3: 实现 handler**

在 `controller/token.go` 末尾新增：

```go
// GetTokenChannelQuotas 列出令牌的分渠道额度配置（令牌所有者或管理员）。
func GetTokenChannelQuotas(c *gin.Context) {
	tokenId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的令牌 ID"})
		return
	}
	// 权限校验：令牌所有者或管理员
	tok, err := model.GetTokenById(tokenId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "令牌不存在"})
		return
	}
	userId := c.GetInt("id")
	role := c.GetInt("role")
	if tok.UserId != userId && role < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问该令牌"})
		return
	}
	rows, err := model.GetAllTokenChannelQuotas(tokenId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// UpdateTokenChannelQuotas 全量覆盖令牌的分渠道额度配置。
func UpdateTokenChannelQuotas(c *gin.Context) {
	tokenId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的令牌 ID"})
		return
	}
	tok, err := model.GetTokenById(tokenId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "令牌不存在"})
		return
	}
	userId := c.GetInt("id")
	role := c.GetInt("role")
	if tok.UserId != userId && role < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权修改该令牌"})
		return
	}
	var req struct {
		Items []model.TokenChannelQuota `json:"items"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求体解析失败: " + err.Error()})
		return
	}
	if err := model.ReplaceTokenChannelQuotas(tokenId, req.Items); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
```

> `common`、`strconv`、`http` import 按需（token.go 已有）。

- [ ] **Step 4: 注册路由**

修改 `router/api-router.go` 的 `tokenRoute` 块（约 232-243 行），在现有路由后追加：

```go
		tokenRoute.GET("/:id/channel_quotas", controller.GetTokenChannelQuotas)
		tokenRoute.PUT("/:id/channel_quotas", controller.UpdateTokenChannelQuotas)
```

> 注意 Gin 路由冲突：`/:id` 与 `/:id/channel_quotas` 不冲突（后者有额外段）。`/search`、`/batch` 已是静态段，需确认 Gin 树允许混用——既有代码已用 `/:id` + `/search` 共存，说明项目使用兼容版本。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./controller/ -run TestGetTokenChannelQuotas -v`
Expected: PASS。

Run: `go build ./...`
Expected: 编译通过。

- [ ] **Step 6: 提交**

```bash
git add controller/token.go controller/token_channel_quota_handler_test.go router/api-router.go
git commit -m "feat(controller): 分渠道额度管理 API"
```

---

## Task A6：前端令牌编辑 UI（分渠道额度）

**Files:**
- Modify: `web/default/src/features/tokens/**`（令牌编辑表单/抽屉）
- Modify: `web/default/src/i18n/locales/*.json`
- 参考：`web/default/AGENTS.md`

- [ ] **Step 1: 定位令牌编辑组件**

Run: 查找令牌编辑表单（含 `reset_period`、`remain_quota` 字段的组件）。
`grep -r "reset_period\|resetQuota\|remain_quota" web/default/src/features/tokens/`
确定编辑抽屉/表单文件路径。

- [ ] **Step 2: 加分渠道开关与子表格**

在令牌编辑表单中：
- 新增 `<Checkbox>` 或 `<Switch>`「分渠道额度」（绑定 `channel_quota_mode`）。
- 开启时：隐藏令牌总额度相关输入（`remain_quota`、`unlimited_quota`），展示子表格。
- 子表格：渠道选择器（拉取 `/api/channel/` 列表，复用现有 `useChannels`）+ 每渠道额度输入 + 默认 `reset_quota = 输入额度`。
- 保存时调用 `PUT /api/token/:id/channel_quotas`（items 数组）。
- 周期 `reset_period` 沿用令牌已有的选择器（不隐藏）。

> 实现遵循 `web/default/AGENTS.md` 的组件结构、Tailwind 样式、i18n 规范。

- [ ] **Step 3: i18n 文案**

新增 key（英文为 key，各 locale 文件补翻译）：
- `"Per-channel quota"` / 分渠道额度
- `"Per-channel quota mode"` / 分渠道额度模式
- `"Channel quota configuration"` / 渠道额度配置
- `"Add channel quota"` / 添加渠道额度
- `"Channel"` / 渠道
- `"Quota"` / 额度
- `"Reset quota"` / 重置额度
- `"When enabled, the token uses per-channel quotas instead of a total quota. Requests to unconfigured channels are not limited."` / 开启后令牌使用分渠道额度而非总额度；请求未配置的渠道不限额。
- `"Save channel quotas"` / 保存渠道额度

运行 `cd web/default && bun run i18n:sync` 补齐缺失 locale。

- [ ] **Step 4: 构建校验**

Run: `cd web/default && bun run build`
Expected: 构建成功，无类型错误。

- [ ] **Step 5: 提交**

```bash
git add web/default/src/features/tokens/ web/default/src/i18n/locales/
git commit -m "feat(web): 令牌编辑分渠道额度 UI"
```

---

# 阶段 B：对话详情独立库

## Task B1：ChatLog 模型与独立库句柄

**Files:**
- Create: `model/chat_log.go`
- Modify: `model/main.go`（CHATLOG_DB 句柄、InitChatLogDB、migrateChatLogDB、SetChatLogDatabaseType）
- Modify: `main.go:305-330`（启动调用）
- Modify: `common/constants.go` 或 `common/database.go`（DatabaseType 判断 helper）—— 复用现有
- Test: `model/chat_log_test.go`

- [ ] **Step 1: 写失败测试**

新建 `model/chat_log_test.go`：

```go
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatLog_CreateAndQuery(t *testing.T) {
	// 需 CHATLOG_DB 已初始化（TestMain 或本测试 setup）
	if CHATLOG_DB == nil {
		t.Skip("CHATLOG_DB not configured")
	}
	cl := &ChatLog{
		TokenId: 1, UserId: 1, ChannelId: 5,
		ModelName: "gpt-4", RequestId: "req-1",
		RequestBody: `{"messages":[]}`, ResponseBody: `{"choices":[]}`,
		IsStream: true, StatusCode: 200, UseTime: 2,
	}
	require.NoError(t, cl.Insert())

	got, err := GetChatLogById(cl.Id)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", got.ModelName)
	assert.Equal(t, `{"messages":[]}`, got.RequestBody)

	list, total, err := SearchChatLogs(0, 0, 0, "", "", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, list, 1)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./model/ -run TestChatLog_CreateAndQuery -v`
Expected: 编译失败（`ChatLog`、`CHATLOG_DB`、`GetChatLogById` 未定义）。

- [ ] **Step 3: 实现模型与句柄**

新建 `model/chat_log.go`：

```go
package model

import (
	"github.com/QuantumNous/new-api/common"
)

// ChatLog 存储开启对话详情记录的令牌的完整请求/响应 JSON。
// 位于独立数据库（CHAT_LOG_SQL_DSN）；未配置时该表不存在、功能静默禁用。
type ChatLog struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	TokenId      int    `json:"token_id" gorm:"index"`
	UserId       int    `json:"user_id" gorm:"index"`
	ChannelId    int    `json:"channel_id" gorm:"index"`
	ModelName    string `json:"model_name" gorm:"type:varchar(128);index"`
	RequestId    string `json:"request_id" gorm:"type:varchar(64);index"`
	RequestBody  string `json:"request_body" gorm:"type:longtext"`
	ResponseBody string `json:"response_body" gorm:"type:longtext"`
	IsStream     bool   `json:"is_stream"`
	Truncated    bool   `json:"truncated"`
	StatusCode   int    `json:"status_code" gorm:"default:0"`
	UseTime      int    `json:"use_time" gorm:"default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
}

func (ChatLog) TableName() string {
	return "chat_logs"
}

// ChatLogEnabled 报告独立库是否已配置（功能是否可用）。
func ChatLogDBEnabled() bool {
	return CHATLOG_DB != nil
}

func (cl *ChatLog) Insert() error {
	cl.CreatedAt = common.GetTimestamp()
	return CHATLOG_DB.Create(cl).Error
}

func GetChatLogById(id int) (*ChatLog, error) {
	var cl ChatLog
	err := CHATLOG_DB.First(&cl, "id = ?", id).Error
	return &cl, err
}

// SearchChatLogs 分页查询。tokenId/userId/channelId 为 0 表示不过滤；modelName 空不过滤。
func SearchChatLogs(tokenId, userId, channelId int, modelName, requestId string, page, pageSize int) ([]*ChatLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := CHATLOG_DB.Model(&ChatLog{})
	if tokenId > 0 {
		q = q.Where("token_id = ?", tokenId)
	}
	if userId > 0 {
		q = q.Where("user_id = ?", userId)
	}
	if channelId > 0 {
		q = q.Where("channel_id = ?", channelId)
	}
	if modelName != "" {
		q = q.Where("model_name = ?", modelName)
	}
	if requestId != "" {
		q = q.Where("request_id = ?", requestId)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []*ChatLog
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
```

- [ ] **Step 4: 独立库句柄与初始化**

修改 `model/main.go`：

(a) 在 `LOG_DB` 声明附近新增句柄变量：

```go
var (
	DB       *gorm.DB
	LOG_DB   *gorm.DB
	CHATLOG_DB *gorm.DB
)
```

(b) 新增 `InitChatLogDB`（仿 `InitLogDB`，约 222 行之后）。`CHAT_LOG_SQL_DSN` 未设置时 `CHATLOG_DB = nil` 并 warning：

```go
func InitChatLogDB() (err error) {
	if os.Getenv("CHAT_LOG_SQL_DSN") == "" {
		// 功能未配置，静默禁用
		common.SysLog("CHAT_LOG_SQL_DSN not set, chat-log detail storage disabled")
		return nil
	}
	db, dbType, err := chooseDB("CHAT_LOG_SQL_DSN", false)
	if err != nil {
		common.FatalLog(err)
		return err
	}
	common.SysLog("using " + string(dbType) + " as chat-log detail database")
	if common.DebugEnabled {
		db = db.Debug()
	}
	CHATLOG_DB = db
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
	sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
	sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

	if !common.IsMasterNode {
		return nil
	}
	return migrateChatLogDB()
}
```

> `chooseDB` 第二参 `isLog` 仅影响 ClickHouse 校验；chat-log 库同样不应使用 ClickHouse（用 false 即可，或 true 允许；此处用 false 沿用主库校验）。

(c) 新增 `migrateChatLogDB`（方言感知的大文本列）：

```go
func migrateChatLogDB() error {
	if err := CHATLOG_DB.AutoMigrate(&ChatLog{}); err != nil {
		return err
	}
	// MySQL：将 request_body/response_body 显式升为 longtext（AutoMigrate 默认可能给 text/mediumtext）
	if common.UsingLogDatabase(common.DatabaseTypeMySQL) {
		// 注意：CHATLOG_DB 的方言需用专门判断。复用 chooseDB 返回的 dbType。
		// 简化：用 CHATLOG_DB.Migrator().HasColumn 之类判断后 ALTER。
		// 见下方说明：通过 migrateChatLogDB 内捕获的 dbType 判断。
	}
	return nil
}
```

> 方言判断：`migrateChatLogDB` 改为接收 `dbType common.DatabaseType` 参数（从 `InitChatLogDB` 传入），用它判断 MySQL 分支执行 `ALTER TABLE chat_logs MODIFY request_body LONGTEXT, MODIFY response_body LONGTEXT`。PostgreSQL/SQLite 的 `text` 无限长，无需处理。注意 GORM AutoMigrate 已建好表，此处仅做 MySQL 列类型升级。

修正后的 `migrateChatLogDB(dbType common.DatabaseType)`：

```go
func migrateChatLogDB(dbType common.DatabaseType) error {
	if err := CHATLOG_DB.AutoMigrate(&ChatLog{}); err != nil {
		return err
	}
	if dbType == common.DatabaseTypeMySQL {
		if err := CHATLOG_DB.Exec("ALTER TABLE chat_logs MODIFY COLUMN request_body LONGTEXT").Error; err != nil {
			return err
		}
		if err := CHATLOG_DB.Exec("ALTER TABLE chat_logs MODIFY COLUMN response_body LONGTEXT").Error; err != nil {
			return err
		}
	}
	return nil
}
```

并相应修改 `InitChatLogDB` 调用：`return migrateChatLogDB(dbType)`。

(d) main.go 启动调用：修改 `main.go` 在 `model.InitLogDB()`（约 327 行）之后：

```go
	err = model.InitLogDB()
	// ... 既有错误处理 ...
	err = model.InitChatLogDB()
	if err != nil {
		common.FatalLog(err)
	}
```

- [ ] **Step 5: Token 加字段**

修改 `model/token.go` 的 `Token` 结构体，在 `ChannelQuotaMode` 之后加：

```go
	ChatLogEnabled bool           `json:"chat_log_enabled" gorm:"default:0"`
```

`Update()` Select 列表加 `"chat_log_enabled"`。

- [ ] **Step 6: 运行确认通过**

Run: `CHAT_LOG_SQL_DSN= go test ./model/ -run TestChatLog_CreateAndQuery -v`（不设 DSN 时应 skip）。
为真正测落库，临时设环境变量指向内存 SQLite：
Run: `$env:CHAT_LOG_SQL_DSN=""; go test ./model/ -run TestChatLog_CreateAndQuery -v`（Windows pwsh）
> 内存 SQLite 需 DSN 形如 `file::memory:?cache=shared` 或临时文件。测试 fixture 可在 setupTestDB 中设置。若复杂，改用临时文件路径。

Expected: CHATLOG_DB 初始化后测试 PASS。

- [ ] **Step 7: 提交**

```bash
git add model/chat_log.go model/chat_log_test.go model/main.go model/token.go main.go
git commit -m "feat(model): ChatLog 独立库句柄与 Token.ChatLogEnabled"
```

---

## Task B2：响应采集 ResponseWriter 包装器

**Files:**
- Create: `service/chat_log_capture.go`
- Test: `service/chat_log_capture_test.go`

- [ ] **Step 1: 写失败测试**

新建 `service/chat_log_capture_test.go`：

```go
package service

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapturingResponseWriter_BuffersBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	orig := c.Writer

	cap := wrapWithChatLogCapture(c, orig, 1024)
	_, _ = cap.Write([]byte(`{"hello":"world"}`))

	assert.Equal(t, `{"hello":"world"}`, string(cap.(*chatLogCaptureWriter).buffer.Bytes()))
	// 原始 writer 也收到数据
	assert.Equal(t, `{"hello":"world"}`, w.Body.String())
}

func TestCapturingResponseWriter_Truncates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	cap := wrapWithChatLogCapture(c, w, 4)
	_, _ = cap.Write([]byte(`1234567890`))

	cw := cap.(*chatLogCaptureWriter)
	assert.Equal(t, `1234`, string(cw.buffer.Bytes()))
	require.True(t, cw.truncated)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./service/ -run TestCapturingResponseWriter -v`
Expected: 编译失败。

- [ ] **Step 3: 实现 wrapper**

新建 `service/chat_log_capture.go`：

```go
package service

import (
	"bytes"
	"sync"

	"github.com/gin-gonic/gin"
)

// chatLogCaptureWriter 包装 gin.ResponseWriter，缓冲写出字节用于落库对话详情。
// 超过 maxBytes 后停止缓冲并标记 truncated；写入仍透传给底层 writer。
type chatLogCaptureWriter struct {
	gin.ResponseWriter
	buffer    bytes.Buffer
	mu        sync.Mutex
	maxBytes  int
	truncated bool
}

// wrapWithChatLogCapture 返回包装后的 writer。maxBytes<=0 时不缓冲。
func wrapWithChatLogCapture(c *gin.Context, original gin.ResponseWriter, maxBytes int) gin.ResponseWriter {
	if maxBytes <= 0 {
		return original
	}
	w := &chatLogCaptureWriter{
		ResponseWriter: original,
		maxBytes:       maxBytes,
	}
	c.Writer = w
	return w
}

func (w *chatLogCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data) // 先透传，保证客户端不受影响
	if err != nil {
		return n, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.truncated {
		return n, nil
	}
	remaining := w.maxBytes - w.buffer.Len()
	if remaining <= 0 {
		w.truncated = true
		return n, nil
	}
	if len(data) <= remaining {
		w.buffer.Write(data)
	} else {
		w.buffer.Write(data[:remaining])
		w.truncated = true
	}
	return n, nil
}

// capturedBytes 返回缓冲内容副本与是否截断。
func (w *chatLogCaptureWriter) capturedBytes() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String(), w.truncated
}

// Must also support http.Hijack / http.Flusher passthrough for streaming.
// gin.ResponseWriter 已实现 Flusher/Hijacker；嵌入后透传，无需重写。
```

> 流式响应依赖 `gin.ResponseWriter` 的 `Flush()`；嵌入后自动透传（嵌入类型保留其方法）。`Hijack`（WebSocket）场景：WebSocket 帧不走 `Write`，故对话详情对 realtime/ws 协议不采集响应体——符合预期（仅采集 HTTP 文本协议）。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./service/ -run TestCapturingResponseWriter -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add service/chat_log_capture.go service/chat_log_capture_test.go
git commit -m "feat(service): 对话详情响应采集 ResponseWriter 包装器"
```

---

## Task B3：中继入口安装采集器 + 异步落库

**Files:**
- Modify: `controller/relay.go:68-249`（Relay 函数）
- Possibly Modify: `setting/operation_setting` 新增 `chat_log_max_body_bytes` 选项（默认 262144）

- [ ] **Step 1: 写失败测试**

新建 `controller/relay_chatlog_test.go`：测试当 `token.ChatLogEnabled && CHATLOG_DB != nil` 时，请求结束后 `chat_logs` 表新增一行。可用集成测试风格（mock relay handler 直接写响应），或测试一个抽取出的 `maybeInstallChatLogCapture` 函数。

```go
package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaybeInstallChatLogCapture(t *testing.T) {
	if !model.ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	tok := &model.Token{UserId: 1, Key: "sk-chatlog", Status: 1, ChatLogEnabled: true}
	require.NoError(t, tok.Insert())

	r.Use(func(c *gin.Context) {
		c.Set("token_id", tok.Id)
		c.Set("id", tok.UserId)
		c.Set("channel_id", 7)
		c.Set("original_model", "gpt-4")
		c.Set(common.RequestIdKey, "req-chatlog")
		c.Next()
	})
	r.GET("/", func(c *gin.Context) {
		// 假定采集器已安装
		_, _ = c.Writer.Write([]byte(`{"ok":true}`))
	})

	// 直接调用采集编排函数（从 Relay 抽取）
	service.maybeInstallChatLogCapture(c, true)
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	// 异步落库，用 require.Eventually 轮询
	require.Eventually(t, func() bool {
		logs, total, _ := model.SearchChatLogs(tok.Id, 0, 0, "", "", 1, 10)
		return total == 1 && len(logs) == 1 && logs[0].ResponseBody == `{"ok":true}`
	}, 2*time.Second, 50*time.Millisecond)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./controller/ -run TestMaybeInstallChatLogCapture -v`
Expected: 编译失败（`installChatLogCaptureIfNeeded` 未定义）。

- [ ] **Step 3: 实现采集编排**

新建 `service/chat_log_persist.go`：

```go
package service

import (
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const defaultChatLogMaxBodyBytes = 262144 // 256KB

// chatLogMaxBodyBytes 返回配置的上限（从 setting 读取，默认 256KB）。
func chatLogMaxBodyBytes() int {
	// 一期：固定默认值。后续可接 setting/operation_setting 配置项。
	return defaultChatLogMaxBodyBytes
}

// maybeInstallChatLogCapture 在令牌开启对话详情且独立库已配置时，安装响应采集 writer。
// 返回安装后的 writer（用于后续读取缓冲）与是否安装。
func maybeInstallChatLogCapture(c *gin.Context, chatLogEnabled bool) (*chatLogCaptureWriter, bool) {
	if !chatLogEnabled || !model.ChatLogDBEnabled() {
		return nil, false
	}
	maxBytes := chatLogMaxBodyBytes()
	wrapped := wrapWithChatLogCapture(c, c.Writer, maxBytes)
	cw, ok := wrapped.(*chatLogCaptureWriter)
	return cw, ok
}

// persistChatLog 异步落库一条对话详情。requestBody 由调用方从 BodyStorage 读取。
func persistChatLog(c *gin.Context, cw *chatLogCaptureWriter, requestBody string) {
	if cw == nil {
		return
	}
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	channelId := c.GetInt("channel_id")
	modelName := c.GetString("original_model")
	requestId := c.GetString(common.RequestIdKey)
	isStream := c.GetBool("is_stream")

	cw2 := cw
	gopool.Go(func() {
		respBody, truncated := cw2.capturedBytes()
		useTime := 0
		cl := &model.ChatLog{
			TokenId: tokenId, UserId: userId, ChannelId: channelId,
			ModelName: modelName, RequestId: requestId,
			RequestBody: truncateStr(requestBody, defaultChatLogMaxBodyBytes),
			ResponseBody: respBody,
			IsStream: isStream, Truncated: truncated,
			StatusCode: cw2.ResponseWriter.Status(),
			UseTime: useTime,
		}
		if err := cl.Insert(); err != nil {
			common.SysError("failed to insert chat log: " + err.Error())
		}
	})
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// 防止 io 未用（如需）
var _ = io.EOF
```

> 注意 import 冲突：`relay/common` 与 `common` 包名相同。项目惯例用 `relaycommon` 别名（见 `relay/helper/stream_result.go:4`）。此处 `relay/common.RelayInfo` 不需要，只用到 `common.RequestIdKey` 与 `model`、`gin`。`common` 即 `github.com/QuantumNous/new-api/common`。修正：去掉 `relay/common` import，仅用顶层 `common`。

在 `controller/relay.go` 的 `Relay` 函数中接入。在 `relayInfo` 生成后、`relayHandler` 调用前安装；在函数末尾（成功 return 前）落库。

修改 `controller/relay.go:68` 的 `Relay`：

```go
func Relay(c *gin.Context, relayFormat types.RelayFormat) {
	requestId := c.GetString(common.RequestIdKey)
	// ... 既有 ...

	// 对话详情采集（条件式安装）
	var chatLogWriter *chatLogCaptureWriter
	// 需要读取令牌的 ChatLogEnabled：relayInfo 在下方生成；改在 relayInfo 生成后安装。
	// 见下方 GenRelayInfo 之后插入。
```

在 `relayInfo` 生成成功后（约 124 行 `info.InitRequestConversionChain()` 返回后）插入：

```go
	// 安装对话详情采集（令牌开启且独立库已配置）
	if token, terr := model.GetTokenByKey(relayInfo.TokenKey, false); terr == nil && token.ChatLogEnabled {
		chatLogWriter, _ = maybeInstallChatLogCapture(c, true)
	}
```

> 简化：直接 `chatLogWriter, _ = maybeInstallChatLogCapture(c, token.ChatLogEnabled)`，函数内部再判 ChatLogDBEnabled。

在请求体读取处（约 201 行 `bodyStorage, bodyErr := common.GetBodyStorage(c)` 成功后），缓存请求体字节供落库：

```go
	var chatLogRequestBody string
	if chatLogWriter != nil && bodyStorage != nil {
		if bs, _ := bodyStorage.Bytes(); bs != nil {
			chatLogRequestBody = string(bs)
		}
	}
```

在 `Relay` 函数成功 return 前（约 226 行 `return` 之前）：

```go
	if chatLogWriter != nil {
		persistChatLog(c, chatLogWriter, chatLogRequestBody)
	}
```

失败路径（newAPIError != nil 的 return）：可不落库对话详情（失败请求意义不大），或也落库标记 StatusCode。一期：仅在成功路径落库。

- [ ] **Step 4: 运行确认通过**

Run: `go test ./controller/ -run TestMaybeInstallChatLogCapture -v`（需 CHATLOG_DB 配置，否则 skip）
Expected: 配置后 PASS。

Run: `go build ./...`
Expected: 编译通过。

- [ ] **Step 5: 提交**

```bash
git add service/chat_log_persist.go controller/relay.go controller/relay_chatlog_test.go
git commit -m "feat(controller): 中继入口条件式安装对话详情采集与异步落库"
```

---

## Task B4：管理员查询 API

**Files:**
- Create: `controller/chat_log.go`
- Create: `router/chat-log-router.go`
- Modify: `router/main.go:16`（注册）

- [ ] **Step 1: 写失败测试**

新建 `controller/chat_log_handler_test.go`：

```go
package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminGetChatLogs_ListAndDetail(t *testing.T) {
	if !model.ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	cl := &model.ChatLog{
		TokenId: 1, UserId: 1, ChannelId: 5, ModelName: "gpt-4",
		RequestId: "req-x", RequestBody: `{"q":1}`, ResponseBody: `{"a":2}`,
	}
	require.NoError(t, cl.Insert())

	// 列表
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.Use(func(c *gin.Context) { c.Set("role", 100); c.Next() })
	r.GET("/api/chat_logs", controller.AdminGetChatLogs)
	req := httptest.NewRequest(http.MethodGet, "/api/chat_logs?page=1&page_size=10", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Data []*model.ChatLog `json:"data"`
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Equal(t, int64(1), listResp.Total)

	// 详情
	w2 := httptest.NewRecorder()
	_, r2 := gin.CreateTestContext(w2)
	r2.Use(func(c *gin.Context) { c.Set("role", 100); c.Next() })
	r2.GET("/api/chat_logs/:id", controller.AdminGetChatLogDetail)
	req2 := httptest.NewRequest(http.MethodGet, "/api/chat_logs/"+strconv.Itoa(cl.Id), nil)
	r2.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	var detail struct {
		Data *model.ChatLog `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &detail))
	assert.Equal(t, `{"q":1}`, detail.Data.RequestBody)
}

func TestAdminGetChatLogs_ForbiddenForNonAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.Use(func(c *gin.Context) { c.Set("role", 1); c.Next() }) // 普通用户
	r.GET("/api/chat_logs", controller.AdminGetChatLogs)
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat_logs", nil))
	assert.Equal(t, http.StatusForbidden, w.Code)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./controller/ -run TestAdminGetChatLogs -v`
Expected: 编译失败。

- [ ] **Step 3: 实现 handler**

新建 `controller/chat_log.go`：

```go
package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func requireAdmin(c *gin.Context) bool {
	role := c.GetInt("role")
	if role < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问"})
		return false
	}
	return true
}

// AdminGetChatLogs 列表（不含 body，仅元数据）。
func AdminGetChatLogs(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	if !model.ChatLogDBEnabled() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "对话详情库未配置"})
		return
	}
	tokenId, _ := strconv.Atoi(c.Query("token_id"))
	userId, _ := strconv.Atoi(c.Query("user_id"))
	channelId, _ := strconv.Atoi(c.Query("channel_id"))
	modelName := c.Query("model_name")
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	logs, total, err := model.SearchChatLogs(tokenId, userId, channelId, modelName, "", page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	// 列表隐藏 body 字段
	type meta struct {
		Id        int    `json:"id"`
		TokenId   int    `json:"token_id"`
		UserId    int    `json:"user_id"`
		ChannelId int    `json:"channel_id"`
		ModelName string `json:"model_name"`
		RequestId string `json:"request_id"`
		IsStream  bool   `json:"is_stream"`
		Truncated bool   `json:"truncated"`
		StatusCode int   `json:"status_code"`
		UseTime   int    `json:"use_time"`
		CreatedAt int64  `json:"created_at"`
	}
	out := make([]meta, 0, len(logs))
	for _, l := range logs {
		out = append(out, meta{
			Id: l.Id, TokenId: l.TokenId, UserId: l.UserId, ChannelId: l.ChannelId,
			ModelName: l.ModelName, RequestId: l.RequestId, IsStream: l.IsStream,
			Truncated: l.Truncated, StatusCode: l.StatusCode, UseTime: l.UseTime, CreatedAt: l.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out, "total": total})
}

// AdminGetChatLogDetail 单条详情（含 request/response body）。
func AdminGetChatLogDetail(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	if !model.ChatLogDBEnabled() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "对话详情库未配置"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效 ID"})
		return
	}
	cl, err := model.GetChatLogById(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "记录不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cl})
}
```

- [ ] **Step 4: 注册路由**

新建 `router/chat-log-router.go`：

```go
package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerChatLogRoutes(apiRouter *gin.RouterGroup) {
	chatLogRoute := apiRouter.Group("/chat_logs")
	chatLogRoute.Use(middleware.AdminAuth())
	chatLogRoute.GET("/", controller.AdminGetChatLogs)
	chatLogRoute.GET("/:id", controller.AdminGetChatLogDetail)
}
```

修改 `router/main.go` 的 `SetRouter`（约 16 行附近，与其他 register 调用并列）：

```go
	registerChatLogRoutes(apiRouter)
```

> 确认 `router/main.go` 暴露的是 `apiRouter`；若 register 函数签名需要传入，参考 `registerChannelRoutes(apiRouter)` 模式。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./controller/ -run TestAdminGetChatLogs -v`
Expected: PASS（配置 CHATLOG_DB 时；否则 skip）。

Run: `go build ./...`
Expected: 编译通过。

- [ ] **Step 6: 提交**

```bash
git add controller/chat_log.go controller/chat_log_handler_test.go router/chat-log-router.go router/main.go
git commit -m "feat(controller): 对话详情管理员查询 API"
```

---

## Task B5：前端管理员对话详情页

**Files:**
- Modify/Create: `web/default/src/features/admin/**`（新增 chat-logs 页面）
- Modify: 路由注册（`web/default/src/routeTree.gen.ts` 自动生成，新增 route 文件即可）
- Modify: `web/default/src/i18n/locales/*.json`

- [ ] **Step 1: 定位管理员页面与路由模式**

Run: 查找现有管理员页面（如 channels、logs）的结构。
`grep -r "admin" web/default/src/routes/` 或查看 `web/default/src/features/` 目录。

确定 route 文件命名约定（如 `web/default/src/routes/_authenticated/admin/`）。

- [ ] **Step 2: 新增对话详情页**

- API 客户端：`web/default/src/features/admin/chat-logs/api.ts`，封装 `GET /api/chat_logs` 与 `GET /api/chat_logs/:id`。
- 列表页：表格（时间、令牌 ID、用户、渠道、模型、流式、截断标记、状态码），筛选（token_id、model_name）+ 分页，复用现有 `useQuery` / 表格组件。
- 详情抽屉：点击行打开，展示 request_body / response_body（JSON 折叠/展开，可用 `<pre>` + 简单 JSON 格式化）。
- 路由：新增 `chat-logs` route 文件，挂在管理员区域。

> 遵循 `web/default/AGENTS.md`、`shadcn-ui` 组件规范。权限：仅管理员可见入口（沿用现有 admin 布局守卫）。

- [ ] **Step 3: i18n 文案**

新增 key：
- `"Chat Logs"` / 对话详情
- `"Request body"` / 请求体
- `"Response body"` / 响应体
- `"Truncated"` / 已截断
- `"Conversation detail is only available when the token has chat-log enabled and the standalone database is configured."` / 仅当令牌开启对话详情记录且独立库已配置时可用。
- `"View detail"` / 查看详情

`cd web/default && bun run i18n:sync`。

- [ ] **Step 4: 构建校验**

Run: `cd web/default && bun run build`
Expected: 构建成功。

- [ ] **Step 5: 提交**

```bash
git add web/default/src/features/admin/chat-logs/ web/default/src/routes/ web/default/src/i18n/locales/
git commit -m "feat(web): 管理员对话详情页"
```

---

## Task C1：全量构建与回归

- [ ] **Step 1: 后端编译 + vet**

Run: `go build ./... && go vet ./...`
Expected: 通过。

- [ ] **Step 2: 后端全量测试**

Run: `go test ./model/ ./service/ ./controller/ -v`
Expected: 全部 PASS（CHAT_LOG_SQL_DSN 未设时相关测试 skip）。

- [ ] **Step 3: 前端构建**

Run: `cd web/default && bun run build`
Expected: 构建成功。

- [ ] **Step 4: 数据库兼容性手动验证**

分别在 SQLite / MySQL / PostgreSQL 下：
- 启动确认 `token_channel_quotas`、`chat_logs` 表创建成功。
- MySQL 下确认 `chat_logs.request_body`/`response_body` 为 `longtext`。
- 令牌开启分渠道额度 → 请求命中已配置渠道扣减正确；命中未配置渠道不限额。
- 令牌开启对话详情（配置 CHAT_LOG_SQL_DSN）→ `chat_logs` 新增记录。

- [ ] **Step 5: 最终提交（如有）**

```bash
git add -A
git commit -m "test: 全量构建与回归通过" --allow-empty
```

---

## 备注：实现顺序与依赖

- Task A1 → A2 → A3 → A4 → A5 → A6（功能一后端到前端，顺序依赖）。
- Task B1 → B2 → B3 → B4 → B5（功能二后端到前端）。
- A 与 B 相互独立，可并行。
- Task C1 收尾。

## 备注：跨库 JSON 规范

所有新代码的 marshal/unmarshal 必须用 `common.Marshal` / `common.Unmarshal` / `common.DecodeJson`，不得直接 import `encoding/json`（项目规范）。本计划中测试用 `encoding/json` 仅用于断言解析响应体，可改用 `common.Unmarshal` 以保持一致——实现时统一用 `common.*`。

## 备注：计费安全不变式

- `DecreaseTokenChannelQuota` 入口拒绝负数。
- 渠道额度由管理员整数配置，无浮点→整数转换路径，无需 `common.QuotaFromFloat`。
- 分渠道模式下信任旁路禁用，保证精确记账。
