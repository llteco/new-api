package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// logAutoExportHandler runs the scheduled weekly log auto-export job.
// It exports consumption logs to a JSON file in the configured directory.
// The trigger time, duration, and output directory are all configurable
// via the admin management panel.
type logAutoExportHandler struct{}

func (logAutoExportHandler) Type() string { return model.SystemTaskTypeLogAutoExport }

func (logAutoExportHandler) Enabled() bool {
	setting := operation_setting.GetLogExportSetting()
	if !setting.Enabled {
		return false
	}
	return isWeeklyLogExportDue()
}

func (logAutoExportHandler) Interval() time.Duration {
	setting := operation_setting.GetLogExportSetting()
	return time.Duration(setting.IntervalMinutes) * time.Minute
}

func (logAutoExportHandler) NewPayload() any { return nil }

func (h logAutoExportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if err := runLogAutoExport(ctx); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, nil, nil)
}

// isWeeklyLogExportDue checks whether the current time is past the configured
// weekday/hour/minute and the job has not yet run this week.
func isWeeklyLogExportDue() bool {
	setting := operation_setting.GetLogExportSetting()
	now := time.Now()

	// Find the configured day this week at the configured time
	weekday := now.Weekday()
	daysUntilTarget := (time.Weekday(setting.Weekday) - weekday) % 7
	if daysUntilTarget < 0 {
		daysUntilTarget += 7
	}
	targetTime := time.Date(now.Year(), now.Month(), now.Day(),
		setting.Hour, setting.Minute, 0, 0, now.Location())
	targetTime = targetTime.AddDate(0, 0, int(daysUntilTarget))

	if now.Before(targetTime) {
		return false
	}

	// Check if we already ran this week (after the target time)
	latest, err := model.GetLatestSystemTask(model.SystemTaskTypeLogAutoExport)
	if err != nil {
		return true
	}
	if latest != nil && latest.UpdatedAt >= targetTime.Unix() {
		return false
	}
	return true
}

// logAutoExportData matches the requested JSON output structure.
type logAutoExportData struct {
	Tokens  int64                                             `json:"tokens"`
	Users   map[string]map[string]int64                       `json:"users"`
	Details map[string]map[string]map[string]logHourDetail `json:"details"`
}

type logHourDetail struct {
	Prompt     int64    `json:"prompt"`
	Completion int64    `json:"completion"`
	Cache      int64    `json:"cache"`
	Types      []string `json:"types,omitempty"`
}

func runLogAutoExport(ctx context.Context) error {
	setting := operation_setting.GetLogExportSetting()
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -setting.DurationDays)

	logs, err := model.GetLogsForExport(startTime.Unix(), endTime.Unix())
	if err != nil {
		return fmt.Errorf("query logs failed: %w", err)
	}

	data := buildLogExportData(logs)

	jsonBytes, err := common.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal export data failed: %w", err)
	}

	logDir := setting.OutputDir
	if logDir == "" {
		logDir = *common.LogDir
	}
	if logDir == "" || logDir == "." {
		logDir = filepath.Dir(logger.GetCurrentLogPath())
	}
	if logDir == "" || logDir == "." {
		logDir, _ = os.Getwd()
	}

	filename := fmt.Sprintf("log-export-%s.json", endTime.Format("20060102-150405"))
	filePath := filepath.Join(logDir, filename)

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("create export directory failed: %w", err)
	}

	if err := os.WriteFile(filePath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("write export file failed: %w", err)
	}

	logger.LogInfo(ctx, fmt.Sprintf("log auto export completed: %s (logs=%d, tokens=%d)", filePath, len(logs), data.Tokens))
	return nil
}

func buildLogExportData(logs []*model.Log) logAutoExportData {
	data := logAutoExportData{
		Tokens:  0,
		Users:   make(map[string]map[string]int64),
		Details: make(map[string]map[string]map[string]logHourDetail),
	}

	for _, log := range logs {
		if log.Type != model.LogTypeConsume {
			continue
		}

		username := log.Username
		if username == "" {
			username = fmt.Sprintf("user_%d", log.UserId)
		}
		modelName := log.ModelName
		if modelName == "" {
			modelName = "unknown"
		}

		promptTokens := int64(log.PromptTokens)
		completionTokens := int64(log.CompletionTokens)
		cacheTokens := int64(0)

		// Extract cache_tokens from Other field
		var typeLabel string
		if log.Other != "" {
			otherMap, _ := common.StrToMap(log.Other)
			if otherMap != nil {
				if v, ok := otherMap["cache_tokens"]; ok {
					switch val := v.(type) {
					case float64:
						cacheTokens = int64(val)
					case int:
						cacheTokens = int64(val)
					case int64:
						cacheTokens = val
					}
				}
				if conv, ok := otherMap["request_conversion"]; ok {
					if convArr, ok := conv.([]interface{}); ok && len(convArr) > 0 {
						parts := make([]string, 0, len(convArr))
						for _, item := range convArr {
							if s, ok := item.(string); ok {
								parts = append(parts, s)
							}
						}
						if len(parts) > 0 {
							typeLabel = strings.Join(parts, " → ")
						}
					}
				}
			}
		}

		totalTokens := promptTokens + completionTokens
		data.Tokens += totalTokens

		// Users aggregation
		if data.Users[username] == nil {
			data.Users[username] = make(map[string]int64)
		}
		data.Users[username][modelName] += totalTokens

		// Details aggregation by hour
		hourKey := time.Unix(log.CreatedAt, 0).UTC().Format("2006-01-02T15:00:00Z")
		if data.Details[username] == nil {
			data.Details[username] = make(map[string]map[string]logHourDetail)
		}
		if data.Details[username][hourKey] == nil {
			data.Details[username][hourKey] = make(map[string]logHourDetail)
		}
		detail := data.Details[username][hourKey][modelName]
		detail.Prompt += promptTokens
		detail.Completion += completionTokens
		detail.Cache += cacheTokens
		if typeLabel != "" {
			found := false
			for _, t := range detail.Types {
				if t == typeLabel {
					found = true
					break
				}
			}
			if !found {
				detail.Types = append(detail.Types, typeLabel)
			}
		}
		data.Details[username][hourKey][modelName] = detail
	}

	return data
}
