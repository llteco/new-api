package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractChatLogChain_OpenAI(t *testing.T) {
	c := extractChatLogChain([]byte(`{"model":"gpt","messages":[{"role":"user","content":"hi"}]}`))
	require.NotNil(t, c)
	require.Nil(t, c.SystemRaw)
	require.Len(t, c.Elements, 1)
	require.Len(t, c.Messages, 1)
}

func TestExtractChatLogChain_ClaudeSystemString(t *testing.T) {
	c := extractChatLogChain([]byte(`{"system":"be brief","messages":[{"role":"user","content":"hi"}]}`))
	require.NotNil(t, c)
	require.JSONEq(t, `"be brief"`, string(c.SystemRaw))
	require.Len(t, c.Elements, 2) // system + 1 message
	require.Len(t, c.Messages, 1) // messages without the system element
}

func TestExtractChatLogChain_ClaudeSystemArray(t *testing.T) {
	c := extractChatLogChain([]byte(`{"system":[{"type":"text","text":"a"}],"messages":[{"role":"user","content":"hi"}]}`))
	require.Len(t, c.Elements, 2)
}

func TestExtractChatLogChain_Gemini(t *testing.T) {
	c := extractChatLogChain([]byte(`{"systemInstruction":{"role":"user","parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	require.NotNil(t, c)
	require.Len(t, c.Elements, 2)
}

func TestExtractChatLogChain_UnknownShape(t *testing.T) {
	require.Nil(t, extractChatLogChain([]byte(`{"input": "x"}`)))
	require.Nil(t, extractChatLogChain([]byte(`not json`)))
}

func TestComputeChatLogChainHashes_PrefixProperty(t *testing.T) {
	elems := []json.RawMessage{[]byte(`{"role":"user","content":"a"}`), []byte(`{"role":"assistant","content":"b"}`)}
	h2 := computeChatLogChainHashes(7, "claude", elems)
	elems = append(elems, json.RawMessage(`{"role":"user","content":"c"}`))
	h3 := computeChatLogChainHashes(7, "claude", elems)
	require.Len(t, h2, 2)
	require.Len(t, h3, 3)
	assert.Equal(t, h2, h3[:2], "extending the conversation must not change earlier prefix hashes")
	assert.NotEqual(t, computeChatLogChainHashes(8, "claude", elems[:1])[0], h3[0], "seed differs by token")
}

func TestComputeChatLogChainHashes_Deterministic(t *testing.T) {
	elems := []json.RawMessage{[]byte(`{"role":"user","content":"a"}`), []byte(`{"role":"assistant","content":"b"}`)}
	assert.Equal(t, computeChatLogChainHashes(7, "claude", elems), computeChatLogChainHashes(7, "claude", elems))
	assert.NotEqual(t, computeChatLogChainHashes(7, "openai", elems[:1])[0], computeChatLogChainHashes(7, "claude", elems[:1])[0], "seed differs by format")
}
