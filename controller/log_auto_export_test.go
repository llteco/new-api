package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
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

	for _, byHour := range data.Details {
		for _, byModel := range byHour {
			for modelName, detail := range byModel {
				switch modelName {
				case "gpt-4":
					assert.EqualValues(t, 100, detail.Prompt)
					assert.EqualValues(t, 50, detail.Completion)
					assert.EqualValues(t, 0, detail.Cache)
				case "claude-3-5-sonnet":
					assert.EqualValues(t, 100, detail.Prompt)
					assert.EqualValues(t, 50, detail.Completion)
					assert.EqualValues(t, 50, detail.Cache)
				}
			}
		}
	}
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
	for _, byModel := range data.Details["alice"] {
		detail := byModel["claude-3-sonnet"]
		assert.EqualValues(t, 10, detail.Prompt)
		assert.EqualValues(t, 5, detail.Completion)
		assert.EqualValues(t, 55, detail.Cache) // 5 + 50
		break
	}
}
