package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

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

func toChatSessionMeta(s *model.ChatSession) chatSessionMeta {
	return chatSessionMeta{
		Id: s.Id, TokenId: s.TokenId, UserId: s.UserId, ModelName: s.ModelName,
		TurnCount: s.TurnCount, MessageCount: s.MessageCount,
		CreatedAt: s.CreatedAt, LastActiveAt: s.LastActiveAt,
	}
}

// AdminGetChatSessions lists sessions newest-first using cursor pagination.
// The unfiltered first page is served from the hot cache when possible; every
// other query falls through to keyset queries on the chat-log database.
func AdminGetChatSessions(c *gin.Context) {
	if !model.ChatLogDBEnabled() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "对话详情库未配置"})
		return
	}
	tokenId, _ := strconv.Atoi(c.Query("token_id"))
	userId, _ := strconv.Atoi(c.Query("user_id"))
	modelName := c.Query("model_name")
	startTs, _ := strconv.ParseInt(c.Query("start_ts"), 10, 64)
	endTs, _ := strconv.ParseInt(c.Query("end_ts"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	cursorStr := c.Query("cursor")

	filter := model.ChatSessionFilter{
		TokenId: tokenId, UserId: userId, ModelName: modelName,
		StartTs: startTs, EndTs: endTs,
	}

	var sessions []*model.ChatSession
	var hasMore bool
	servedFromHot := false
	if filter.Empty() && cursorStr == "" {
		// 近期会话默认视图：优先走内存热缓存，未命中或刷新失败再回源数据库
		if hot, hotHasMore, ok := model.ListRecentChatSessions(limit); ok {
			sessions, hasMore, servedFromHot = hot, hotHasMore, true
		}
	}
	if !servedFromHot {
		cursor, err := model.DecodeChatSessionCursor(cursorStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的分页游标"})
			return
		}
		sessions, hasMore, err = model.ListChatSessions(filter, cursor, limit)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	items := make([]chatSessionMeta, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, toChatSessionMeta(s))
	}
	var nextCursor any
	if hasMore && len(sessions) > 0 {
		last := sessions[len(sessions)-1]
		nextCursor = model.ChatSessionCursor{LastActiveAt: last.LastActiveAt, Id: last.Id}.Encode()
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":       items,
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		},
	})
}

// AdminGetChatSessionDetail returns a session's metadata plus one page of
// turns (ascending). The newest page is served from the hot cache when
// available; older pages cold-load from the database via before_id.
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
	limit, _ := strconv.Atoi(c.Query("limit"))
	beforeId, _ := strconv.Atoi(c.Query("before_id"))

	session, err := model.GetChatSessionById(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "会话不存在"})
		return
	}

	var turns []*model.ChatTurn
	var hasMore bool
	servedFromHot := false
	if beforeId == 0 {
		if hot, hotHasMore, ok := model.GetHotChatTurns(id, session.TurnCount, limit); ok {
			turns, hasMore, servedFromHot = hot, hotHasMore, true
		}
	}
	if !servedFromHot {
		turns, hasMore, err = model.GetChatTurnsPage(id, beforeId, limit)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if beforeId == 0 {
			model.AdmitChatTurns(id, turns)
		}
	}

	var nextTurnId any
	if hasMore && len(turns) > 0 {
		nextTurnId = turns[0].Id
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"session":      session,
			"turns":        turns,
			"has_more":     hasMore,
			"next_turn_id": nextTurnId,
		},
	})
}
