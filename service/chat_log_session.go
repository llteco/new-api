package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

type chatLogChain struct {
	SystemRaw json.RawMessage   // claude "system" / gemini "systemInstruction" field, nil if absent
	Elements  []json.RawMessage // chain elements: [system?, messages...]
	Messages  []json.RawMessage // Elements without the leading system element (delta storage)
}

// Returns nil chain when the body has no recognizable message list.
func extractChatLogChain(body []byte) *chatLogChain {
	var probe struct {
		System            json.RawMessage   `json:"system"`
		SystemInstruction json.RawMessage   `json:"systemInstruction"`
		Messages          []json.RawMessage `json:"messages"`
		Contents          []json.RawMessage `json:"contents"`
	}
	if err := common.Unmarshal(body, &probe); err != nil {
		return nil
	}
	chain := &chatLogChain{}
	msgs := probe.Messages
	if probe.System != nil {
		chain.SystemRaw = probe.System
	} else if probe.Contents != nil {
		if probe.SystemInstruction != nil {
			chain.SystemRaw = probe.SystemInstruction
		}
		msgs = probe.Contents
	}
	if len(msgs) == 0 {
		return nil
	}
	if chain.SystemRaw != nil {
		chain.Elements = append(chain.Elements, chain.SystemRaw)
	}
	chain.Messages = msgs
	chain.Elements = append(chain.Elements, msgs...)
	return chain
}

// h(-1) = SHA256("<tokenId>|<format>"); h(i) = SHA256(h(i-1) || SHA256(elem_i)).
// Returns hex hashes h_0..h_{len(elements)-1}.
func computeChatLogChainHashes(tokenId int, format string, elements []json.RawMessage) []string {
	seed := sha256.Sum256([]byte(strconv.Itoa(tokenId) + "|" + format))
	prev := seed[:]
	hashes := make([]string, 0, len(elements))
	for _, elem := range elements {
		elemHash := sha256.Sum256(elem)
		combined := append(append(make([]byte, 0, len(prev)+sha256.Size), prev...), elemHash[:]...)
		next := sha256.Sum256(combined)
		hashes = append(hashes, hex.EncodeToString(next[:]))
		prev = next[:]
	}
	return hashes
}
