package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildLogExportData_TokenCounts(t *testing.T) {
	logs := []*model.Log{
		{
			Type:             model.LogTypeConsume,
			UserId:           1,
			Username:         "alice",
			ModelName:        "gpt-4",
			PromptTokens:     100,
			CompletionTokens: 50,
			Other:            "",
		},
		{
			Type:             model.LogTypeConsume,
			UserId:           2,
			Username:         "bob",
			ModelName:        "claude-3-5-sonnet",
			PromptTokens:     100,
			CompletionTokens: 50,
			Other:            `{"usage_semantic":"anthropic","cache_tokens":30,"cache_creation_tokens":20}`,
		},
	}

	data := buildLogExportData(logs)

	assert.EqualValues(t, 350, data.Tokens) // 150 (OpenAI) + 200 (Anthropic)
	assert.EqualValues(t, 150, data.Users["alice"]["gpt-4"])
	assert.EqualValues(t, 200, data.Users["bob"]["claude-3-5-sonnet"])

	// Details holds one normalized record per log row, in input order.
	require.Len(t, data.Details, 2)

	alice := data.Details[0]
	assert.Equal(t, "alice", alice.Username)
	assert.Equal(t, "gpt-4", alice.Model)
	assert.EqualValues(t, 100, alice.Prompt)
	assert.EqualValues(t, 50, alice.Completion)
	assert.EqualValues(t, 0, alice.Cache)
	assert.Empty(t, alice.Types)

	bob := data.Details[1]
	assert.Equal(t, "bob", bob.Username)
	assert.Equal(t, "claude-3-5-sonnet", bob.Model)
	assert.EqualValues(t, 100, bob.Prompt)
	assert.EqualValues(t, 50, bob.Completion)
	assert.EqualValues(t, 50, bob.Cache)
}

func TestBuildLogExportData_CacheCreationSplit(t *testing.T) {
	log := &model.Log{
		Type:             model.LogTypeConsume,
		UserId:           1,
		Username:         "alice",
		ModelName:        "claude-3-sonnet",
		PromptTokens:     10,
		CompletionTokens: 5,
		Other:            `{"usage_semantic":"anthropic","cache_tokens":5,"cache_creation_tokens_5m":20,"cache_creation_tokens_1h":30}`,
	}

	data := buildLogExportData([]*model.Log{log})

	assert.EqualValues(t, 70, data.Tokens) // 10 + 5 + 50 + 5
	require.Len(t, data.Details, 1)
	record := data.Details[0]
	assert.EqualValues(t, 10, record.Prompt)
	assert.EqualValues(t, 5, record.Completion)
	assert.EqualValues(t, 55, record.Cache) // 5 + 50
}

// TestBuildLogExportData_NormalizesCacheBySemantic pins the per-kind cache
// normalization: openai-native and converted-to-openai rows carry the cache
// inside prompt_tokens so the record prompt must subtract it back out;
// anthropic-native and converted-to-anthropic rows already exclude cache from
// prompt_tokens. The tokens/users sums must stay dashboard-aligned either way
// (cache counted exactly once).
func TestBuildLogExportData_NormalizesCacheBySemantic(t *testing.T) {
	cases := []struct {
		name       string
		other      string
		wantTokens int64
		wantRecord logExportRecord
	}{
		{
			name:       "openai native with cache",
			other:      `{"cache_tokens":30}`,
			wantTokens: 150, // 100 + 50, cache already inside prompt
			wantRecord: logExportRecord{Username: "alice", Model: "gpt-4", Prompt: 70, Completion: 50, Cache: 30},
		},
		{
			name:       "converted to openai",
			other:      `{"cache_tokens":30,"request_conversion":["Claude Messages","OpenAI Compatible"]}`,
			wantTokens: 150,
			wantRecord: logExportRecord{Username: "alice", Model: "gpt-4", Prompt: 70, Completion: 50, Cache: 30, Types: []string{"Claude Messages → OpenAI Compatible"}},
		},
		{
			name:       "converted to anthropic",
			other:      `{"usage_semantic":"anthropic","cache_tokens":30,"request_conversion":["OpenAI Compatible","Claude Messages"]}`,
			wantTokens: 180, // 100 + 30 + 50
			wantRecord: logExportRecord{Username: "alice", Model: "gpt-4", Prompt: 100, Completion: 50, Cache: 30, Types: []string{"OpenAI Compatible → Claude Messages"}},
		},
		{
			name:       "openai cache exceeding prompt clamps record prompt",
			other:      `{"cache_tokens":120}`,
			wantTokens: 150, // corrupt row: total stays prompt + completion
			wantRecord: logExportRecord{Username: "alice", Model: "gpt-4", Prompt: 0, Completion: 50, Cache: 120},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := &model.Log{
				Type:             model.LogTypeConsume,
				UserId:           1,
				Username:         "alice",
				ModelName:        "gpt-4",
				PromptTokens:     100,
				CompletionTokens: 50,
				Other:            tc.other,
			}

			data := buildLogExportData([]*model.Log{log})

			require.EqualValues(t, tc.wantTokens, data.Tokens)
			require.EqualValues(t, tc.wantTokens, data.Users["alice"]["gpt-4"])
			require.Len(t, data.Details, 1)
			record := data.Details[0]
			tc.wantRecord.Time = record.Time // timestamp formatting is covered separately
			assert.Equal(t, tc.wantRecord, record)
		})
	}
}

func TestBuildLogExportData_RecordTimeAndOrder(t *testing.T) {
	logs := []*model.Log{
		{Type: model.LogTypeConsume, UserId: 1, Username: "alice", ModelName: "gpt-4", CreatedAt: 1751558400},
		{Type: model.LogTypeConsume, UserId: 1, Username: "alice", ModelName: "gpt-4", CreatedAt: 1751562000},
	}

	data := buildLogExportData(logs)

	// Records keep the input order (GetLogsForExport sorts by created_at) and
	// render per-record UTC timestamps.
	require.Len(t, data.Details, 2)
	assert.Equal(t, "2025-07-03T16:00:00Z", data.Details[0].Time)
	assert.Equal(t, "2025-07-03T17:00:00Z", data.Details[1].Time)
}
