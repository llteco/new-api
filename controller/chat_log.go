package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func AdminGetChatLogs(c *gin.Context) {
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
	type chatLogMeta struct {
		Id         int    `json:"id"`
		TokenId    int    `json:"token_id"`
		UserId     int    `json:"user_id"`
		ChannelId  int    `json:"channel_id"`
		ModelName  string `json:"model_name"`
		RequestId  string `json:"request_id"`
		IsStream   bool   `json:"is_stream"`
		Truncated  bool   `json:"truncated"`
		StatusCode int    `json:"status_code"`
		UseTime    int    `json:"use_time"`
		CreatedAt  int64  `json:"created_at"`
	}
	out := make([]chatLogMeta, 0, len(logs))
	for _, l := range logs {
		out = append(out, chatLogMeta{
			Id: l.Id, TokenId: l.TokenId, UserId: l.UserId, ChannelId: l.ChannelId,
			ModelName: l.ModelName, RequestId: l.RequestId, IsStream: l.IsStream,
			Truncated: l.Truncated, StatusCode: l.StatusCode, UseTime: l.UseTime, CreatedAt: l.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out, "total": total})
}

func AdminGetChatLogDetail(c *gin.Context) {
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
