package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectivePromptTokensExprRespectsDatabaseDialects(t *testing.T) {
	tests := []struct {
		name     string
		dbType   common.DatabaseType
		contains []string
	}{
		{
			name:   "sqlite",
			dbType: common.DatabaseTypeSQLite,
			contains: []string{
				"json_extract(logs.other, '$.usage_semantic') = 'anthropic'",
				"json_extract(logs.other, '$.claude') = 1",
				"json_extract(logs.other, '$.cache_tokens')",
				"MAX(",
			},
		},
		{
			name:   "mysql",
			dbType: common.DatabaseTypeMySQL,
			contains: []string{
				"JSON_EXTRACT(logs.other, '$.usage_semantic')",
				"JSON_EXTRACT(logs.other, '$.claude') = true",
				"JSON_EXTRACT(logs.other, '$.cache_tokens')",
				"GREATEST(",
			},
		},
		{
			name:   "postgres",
			dbType: common.DatabaseTypePostgreSQL,
			contains: []string{
				"jsonb_typeof(logs.other::jsonb->'usage_semantic')",
				"logs.other::jsonb->>'usage_semantic' = 'anthropic'",
				"jsonb_typeof(logs.other::jsonb->'claude')",
				"(logs.other::jsonb->>'claude')::boolean = true",
				"logs.other::jsonb->>'cache_tokens')::int",
				"GREATEST(",
			},
		},
		{
			name:   "clickhouse",
			dbType: common.DatabaseTypeClickHouse,
			contains: []string{
				"JSONExtractString(logs.other, 'usage_semantic') = 'anthropic'",
				"JSONExtractBool(logs.other, 'claude') = true",
				"JSONExtractInt(logs.other, 'cache_tokens')",
				"greatest(",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := common.LogDatabaseType()
			t.Cleanup(func() {
				common.SetLogDatabaseType(original)
			})
			common.SetLogDatabaseType(tt.dbType)

			expr := effectivePromptTokensExpr()
			for _, want := range tt.contains {
				assert.Contains(t, expr, want)
			}
			assert.Contains(t, expr, "prompt_tokens")
			assert.Contains(t, expr, "cache_creation_tokens")
			assert.Contains(t, expr, "cache_creation_tokens_5m")
			assert.Contains(t, expr, "cache_creation_tokens_1h")
		})
	}
}

func TestEffectiveTotalTokensExprIncludesCompletion(t *testing.T) {
	original := common.LogDatabaseType()
	t.Cleanup(func() {
		common.SetLogDatabaseType(original)
	})
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)

	expr := effectiveTotalTokensExpr()
	assert.Contains(t, expr, effectivePromptTokensExpr())
	assert.Contains(t, expr, "completion_tokens")
}

func TestSumTokensByDimensionIncludesAnthropicCacheInPromptTokens(t *testing.T) {
	require.NotNil(t, LOG_DB, "LOG_DB must be initialized by TestMain")
	truncateTables(t)

	now := time.Now().Unix()
	baseLog := Log{
		Username:         "user1",
		CreatedAt:        now,
		Type:             LogTypeConsume,
		ModelName:        "claude-3-7-sonnet",
		PromptTokens:     100,
		CompletionTokens: 20,
		Quota:            1,
	}

	logs := []Log{
		// Anthropic-native row: prompt_tokens is input_tokens only, cache is stored in other.
		func() Log {
			l := baseLog
			l.Other = common.MapToJsonStr(map[string]interface{}{
				"usage_semantic": "anthropic",
				"cache_tokens":   50,
				"cache_creation_tokens": 10,
			})
			return l
		}(),
		// OpenAI-compatible row: prompt_tokens already includes cache.
		func() Log {
			l := baseLog
			l.ModelName = "gpt-4o"
			l.PromptTokens = 200
			l.CompletionTokens = 30
			l.Other = common.MapToJsonStr(map[string]interface{}{
				"cache_tokens": 40,
			})
			return l
		}(),
		// Legacy anthropic row without usage_semantic, only claude flag.
		func() Log {
			l := baseLog
			l.ModelName = "claude-3-5-sonnet"
			l.PromptTokens = 80
			l.CompletionTokens = 15
			l.Other = common.MapToJsonStr(map[string]interface{}{
				"claude":       true,
				"cache_tokens": 20,
			})
			return l
		}(),
	}

	for i := range logs {
		require.NoError(t, LOG_DB.Create(&logs[i]).Error)
	}

	rows, err := SumTokensByDimension(TokenStatDimensionModel, TokenStatFilter{
		StartTimestamp: now - 1,
		EndTimestamp:   now + 1,
	}, 0)
	require.NoError(t, err)

	byName := make(map[string]*TokenDimensionStat)
	for _, row := range rows {
		byName[row.Name] = row
	}

	require.Len(t, byName, 3)

	// Anthropic: prompt_tokens should be input + cache_read + cache_creation.
	assert.Equal(t, 100+50+10, byName["claude-3-7-sonnet"].PromptTokens)
	assert.Equal(t, 100+50+10+20, byName["claude-3-7-sonnet"].TotalTokens)

	// OpenAI: prompt_tokens already contains cache, should not double-count.
	assert.Equal(t, 200, byName["gpt-4o"].PromptTokens)
	assert.Equal(t, 200+30, byName["gpt-4o"].TotalTokens)

	// Legacy anthropic via claude flag.
	assert.Equal(t, 80+20, byName["claude-3-5-sonnet"].PromptTokens)
	assert.Equal(t, 80+20+15, byName["claude-3-5-sonnet"].TotalTokens)
}
