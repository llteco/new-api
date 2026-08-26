package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TokenChannelQuota struct {
	Id          int `json:"id" gorm:"primaryKey"`
	TokenId     int `json:"token_id" gorm:"uniqueIndex:idx_token_channel,priority:1"`
	ChannelId   int `json:"channel_id" gorm:"uniqueIndex:idx_token_channel,priority:2"`
	RemainQuota int `json:"remain_quota" gorm:"default:0"`
	UsedQuota   int `json:"used_quota" gorm:"default:0"`
	ResetQuota  int `json:"reset_quota" gorm:"default:0"`
}

func (TokenChannelQuota) TableName() string {
	return "token_channel_quotas"
}

func GetTokenChannelQuota(tokenId, channelId int) (*TokenChannelQuota, error) {
	var row TokenChannelQuota
	err := DB.Where("token_id = ? AND channel_id = ?", tokenId, channelId).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func UpsertTokenChannelQuota(tokenId, channelId, resetQuota int) error {
	row := TokenChannelQuota{
		TokenId:     tokenId,
		ChannelId:   channelId,
		RemainQuota: resetQuota,
		ResetQuota:  resetQuota,
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token_id"}, {Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"remain_quota", "reset_quota"}),
	}).Create(&row).Error
}

func DecreaseTokenChannelQuota(tokenId, channelId, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return DB.Model(&TokenChannelQuota{}).
		Where("token_id = ? AND channel_id = ?", tokenId, channelId).
		Updates(map[string]interface{}{
			"remain_quota": gorm.Expr("remain_quota - ?", quota),
			"used_quota":   gorm.Expr("used_quota + ?", quota),
		}).Error
}

func IncreaseTokenChannelQuota(tokenId, channelId, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	return DB.Model(&TokenChannelQuota{}).
		Where("token_id = ? AND channel_id = ?", tokenId, channelId).
		Updates(map[string]interface{}{
			"remain_quota": gorm.Expr("remain_quota + ?", quota),
			"used_quota":   gorm.Expr("used_quota - ?", quota),
		}).Error
}

func GetAllTokenChannelQuotas(tokenId int) ([]TokenChannelQuota, error) {
	var rows []TokenChannelQuota
	err := DB.Where("token_id = ?", tokenId).Order("channel_id asc").Find(&rows).Error
	return rows, err
}

func ReplaceTokenChannelQuotas(tokenId int, items []TokenChannelQuota) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token_id = ?", tokenId).Delete(&TokenChannelQuota{}).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].Id = 0
			items[i].TokenId = tokenId
			items[i].RemainQuota = items[i].ResetQuota
			items[i].UsedQuota = 0
			if err := tx.Create(&items[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
