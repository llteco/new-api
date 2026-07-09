// rebuild_quota_data rebuilds quota_data rows from consume logs so that
// Anthropic/Claude cache read and cache creation tokens are included in the
// aggregated token counts. It deletes existing quota_data rows in the given
// time window and re-aggregates them from logs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type logRow struct {
	UserID           int    `gorm:"column:user_id"`
	Username         string `gorm:"column:username"`
	ModelName        string `gorm:"column:model_name"`
	UseGroup         string `gorm:"column:group"`
	TokenID          int    `gorm:"column:token_id"`
	ChannelID        int    `gorm:"column:channel_id"`
	CreatedAt        int64  `gorm:"column:created_at"`
	PromptTokens     int    `gorm:"column:prompt_tokens"`
	CompletionTokens int    `gorm:"column:completion_tokens"`
	Quota            int    `gorm:"column:quota"`
	Other            string `gorm:"column:other"`
}

func main() {
	startStr := flag.String("start", "2026-07-01T00:00:00Z", "start time (RFC3339)")
	endStr := flag.String("end", time.Now().UTC().Format(time.RFC3339), "end time (RFC3339)")
	dryRun := flag.Bool("dry-run", false, "print what would be done without writing")
	confirm := flag.Bool("confirm", false, "confirm deletion and rebuild")
	flag.Parse()

	start, err := time.Parse(time.RFC3339, *startStr)
	if err != nil {
		log.Fatalf("invalid start time: %v", err)
	}
	end, err := time.Parse(time.RFC3339, *endStr)
	if err != nil {
		log.Fatalf("invalid end time: %v", err)
	}

	if err := model.InitDB(); err != nil {
		log.Fatalf("init main db failed: %v", err)
	}
	if err := model.InitLogDB(); err != nil {
		log.Fatalf("init log db failed: %v", err)
	}

	startTs, endTs := start.Unix(), end.Unix()

	var logCount int64
	if err := model.LOG_DB.Table("logs").
		Where("type = ? AND created_at >= ? AND created_at <= ?", model.LogTypeConsume, startTs, endTs).
		Count(&logCount).Error; err != nil {
		log.Fatalf("count logs failed: %v", err)
	}

	fmt.Printf("Time range: %s UTC -> %s UTC\n", start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))
	fmt.Printf("Consume logs in range: %d\n", logCount)

	if *dryRun {
		fmt.Println("dry-run: no changes made")
		return
	}

	if !*confirm {
		fmt.Println("WARNING: this will delete and rebuild quota_data rows in the time range.")
		fmt.Println("Run with -confirm to proceed, or -dry-run to preview.")
		os.Exit(1)
	}

	var deleted int64
	result := model.DB.Table("quota_data").
		Where("created_at >= ? AND created_at <= ?", startTs, endTs).
		Delete(&model.QuotaData{})
	if result.Error != nil {
		log.Fatalf("delete quota_data failed: %v", result.Error)
	}
	deleted = result.RowsAffected
	fmt.Printf("Deleted %d existing quota_data rows\n", deleted)

	var rows []logRow
	if err := model.LOG_DB.Table("logs").
		Select("user_id, username, model_name, `group`, token_id, channel_id, created_at, prompt_tokens, completion_tokens, quota, other").
		Where("type = ? AND created_at >= ? AND created_at <= ?", model.LogTypeConsume, startTs, endTs).
		Scan(&rows).Error; err != nil {
		log.Fatalf("scan logs failed: %v", err)
	}

	aggregates := make(map[string]*model.QuotaData)
	for _, r := range rows {
		effectivePrompt, err := effectivePromptTokens(r.PromptTokens, r.Other)
		if err != nil {
			log.Printf("parse other failed for log user=%d model=%s: %v", r.UserID, r.ModelName, err)
			effectivePrompt = r.PromptTokens
		}
		tokenUsed := effectivePrompt + r.CompletionTokens
		hour := r.CreatedAt - (r.CreatedAt % 3600)

		key := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s\x00%d\x00%d\x00%s",
			r.UserID, r.Username, r.ModelName, hour, r.UseGroup, r.TokenID, r.ChannelID, common.NodeName)

		if existing, ok := aggregates[key]; ok {
			existing.Count++
			existing.Quota += r.Quota
			existing.TokenUsed += tokenUsed
			continue
		}
		aggregates[key] = &model.QuotaData{
			UserID:    r.UserID,
			Username:  r.Username,
			ModelName: r.ModelName,
			CreatedAt: hour,
			UseGroup:  r.UseGroup,
			TokenID:   r.TokenID,
			ChannelID: r.ChannelID,
			NodeName:  common.NodeName,
			Count:     1,
			Quota:     r.Quota,
			TokenUsed: tokenUsed,
		}
	}

	inserted := 0
	for _, qd := range aggregates {
		if err := model.DB.Table("quota_data").Create(qd).Error; err != nil {
			log.Printf("insert quota_data failed: %v", err)
			continue
		}
		inserted++
	}
	fmt.Printf("Inserted %d quota_data rows\n", inserted)
}

func effectivePromptTokens(promptTokens int, otherJSON string) (int, error) {
	if otherJSON == "" {
		return promptTokens, nil
	}
	var other map[string]interface{}
	if err := json.Unmarshal([]byte(otherJSON), &other); err != nil {
		return promptTokens, err
	}
	if !isAnthropic(other) {
		return promptTokens, nil
	}
	cacheTokens := jsonInt(other, "cache_tokens")
	cacheCreation := cacheCreationTotal(other)
	return promptTokens + cacheTokens + cacheCreation, nil
}

func isAnthropic(other map[string]interface{}) bool {
	if us, ok := other["usage_semantic"].(string); ok && us == "anthropic" {
		return true
	}
	if c, ok := other["claude"].(bool); ok && c {
		return true
	}
	return false
}

func cacheCreationTotal(other map[string]interface{}) int {
	aggregate := jsonInt(other, "cache_creation_tokens")
	split5m := jsonInt(other, "cache_creation_tokens_5m")
	split1h := jsonInt(other, "cache_creation_tokens_1h")
	if split5m > 0 || split1h > 0 {
		splitTotal := split5m + split1h
		if aggregate > splitTotal {
			return aggregate
		}
		return splitTotal
	}
	return aggregate
}

func jsonInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

// suppress unused gorm import warning if the build flags prune it
var _ = gorm.Expr
