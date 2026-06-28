package controller

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	logExportAdminMaxRows = 100_000
	logExportSelfMaxRows  = 50_000

	// csvUTF8BOM is the byte-order mark that makes Microsoft Excel
	// interpret the file as UTF-8 instead of a legacy codepage.
	csvUTF8BOM = "\xEF\xBB\xBF"
)

var logExportHeader = []string{
	"Time",
	"Type",
	"Username",
	"TokenName",
	"Group",
	"ModelName",
	"ChannelId",
	"PromptTokens",
	"CompletionTokens",
	"TotalTokens",
	"Quota",
	"UseTime(ms)",
	"Stream",
	"RequestId",
	"UpstreamRequestId",
	"Content",
}

type logExportRow struct {
	Time              string
	Type              string
	Username          string
	TokenName         string
	Group             string
	ModelName         string
	ChannelId         int
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
	Quota             int
	UseTimeMs         int
	IsStream          bool
	RequestId         string
	UpstreamRequestId string
	Content           string
}

func rowFromLog(log *model.Log) logExportRow {
	return logExportRow{
		Time:              time.Unix(log.CreatedAt, 0).UTC().Format("2006-01-02 15:04:05"),
		Type:              strconv.Itoa(log.Type),
		Username:          log.Username,
		TokenName:         log.TokenName,
		Group:             log.Group,
		ModelName:         log.ModelName,
		ChannelId:         log.ChannelId,
		PromptTokens:      log.PromptTokens,
		CompletionTokens:  log.CompletionTokens,
		TotalTokens:       log.PromptTokens + log.CompletionTokens,
		Quota:             log.Quota,
		UseTimeMs:         log.UseTime,
		IsStream:          log.IsStream,
		RequestId:         log.RequestId,
		UpstreamRequestId: log.UpstreamRequestId,
		Content:           log.Content,
	}
}

func (row logExportRow) toCSVRecord() []string {
	streamValue := "false"
	if row.IsStream {
		streamValue = "true"
	}
	return []string{
		row.Time,
		row.Type,
		row.Username,
		row.TokenName,
		row.Group,
		row.ModelName,
		strconv.Itoa(row.ChannelId),
		strconv.Itoa(row.PromptTokens),
		strconv.Itoa(row.CompletionTokens),
		strconv.Itoa(row.TotalTokens),
		strconv.Itoa(row.Quota),
		strconv.Itoa(row.UseTimeMs),
		streamValue,
		row.RequestId,
		row.UpstreamRequestId,
		row.Content,
	}
}

type logExportParams struct {
	logType           int
	startTimestamp    int64
	endTimestamp      int64
	username          string
	tokenName         string
	modelName         string
	channel           int
	group             string
	requestId         string
	upstreamRequestId string
	userID            int
}

func parseLogExportParams(c *gin.Context, isAdmin bool) (logExportParams, bool) {
	params := logExportParams{}
	params.logType, _ = strconv.Atoi(c.Query("type"))
	params.startTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	params.endTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	params.tokenName = c.Query("token_name")
	params.modelName = c.Query("model_name")
	params.group = c.Query("group")
	params.requestId = c.Query("request_id")
	params.upstreamRequestId = c.Query("upstream_request_id")
	params.channel, _ = strconv.Atoi(c.Query("channel"))

	if isAdmin {
		params.username = c.Query("username")
	} else {
		// Self view: bind the filter to the authenticated user so a
		// non-admin caller cannot pivot to other accounts.
		params.userID = c.GetInt("id")
		if params.userID <= 0 {
			common.ApiErrorMsg(c, "missing user id in context")
			return params, false
		}
	}
	return params, true
}

func writeLogsCSV(c *gin.Context, params logExportParams, maxRows int) {
	// Skip gzip compression: csv downloads should not be silently
	// rewritten as application/gzip.
	c.Header("Content-Encoding", "identity")
	filename := fmt.Sprintf("logs-%s.csv", time.Now().UTC().Format("20060102-150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)

	if _, err := c.Writer.WriteString(csvUTF8BOM); err != nil {
		common.SysError("failed to write CSV BOM: " + err.Error())
		return
	}

	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write(logExportHeader); err != nil {
		common.SysError("failed to write CSV header: " + err.Error())
		return
	}
	writer.Flush()
	if _, err := c.Writer.Write(buffer.Bytes()); err != nil {
		common.SysError("failed to flush CSV header: " + err.Error())
		return
	}
	buffer.Reset()

	rows, err := model.StreamLogsForExport(model.LogExportQuery{
		LogType:           params.logType,
		StartTimestamp:    params.startTimestamp,
		EndTimestamp:      params.endTimestamp,
		Username:          params.username,
		TokenName:         params.tokenName,
		ModelName:         params.modelName,
		Channel:           params.channel,
		Group:             params.group,
		RequestId:         params.requestId,
		UpstreamRequestId: params.upstreamRequestId,
		UserID:            params.userID,
		Limit:             maxRows,
	})
	if err != nil {
		common.SysError("failed to stream logs for export: " + err.Error())
		return
	}
	defer rows.Close()

	written := 0
	for rows.Next() {
		logRow, scanErr := rows.Scan()
		if scanErr != nil {
			common.SysError("failed to scan log row during export: " + scanErr.Error())
			return
		}
		row := rowFromLog(logRow)
		if err := writer.Write(row.toCSVRecord()); err != nil {
			common.SysError("failed to write CSV row: " + err.Error())
			return
		}
		written++
		// Flush in batches so memory usage stays flat and the
		// browser can start receiving bytes before the query ends.
		if written%200 == 0 {
			writer.Flush()
			if err := writer.Error(); err != nil {
				common.SysError("failed to flush CSV batch: " + err.Error())
				return
			}
			if _, err := c.Writer.Write(buffer.Bytes()); err != nil {
				common.SysError("failed to write CSV batch: " + err.Error())
				return
			}
			buffer.Reset()
		}
	}
	if err := rows.Err(); err != nil {
		common.SysError("log export iteration failed: " + err.Error())
		return
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		common.SysError("failed to flush remaining CSV rows: " + err.Error())
		return
	}
	if _, err := c.Writer.Write(buffer.Bytes()); err != nil {
		common.SysError("failed to write remaining CSV bytes: " + err.Error())
		return
	}
}

// ExportLogsCSV streams admin-scope log rows as CSV. It honours the
// same filter set as GetAllLogs but writes the response body directly
// to keep memory usage constant for large time windows.
func ExportLogsCSV(c *gin.Context) {
	params, ok := parseLogExportParams(c, true)
	if !ok {
		return
	}
	model.RecordLogWithAdminInfo(c.GetInt("id"), model.LogTypeManage, "export logs (admin)", map[string]interface{}{
		"action": "log.export",
		"params": map[string]interface{}{
			"start_timestamp":     params.startTimestamp,
			"end_timestamp":       params.endTimestamp,
			"type":                params.logType,
			"username":            params.username,
			"token_name":          params.tokenName,
			"model_name":          params.modelName,
			"channel":             params.channel,
			"group":               params.group,
			"request_id":          params.requestId,
			"upstream_request_id": params.upstreamRequestId,
		},
	})
	writeLogsCSV(c, params, logExportAdminMaxRows)
}

// ExportSelfLogsCSV streams the caller's own log rows as CSV.
func ExportSelfLogsCSV(c *gin.Context) {
	params, ok := parseLogExportParams(c, false)
	if !ok {
		return
	}
	writeLogsCSV(c, params, logExportSelfMaxRows)
}
