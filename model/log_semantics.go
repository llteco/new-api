package model

// LogOtherTokens extracts Anthropic-semantic token fields from a decoded
// logs.other map. Shared by log auto-export and quota_data rebuild so both
// aggregate identically with the SQL expressions in log_stats.go.

// IsAnthropicLog returns true when the log row stores usage in Anthropic
// semantics (prompt_tokens excludes cache): anthropic-native requests and
// requests converted to an anthropic upstream. It checks both the newer
// usage_semantic marker and the legacy claude flag so historical logs are
// handled correctly.
func IsAnthropicLog(other map[string]interface{}) bool {
	if us, ok := other["usage_semantic"].(string); ok && us == "anthropic" {
		return true
	}
	if c, ok := other["claude"].(bool); ok && c {
		return true
	}
	return false
}

// LogOtherInt extracts an integer value from a JSON-decoded map. It returns 0
// for missing or non-numeric values. encoding/json decodes all numbers as
// float64, so one assertion covers every numeric case.
func LogOtherInt(other map[string]interface{}, key string) int64 {
	if f, ok := other[key].(float64); ok {
		return int64(f)
	}
	return 0
}

// CacheCreationTotalFromLog returns the total cache-creation tokens recorded
// in a log row, matching the logic used when the log was written: prefer the
// aggregate value when it covers the split values, otherwise use the sum of
// the split values. The 5m/1h split fields only exist on anthropic-semantic
// rows; other rows simply yield their aggregate (usually zero).
func CacheCreationTotalFromLog(other map[string]interface{}) int64 {
	aggregate := LogOtherInt(other, "cache_creation_tokens")
	split5m := LogOtherInt(other, "cache_creation_tokens_5m")
	split1h := LogOtherInt(other, "cache_creation_tokens_1h")
	if split5m > 0 || split1h > 0 {
		splitTotal := split5m + split1h
		if aggregate > splitTotal {
			return aggregate
		}
		return splitTotal
	}
	return aggregate
}
