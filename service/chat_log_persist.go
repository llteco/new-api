package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const defaultChatLogMaxBodyBytes = 262144

type ChatLogRecorder struct {
	cw          *chatLogCaptureWriter
	requestBody string
}

func MaybeInstallChatLogCapture(c *gin.Context) *ChatLogRecorder {
	if !model.ChatLogDBEnabled() {
		return nil
	}
	wrapped := wrapWithChatLogCapture(c, c.Writer, defaultChatLogMaxBodyBytes)
	cw, ok := wrapped.(*chatLogCaptureWriter)
	if !ok {
		return nil
	}
	return &ChatLogRecorder{cw: cw}
}

func (r *ChatLogRecorder) SetRequestBody(body []byte) {
	if r == nil || len(body) == 0 {
		return
	}
	if len(body) > defaultChatLogMaxBodyBytes {
		r.requestBody = string(body[:defaultChatLogMaxBodyBytes])
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
	statusCode := cw.ResponseWriter.Status()
	gopool.Go(func() {
		respBody, truncated := cw.capturedBytes()
		useTime := 0
		if !startTime.IsZero() {
			useTime = int(time.Since(startTime).Seconds())
		}
		cl := &model.ChatLog{
			TokenId: tokenId, UserId: userId, ChannelId: channelId,
			ModelName: modelName, RequestId: requestId,
			RequestBody: requestBody, ResponseBody: respBody,
			IsStream: isStream, Truncated: truncated,
			StatusCode: statusCode,
			UseTime:    useTime,
		}
		if err := cl.Insert(); err != nil {
			common.SysError("failed to insert chat log: " + err.Error())
		}
	})
}
