package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// TokenStatDimension selects how token statistics are grouped in
// SumTokensByDimension. Only "user" and "model" are supported.
type TokenStatDimension string

const (
	TokenStatDimensionUser  TokenStatDimension = "user"
	TokenStatDimensionModel TokenStatDimension = "model"
)

// TokenStatGranularity selects the time bucket size used by
// SumTokensTimeseries. Hour buckets align with the logs table index
// granularity; day buckets keep the response payload small.
type TokenStatGranularity string

const (
	TokenStatGranularityHour TokenStatGranularity = "hour"
	TokenStatGranularityDay  TokenStatGranularity = "day"
)

// TokenDimensionStat is one row in a token aggregation. The Name column
// carries either a username or a model name, depending on the chosen
// dimension. TotalTokens is the sum of prompt and completion tokens.
type TokenDimensionStat struct {
	Name             string `json:"name" gorm:"column:name"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"column:prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens" gorm:"column:completion_tokens"`
	TotalTokens      int    `json:"total_tokens" gorm:"column:total_tokens"`
	Count            int    `json:"count" gorm:"column:count"`
	Quota            int    `json:"quota" gorm:"column:quota"`
}

// TokenTimePoint is one bucket of the token timeseries response.
type TokenTimePoint struct {
	Timestamp int64  `json:"timestamp" gorm:"column:timestamp"`
	Name      string `json:"name" gorm:"column:name"`
	Tokens    int    `json:"tokens" gorm:"column:tokens"`
}

// TokenStatFilter captures the common filter set used by both
// SumTokensByDimension and SumTokensTimeseries. Either Username or
// UserID can be set; the controller is expected to set exactly one of
// them so the query binds to a single column unambiguously.
type TokenStatFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
	UserID         int
	Username       string
	ModelName      string
	Channel        int
	Group          string
}

// validateTokenStatFilter ensures the time range is provided and ordered.
// Token statistics always need a bounded window so the underlying
// index on `created_at` can be used.
func validateTokenStatFilter(filter TokenStatFilter) error {
	if filter.StartTimestamp <= 0 || filter.EndTimestamp <= 0 {
		return errors.New("时间范围不合法")
	}
	if filter.EndTimestamp < filter.StartTimestamp {
		return errors.New("时间范围不合法")
	}
	return nil
}

