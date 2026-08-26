package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	tokenStatsDefaultTopN = 10
	tokenStatsMaxTopN     = 100
)

// TokenStatResponse is the shape returned by the token statistics
// endpoints. The aggregation and timeseries fields are sliced by the
// chosen dimension so the frontend can render either ranking or trend
// views from the same payload.
type TokenStatResponse struct {
	Dimension  string                    `json:"dimension"`
	Granularity string                   `json:"granularity"`
	TopN       int                       `json:"top_n"`
	Total      *model.TokenDimensionStat `json:"total"`
	Items      []*model.TokenDimensionStat `json:"items"`
	Timeseries [][]model.TokenTimePoint   `json:"timeseries"`
}

func parseTokenStatParams(c *gin.Context) (model.TokenStatFilter, model.TokenStatDimension, model.TokenStatGranularity, int, bool) {
	start, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || start <= 0 {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return model.TokenStatFilter{}, "", "", 0, false
	}
	end, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || end <= 0 {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return model.TokenStatFilter{}, "", "", 0, false
	}
	if end < start {
		common.ApiErrorMsg(c, "invalid time range")
		return model.TokenStatFilter{}, "", "", 0, false
	}

	dimension := model.TokenStatDimension(c.DefaultQuery("dimension", string(model.TokenStatDimensionModel)))
	switch dimension {
	case model.TokenStatDimensionUser, model.TokenStatDimensionModel:
	default:
		common.ApiErrorMsg(c, "invalid dimension")
		return model.TokenStatFilter{}, "", "", 0, false
	}

	granularity := model.TokenStatGranularity(c.DefaultQuery("granularity", string(model.TokenStatGranularityDay)))
	switch granularity {
	case model.TokenStatGranularityHour, model.TokenStatGranularityDay:
	default:
		common.ApiErrorMsg(c, "invalid granularity")
		return model.TokenStatFilter{}, "", "", 0, false
	}

	topN := tokenStatsDefaultTopN
	if raw := c.Query("top_n"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			common.ApiErrorMsg(c, "invalid top_n")
			return model.TokenStatFilter{}, "", "", 0, false
		}
		if parsed > tokenStatsMaxTopN {
			parsed = tokenStatsMaxTopN
		}
		topN = parsed
	}

	channel, _ := strconv.Atoi(c.Query("channel"))
	filter := model.TokenStatFilter{
		StartTimestamp: start,
		EndTimestamp:   end,
		Username:       c.Query("username"),
		ModelName:      c.Query("model_name"),
		Channel:        channel,
		Group:          c.Query("group"),
	}
	return filter, dimension, granularity, topN, true
}

func buildTokenStatResponse(c *gin.Context, filter model.TokenStatFilter, dimension model.TokenStatDimension, granularity model.TokenStatGranularity, topN int) (*TokenStatResponse, bool) {
	items, err := model.SumTokensByDimension(dimension, filter, topN)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return nil, false
	}
	total, err := model.SumTokensByDimension(dimension, filter, 0)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return nil, false
	}
	series, err := model.SumTokensTimeseries(dimension, granularity, filter, topN)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return nil, false
	}
	return &TokenStatResponse{
		Dimension:   string(dimension),
		Granularity: string(granularity),
		TopN:        topN,
		Items:       items,
		Total:       aggregateTokenTotal(total),
		Timeseries:  series,
	}, true
}

func aggregateTokenTotal(rows []*model.TokenDimensionStat) *model.TokenDimensionStat {
	total := &model.TokenDimensionStat{Name: "total"}
	for _, row := range rows {
		total.PromptTokens += row.PromptTokens
		total.CompletionTokens += row.CompletionTokens
		total.TotalTokens += row.TotalTokens
		total.Count += row.Count
		total.Quota += row.Quota
	}
	return total
}

// GetLogTokenStats is the admin token statistics endpoint. Admin users
// can optionally filter by username, model, channel, or group; the
// response aggregates rows from the logs table directly, so token
// counts include both prompt and completion tokens broken out per
// dimension member.
func GetLogTokenStats(c *gin.Context) {
	filter, dimension, granularity, topN, ok := parseTokenStatParams(c)
	if !ok {
		return
	}
	response, ok := buildTokenStatResponse(c, filter, dimension, granularity, topN)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetUserLogTokenStats is the self-service variant. The current user's
// id is always injected into the filter so a non-admin caller cannot
// inspect other users' token usage. The username query parameter is
// ignored on purpose to avoid privilege escalation via parameter
// injection.
func GetUserLogTokenStats(c *gin.Context) {
	filter, dimension, granularity, topN, ok := parseTokenStatParams(c)
	if !ok {
		return
	}
	userID := c.GetInt("id")
	if userID <= 0 {
		common.ApiErrorMsg(c, "missing user id in context")
		return
	}
	// Self view: the controller-side user id is the source of truth.
	filter.UserID = userID
	filter.Username = ""
	response, ok := buildTokenStatResponse(c, filter, dimension, granularity, topN)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}
