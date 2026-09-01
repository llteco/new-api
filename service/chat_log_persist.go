package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ChatLogRecorder struct {
	cw          *chatLogCaptureWriter
	requestBody string
	format      types.RelayFormat
}

func MaybeInstallChatLogCapture(c *gin.Context, format types.RelayFormat) *ChatLogRecorder {
	if !model.ChatLogDBEnabled() {
		return nil
	}
	wrapped := wrapWithChatLogCapture(c, c.Writer)
	cw, ok := wrapped.(*chatLogCaptureWriter)
	if !ok {
		return nil
	}
	return &ChatLogRecorder{cw: cw, format: format}
}

func (r *ChatLogRecorder) SetRequestBody(body []byte) {
	if r == nil || len(body) == 0 {
		return
	}
	r.requestBody = string(body)
}

func (r *ChatLogRecorder) Persist(c *gin.Context) {
	if r == nil || r.cw == nil {
		return
	}
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	channelId := c.GetInt("channel_id")
	modelName := c.GetString("original_model")
	requestId := c.GetString(common.RequestIdKey)
	isStream := c.GetBool("is_stream")
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)

	cw := r.cw
	requestBody := r.requestBody
	format := string(r.format)
	statusCode := cw.ResponseWriter.Status()
	gopool.Go(func() {
		respBody := cw.capturedBytes()
		useTime := 0
		if !startTime.IsZero() {
			useTime = int(time.Since(startTime).Seconds())
		}
		now := common.GetTimestamp()
		session, delta := resolveChatLogSession(tokenId, userId, format, requestId, requestBody)
		if session == nil {
			return
		}
		newMessages := "[]"
		if len(delta) > 0 {
			b, err := common.Marshal(delta)
			if err != nil {
				common.SysError("chat log: marshal delta failed: " + err.Error())
				return
			}
			newMessages = string(b)
		}
		turn := &model.ChatTurn{
			SessionId: session.Id, TurnIndex: session.TurnCount,
			RequestId: requestId, ModelName: modelName, ChannelId: channelId,
			StatusCode: statusCode, UseTime: useTime, IsStream: isStream,
			NewMessages: newMessages, ResponseBody: respBody,
		}
		if err := turn.Insert(); err != nil {
			common.SysError("chat log: insert turn failed: " + err.Error())
			return
		}
		if err := session.Advance(modelName, now); err != nil {
			common.SysError("chat log: advance session failed: " + err.Error())
		}
	})
}

// resolveChatLogSession finds or creates the session a request body belongs to.
// The returned session carries the post-turn MessageCount/PrefixHash so the
// caller's Advance persists exactly those values.
func resolveChatLogSession(tokenId, userId int, format, requestId, requestBody string) (*model.ChatSession, []json.RawMessage) {
	chain := extractChatLogChain([]byte(requestBody))
	if chain == nil {
		sum := sha256.Sum256([]byte(strconv.Itoa(tokenId) + "|" + format + "|" + requestId))
		session := &model.ChatSession{
			TokenId: tokenId, UserId: userId,
			PrefixHash: hex.EncodeToString(sum[:]),
		}
		if err := session.Insert(); err != nil {
			common.SysError("chat log: insert standalone session failed: " + err.Error())
			return nil, nil
		}
		return session, nil
	}
	hashes := computeChatLogChainHashes(tokenId, format, chain.Elements)
	existing, err := model.FindChatSessionByPrefixHashes(tokenId, hashes)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.SysError("chat log: find session failed: " + err.Error())
	}
	if err == nil {
		matchMsgIdx := existing.MessageCount
		if chain.SystemRaw != nil {
			matchMsgIdx--
		}
		delta := []json.RawMessage(nil)
		if matchMsgIdx > 0 {
			delta = chain.Messages[matchMsgIdx:]
		}
		existing.MessageCount = len(chain.Elements)
		existing.PrefixHash = hashes[len(hashes)-1]
		return existing, delta
	}
	session := &model.ChatSession{
		TokenId: tokenId, UserId: userId,
		System:       string(chain.SystemRaw),
		MessageCount: len(chain.Elements),
		PrefixHash:   hashes[len(hashes)-1],
	}
	if err := session.Insert(); err != nil {
		common.SysError("chat log: insert session failed: " + err.Error())
		return nil, nil
	}
	return session, chain.Messages
}
