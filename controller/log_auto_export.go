package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	// Check if we already ran this week (after the target time). Only
	// successful runs count: a failed export is retried on the next check.
	latest, err := model.GetLatestSystemTaskByStatus(model.SystemTaskTypeLogAutoExport, model.SystemTaskStatusSucceeded)
	if err != nil {
		return true
	}
	if latest != nil && latest.UpdatedAt >= targetTime.Unix() {
		return false
	}
	return true
}

// logAutoExportData matches the requested JSON output structure. Details holds
// one record per consume log row in the export window with no aggregation, so
// the exported numbers cannot drift from the raw logs. All token fields are
// normalized across the three log kinds (openai-native, anthropic-native,
// api-converted): prompt excludes cache, cache holds the cached portion
// (read + creation), so prompt + cache is the record's total input. The
// tokens/users sums use the dashboard-aligned effective input, which equals
// prompt + cache + completion summed over the records.
type logAutoExportData struct {
	Tokens  int64                       `json:"tokens"`
	Users   map[string]map[string]int64 `json:"users"`
	Details []logExportRecord           `json:"details"`
}

// logExportRecord is a single consume log row in the export window. Types
// carries the request conversion chain when the request was converted between
// API formats.
type logExportRecord struct {
	Username   string   `json:"username"`
	Model      string   `json:"model"`
	Time       string   `json:"time"`
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

	if setting.PushUrl != "" {
		if err := pushLogExport(ctx, setting.PushUrl, filename, jsonBytes); err != nil {
			return fmt.Errorf("push export to %s failed: %w", setting.PushUrl, err)
		}
		logger.LogInfo(ctx, "log auto export pushed to "+setting.PushUrl)
	}
	return nil
}

// pushLogExportRetryBackoff is the pause between push attempts; tests shorten it.
var pushLogExportRetryBackoff = 5 * time.Second

// pushLogExport POSTs the exported JSON to pushURL with the export filename in
// the filename query parameter. The receiver dedups by filename, so retries
// are idempotent. Non-2xx and transport errors are retried to ride out a
// restarting receiver; the response body (first 200 bytes) is surfaced in the
// error so the receiving side's message stays visible.
func pushLogExport(ctx context.Context, pushURL, filename string, body []byte) error {
	sep := "?"
	if strings.Contains(pushURL, "?") {
		sep = "&"
	}
	target := pushURL + sep + "filename=" + url.QueryEscape(filename)
	client := &http.Client{Timeout: 30 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pushLogExportRetryBackoff):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return fmt.Errorf("push to %s failed after 3 attempts: %w", target, lastErr)
}

func buildLogExportData(logs []*model.Log) logAutoExportData {
	data := logAutoExportData{
		Tokens:  0,
		Users:   make(map[string]map[string]int64),
		Details: make([]logExportRecord, 0, len(logs)),
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
		cacheReadTokens := int64(0)
		cacheCreationTokens := int64(0)
		anthropicSemantic := false

		// Normalize all three log kinds (openai-native, anthropic-native,
		// api-converted) to one split: prompt holds only non-cached input
		// tokens, cache holds the cached portion (read + creation).
		// Anthropic-native and converted-to-anthropic logs store cache
		// separately from prompt_tokens, so the split is already normalized.
		// OpenAI-native and converted-to-openai logs include the cache in
		// prompt_tokens, so the cached portion is subtracted back out for the
		// exported record.
		var typeLabel string
		if log.Other != "" {
			otherMap, _ := common.StrToMap(log.Other)
			if otherMap != nil {
				anthropicSemantic = model.IsAnthropicLog(otherMap)
				cacheReadTokens = int64Value(otherMap, "cache_tokens")
				cacheCreationTokens = model.CacheCreationTotalFromLog(otherMap)
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

		cacheTokens := cacheReadTokens + cacheCreationTokens
		// effectivePromptTokens is the dashboard-aligned total input used for
		// the tokens/users sums: anthropic-semantic rows add the separately
		// stored cache, openai-semantic rows already carry it in prompt_tokens.
		effectivePromptTokens := promptTokens
		if anthropicSemantic {
			effectivePromptTokens += cacheTokens
		}
		totalTokens := effectivePromptTokens + completionTokens
		data.Tokens += totalTokens

		// Users aggregation
		if data.Users[username] == nil {
			data.Users[username] = make(map[string]int64)
		}
		data.Users[username][modelName] += totalTokens

		// recordPrompt excludes the cached portion for every log kind so all
		// exported records share one prompt/cache meaning.
		recordPrompt := promptTokens
		if !anthropicSemantic {
			recordPrompt -= cacheTokens
			if recordPrompt < 0 {
				recordPrompt = 0
			}
		}

		record := logExportRecord{
			Username:   username,
			Model:      modelName,
			Time:       time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339),
			Prompt:     recordPrompt,
			Completion: completionTokens,
			Cache:      cacheTokens,
		}
		if typeLabel != "" {
			record.Types = []string{typeLabel}
		}
		data.Details = append(data.Details, record)
	}

	return data
}

// int64Value extracts an integer value from a JSON-decoded map. It returns 0
// for missing or non-numeric values. encoding/json decodes all numbers as
// float64, so one assertion covers every numeric case.
func int64Value(m map[string]interface{}, key string) int64 {
	return model.LogOtherInt(m, key)
}