func (filter TokenStatFilter) apply(tx *gorm.DB, columnPrefix string) (*gorm.DB, error) {
	if columnPrefix == "" {
		columnPrefix = "logs."
	}
	if filter.StartTimestamp > 0 {
		tx = tx.Where(columnPrefix+"created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		tx = tx.Where(columnPrefix+"created_at <= ?", filter.EndTimestamp)
	}
	if filter.Channel > 0 {
		tx = tx.Where(columnPrefix+"channel_id = ?", filter.Channel)
	}
	if filter.Group != "" {
		tx = tx.Where(columnPrefix+logGroupCol+" = ?", filter.Group)
	}
	if filter.UserID > 0 {
		tx = tx.Where(columnPrefix+"user_id = ?", filter.UserID)
	}
	tx, err := applyExplicitLogTextFilter(tx, columnPrefix+"model_name", filter.ModelName)
	if err != nil {
		return nil, err
	}
	tx, err = applyExplicitLogTextFilter(tx, columnPrefix+"username", filter.Username)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// SumTokensByDimension returns the per-name token totals within the
// supplied filter window. dimension selects the GROUP BY column.
// topN <= 0 returns every row.
func SumTokensByDimension(dimension TokenStatDimension, filter TokenStatFilter, topN int) ([]*TokenDimensionStat, error) {
	if err := validateTokenStatFilter(filter); err != nil {
		return nil, err
	}
	column, ok := tokenDimensionColumn(dimension)
	if !ok {
		return nil, errors.New("unsupported token stat dimension")
	}

	base := LOG_DB.Table("logs").
		Select(strings.Join([]string{
			column + " AS name",
			"COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens",
			"COALESCE(SUM(completion_tokens), 0) AS completion_tokens",
			"COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS total_tokens",
			"COUNT(*) AS count",
			"COALESCE(SUM(quota), 0) AS quota",
		}, ", ")).
		Where("type = ?", LogTypeConsume)
	base, err := filter.apply(base, "logs.")
	if err != nil {
		return nil, err
	}

	base = base.Group(column)
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		base = base.Order(gorm.Expr("total_tokens DESC"))
	} else {
		base = base.Order("total_tokens DESC")
	}
	if topN > 0 {
		base = base.Limit(topN)
	}

	var rows []*TokenDimensionStat
	if err := base.Scan(&rows).Error; err != nil {
		common.SysError("failed to sum tokens by dimension: " + err.Error())
		return nil, errors.New("查询 token 统计失败")
	}
	for i := range rows {
		rows[i].TotalTokens = rows[i].PromptTokens + rows[i].CompletionTokens
	}
	return rows, nil
}

// SumTokensTimeseries returns the per-bucket token total for each top
// dimension member. The topN argument is resolved against the totals for
// the supplied filter window so the line chart only renders the most
// significant series; pass topN <= 0 to keep every dimension value.
func SumTokensTimeseries(dimension TokenStatDimension, granularity TokenStatGranularity, filter TokenStatFilter, topN int) ([][]TokenTimePoint, error) {
	if err := validateTokenStatFilter(filter); err != nil {
		return nil, err
	}
	column, ok := tokenDimensionColumn(dimension)
	if !ok {
		return nil, errors.New("unsupported token stat dimension")
	}
	bucketSeconds, err := tokenGranularitySeconds(granularity)
	if err != nil {
		return nil, err
	}

	totals, err := SumTokensByDimension(dimension, filter, topN)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(totals))
	for _, row := range totals {
		if row.Name == "" {
			continue
		}
		names = append(names, row.Name)
	}
	if len(names) == 0 {
		return [][]TokenTimePoint{}, nil
	}

	tsSelect := tokenTimeseriesTimestampExpr(granularity)
	selectColumns := strings.Join([]string{
		tsSelect + " AS timestamp",
		column + " AS name",
		"COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS tokens",
	}, ", ")

	tx := LOG_DB.Table("logs").
		Select(selectColumns).
		Where("type = ?", LogTypeConsume).
		Where(column+" IN ?", names)
	tx, err = filter.apply(tx, "logs.")
	if err != nil {
		return nil, err
	}
	tx = tx.Group(strings.Join([]string{tsSelect, column}, ", ")).
		Order(tsSelect + " ASC")

	rows := make([]TokenTimePoint, 0)
	if err := tx.Scan(&rows).Error; err != nil {
		common.SysError("failed to sum tokens timeseries: " + err.Error())
		return nil, errors.New("查询 token 趋势失败")
	}

	buckets := make(map[int64]map[string]int, 32)
	timestamps := make([]int64, 0, 32)
	for _, row := range rows {
		ts := alignTimestampToBucket(row.Timestamp, bucketSeconds)
		if _, ok := buckets[ts]; !ok {
			buckets[ts] = make(map[string]int, len(names))
			timestamps = append(timestamps, ts)
		}
		buckets[ts][row.Name] = row.Tokens
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	series := make([][]TokenTimePoint, 0, len(names))
	for _, name := range names {
		points := make([]TokenTimePoint, 0, len(timestamps))
		for _, ts := range timestamps {
			points = append(points, TokenTimePoint{
				Timestamp: ts,
				Name:      name,
				Tokens:    buckets[ts][name],
			})
		}
		series = append(series, points)
	}
	return series, nil
}

func tokenDimensionColumn(dimension TokenStatDimension) (string, bool) {
	switch dimension {
	case TokenStatDimensionUser:
		return "username", true
	case TokenStatDimensionModel:
		return "model_name", true
	default:
		return "", false
	}
}

func tokenGranularitySeconds(granularity TokenStatGranularity) (int64, error) {
	switch granularity {
	case TokenStatGranularityHour:
		return 3600, nil
	case TokenStatGranularityDay:
		return 86400, nil
	default:
		return 0, errors.New("unsupported token stat granularity")
	}
}

func tokenTimeseriesTimestampExpr(granularity TokenStatGranularity) string {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		switch granularity {
		case TokenStatGranularityDay:
			return "toStartOfDay(toDateTime(created_at))"
		default:
			return "toStartOfHour(toDateTime(created_at))"
		}
	}
	switch granularity {
	case TokenStatGranularityDay:
		return "(created_at - (created_at % 86400))"
	default:
		return "(created_at - (created_at % 3600))"
	}
}

func alignTimestampToBucket(ts int64, bucketSeconds int64) int64 {
	if bucketSeconds <= 0 {
		return ts
	}
	return ts - (ts % bucketSeconds)
}
