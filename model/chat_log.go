package model

import (
	"github.com/QuantumNous/new-api/common"
)

type ChatLog struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	TokenId      int    `json:"token_id" gorm:"index"`
	UserId       int    `json:"user_id" gorm:"index"`
	ChannelId    int    `json:"channel_id" gorm:"index"`
	ModelName    string `json:"model_name" gorm:"type:varchar(128);index"`
	RequestId    string `json:"request_id" gorm:"type:varchar(64);index"`
	RequestBody  string `json:"request_body" gorm:"type:text"`
	ResponseBody string `json:"response_body" gorm:"type:text"`
	IsStream     bool   `json:"is_stream"`
	Truncated    bool   `json:"truncated"`
	StatusCode   int    `json:"status_code" gorm:"default:0"`
	UseTime      int    `json:"use_time" gorm:"default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
}

func (ChatLog) TableName() string {
	return "chat_logs"
}

func ChatLogDBEnabled() bool {
	return CHATLOG_DB != nil
}

func (cl *ChatLog) Insert() error {
	cl.CreatedAt = common.GetTimestamp()
	return CHATLOG_DB.Create(cl).Error
}

func GetChatLogById(id int) (*ChatLog, error) {
	var cl ChatLog
	err := CHATLOG_DB.First(&cl, "id = ?", id).Error
	return &cl, err
}

func SearchChatLogs(tokenId, userId, channelId int, modelName, requestId string, page, pageSize int) ([]*ChatLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := CHATLOG_DB.Model(&ChatLog{})
	if tokenId > 0 {
		q = q.Where("token_id = ?", tokenId)
	}
	if userId > 0 {
		q = q.Where("user_id = ?", userId)
	}
	if channelId > 0 {
		q = q.Where("channel_id = ?", channelId)
	}
	if modelName != "" {
		q = q.Where("model_name = ?", modelName)
	}
	if requestId != "" {
		q = q.Where("request_id = ?", requestId)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []*ChatLog
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
