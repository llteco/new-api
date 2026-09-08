package model

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type ChatSession struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	TokenId   int    `json:"token_id" gorm:"index;uniqueIndex:idx_chat_sessions_token_prefix"`
	UserId    int    `json:"user_id" gorm:"index"`
	ModelName string `json:"model_name" gorm:"type:varchar(128);index"`
	System    string `json:"system" gorm:"type:text"`
	TurnCount int    `json:"turn_count"`
	// MessageCount is the number of request messages covered by PrefixHash.
	MessageCount int    `json:"message_count"`
	PrefixHash   string `json:"prefix_hash" gorm:"type:varchar(64);uniqueIndex:idx_chat_sessions_token_prefix"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	LastActiveAt int64  `json:"last_active_at" gorm:"bigint;index"`
}

func (ChatSession) TableName() string {
	return "chat_sessions"
}

func ChatLogDBEnabled() bool {
	return CHATLOG_DB != nil
}

func (s *ChatSession) Insert() error {
	now := common.GetTimestamp()
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	if s.LastActiveAt == 0 {
		s.LastActiveAt = now
	}
	if err := CHATLOG_DB.Create(s).Error; err != nil {
		return err
	}
	chatLogCacheRecordSession(s)
	return nil
}

type ChatTurn struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	SessionId    int    `json:"session_id" gorm:"index:idx_chat_turns_session_turn,priority:1"`
	TurnIndex    int    `json:"turn_index" gorm:"index:idx_chat_turns_session_turn,priority:2"`
	RequestId    string `json:"request_id" gorm:"type:varchar(64);index"`
	ModelName    string `json:"model_name" gorm:"type:varchar(128)"`
	ChannelId    int    `json:"channel_id" gorm:"index"`
	StatusCode   int    `json:"status_code" gorm:"default:0"`
	UseTime      int    `json:"use_time" gorm:"default:0"`
	IsStream     bool   `json:"is_stream"`
	NewMessages  string `json:"new_messages" gorm:"type:text"`
	ResponseBody string `json:"response_body" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
}

func (ChatTurn) TableName() string {
	return "chat_turns"
}

func (t *ChatTurn) Insert() error {
	if t.CreatedAt == 0 {
		t.CreatedAt = common.GetTimestamp()
	}
	if err := CHATLOG_DB.Create(t).Error; err != nil {
		return err
	}
	chatLogCacheRecordTurn(t)
	return nil
}

func FindChatSessionByPrefixHashes(tokenId int, hashes []string) (*ChatSession, error) {
	var s ChatSession
	err := CHATLOG_DB.Where("token_id = ? AND prefix_hash IN ?", tokenId, hashes).
		Order("message_count desc").First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *ChatSession) Advance(modelName string, at int64) error {
	s.TurnCount++
	s.LastActiveAt = at
	s.ModelName = modelName
	if err := CHATLOG_DB.Model(s).Updates(map[string]any{
		"turn_count":     s.TurnCount,
		"message_count":  s.MessageCount,
		"prefix_hash":    s.PrefixHash,
		"last_active_at": s.LastActiveAt,
		"model_name":     s.ModelName,
	}).Error; err != nil {
		return err
	}
	chatLogCacheAdvanceSession(s)
	return nil
}

// ChatSessionCursor is the keyset position of a session in the newest-first
// (last_active_at desc, id desc) session list.
type ChatSessionCursor struct {
	LastActiveAt int64
	Id           int
}

func (c ChatSessionCursor) Encode() string {
	raw := fmt.Sprintf("%d:%d", c.LastActiveAt, c.Id)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func DecodeChatSessionCursor(s string) (ChatSessionCursor, error) {
	if s == "" {
		return ChatSessionCursor{}, nil
	}
	raw, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return ChatSessionCursor{}, err
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 {
		return ChatSessionCursor{}, fmt.Errorf("invalid chat session cursor")
	}
	at, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ChatSessionCursor{}, err
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return ChatSessionCursor{}, err
	}
	return ChatSessionCursor{LastActiveAt: at, Id: id}, nil
}

// ChatSessionFilter narrows the session list. Zero values mean "no filter".
type ChatSessionFilter struct {
	TokenId   int
	UserId    int
	ModelName string
	// StartTs/EndTs bound last_active_at (unix seconds), inclusive.
	StartTs int64
	EndTs   int64
}

func (f ChatSessionFilter) Empty() bool {
	return f.TokenId == 0 && f.UserId == 0 && f.ModelName == "" && f.StartTs == 0 && f.EndTs == 0
}

// ListChatSessions returns one keyset page of sessions, newest first. An empty
// cursor fetches the first page. hasMore reports whether older sessions may
// follow, so callers never need a full-table COUNT.
func ListChatSessions(filter ChatSessionFilter, cursor ChatSessionCursor, limit int) (sessions []*ChatSession, hasMore bool, err error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q := CHATLOG_DB.Model(&ChatSession{})
	if filter.TokenId > 0 {
		q = q.Where("token_id = ?", filter.TokenId)
	}
	if filter.UserId > 0 {
		q = q.Where("user_id = ?", filter.UserId)
	}
	if filter.ModelName != "" {
		q = q.Where("model_name = ?", filter.ModelName)
	}
	if filter.StartTs > 0 {
		q = q.Where("last_active_at >= ?", filter.StartTs)
	}
	if filter.EndTs > 0 {
		q = q.Where("last_active_at <= ?", filter.EndTs)
	}
	if cursor.Id > 0 || cursor.LastActiveAt > 0 {
		// expanded row-value comparison so every supported database can use the
		// last_active_at index (MySQL 5.7 cannot optimize (a,b) < (?,?))
		q = q.Where("last_active_at < ? OR (last_active_at = ? AND id < ?)",
			cursor.LastActiveAt, cursor.LastActiveAt, cursor.Id)
	}
	if err := q.Order("last_active_at desc, id desc").Limit(limit + 1).Find(&sessions).Error; err != nil {
		return nil, false, err
	}
	if len(sessions) > limit {
		sessions = sessions[:limit]
		hasMore = true
	}
	return sessions, hasMore, nil
}

func GetChatSessionById(id int) (*ChatSession, error) {
	var s ChatSession
	err := CHATLOG_DB.First(&s, "id = ?", id).Error
	return &s, err
}

// GetChatTurnsPage returns one page of a session's turns in ascending id
// order. With beforeId == 0 it returns the newest page; otherwise the page
// older than beforeId. hasMore reports whether even older turns exist.
func GetChatTurnsPage(sessionId int, beforeId int, limit int) (turns []*ChatTurn, hasMore bool, err error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q := CHATLOG_DB.Where("session_id = ?", sessionId)
	if beforeId > 0 {
		q = q.Where("id < ?", beforeId)
	}
	if err := q.Order("id desc").Limit(limit + 1).Find(&turns).Error; err != nil {
		return nil, false, err
	}
	if len(turns) > limit {
		turns = turns[:limit]
		hasMore = true
	}
	// newest-first from the DB; the transcript reads oldest-first
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return turns, hasMore, nil
}
