package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func AdminGetChatSessions(c *gin.Context) {
	if !model.ChatLogDBEnabled() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "对话详情库未配置"})
		return
	}
	tokenId, _ := strconv.Atoi(c.Query("token_id"))
	userId, _ := strconv.Atoi(c.Query("user_id"))
	modelName := c.Query("model_name")
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	sessions, total, err := model.SearchChatSessions(tokenId, userId, modelName, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	type chatSessionMeta struct {
		Id           int    `json:"id"`
		TokenId      int    `json:"token_id"`
		UserId       int    `json:"user_id"`
		ModelName    string `json:"model_name"`
		TurnCount    int    `json:"turn_count"`
		MessageCount int    `json:"message_count"`
		CreatedAt    int64  `json:"created_at"`
		LastActiveAt int64  `json:"last_active_at"`
	}
	out := make([]chatSessionMeta, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, chatSessionMeta{
			Id: s.Id, TokenId: s.TokenId, UserId: s.UserId, ModelName: s.ModelName,
			TurnCount: s.TurnCount, MessageCount: s.MessageCount,
			CreatedAt: s.CreatedAt, LastActiveAt: s.LastActiveAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out, "total": total})
}

func AdminGetChatSessionDetail(c *gin.Context) {
	if !model.ChatLogDBEnabled() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "对话详情库未配置"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效 ID"})
		return
	}
	session, err := model.GetChatSessionById(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "会话不存在"})
		return
	}
	turns, err := model.GetChatTurnsBySessionId(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"session": session, "turns": turns}})
}
