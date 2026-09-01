package model

import (
	"github.com/QuantumNous/new-api/common"
)

type ChatSession struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	TokenId      int    `json:"token_id" gorm:"index;uniqueIndex:idx_chat_sessions_token_prefix"`
	UserId       int    `json:"user_id" gorm:"index"`
	ModelName    string `json:"model_name" gorm:"type:varchar(128)"`
	System       string `json:"system" gorm:"type:text"`
	TurnCount    int    `json:"turn_count"`
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
	return CHATLOG_DB.Create(s).Error
}

type ChatTurn struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	SessionId    int    `json:"session_id" gorm:"index"`
	TurnIndex    int    `json:"turn_index"`
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
	return CHATLOG_DB.Create(t).Error
}

func GetChatTurnsBySessionId(sessionId int) ([]*ChatTurn, error) {
	var turns []*ChatTurn
	err := CHATLOG_DB.Where("session_id = ?", sessionId).Order("turn_index asc").Find(&turns).Error
	return turns, err
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
	return CHATLOG_DB.Model(s).Updates(map[string]any{
		"turn_count":     s.TurnCount,
		"message_count":  s.MessageCount,
		"prefix_hash":    s.PrefixHash,
		"last_active_at": s.LastActiveAt,
		"model_name":     s.ModelName,
	}).Error
}

func SearchChatSessions(tokenId, userId int, modelName string, page, pageSize int) ([]*ChatSession, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := CHATLOG_DB.Model(&ChatSession{})
	if tokenId > 0 {
		q = q.Where("token_id = ?", tokenId)
	}
	if userId > 0 {
		q = q.Where("user_id = ?", userId)
	}
	if modelName != "" {
		q = q.Where("model_name = ?", modelName)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var sessions []*ChatSession
	if err := q.Order("last_active_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&sessions).Error; err != nil {
		return nil, 0, err
	}
	return sessions, total, nil
}

func GetChatSessionById(id int) (*ChatSession, error) {
	var s ChatSession
	err := CHATLOG_DB.First(&s, "id = ?", id).Error
	return &s, err
}
