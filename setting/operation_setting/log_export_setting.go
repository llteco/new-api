package operation_setting

import (
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

// LogExportSetting 配置日志自动导出功能
type LogExportSetting struct {
	// 是否启用自动导出
	Enabled bool `json:"enabled"`
	// 检查间隔（分钟）
	IntervalMinutes int `json:"interval_minutes"`
	// 星期几触发（0=周日，1=周一...5=周五）
	Weekday int `json:"weekday"`
	// 触发小时（0-23）
	Hour int `json:"hour"`
	// 触发分钟（0-59）
	Minute int `json:"minute"`
	// 导出时间范围（天）
	DurationDays int `json:"duration_days"`
	// 导出目录，留空使用默认日志目录
	OutputDir string `json:"output_dir"`
}

// 默认配置：每周五 18:00 导出过去 7 天的日志
var logExportSetting = LogExportSetting{
	Enabled:         true,
	IntervalMinutes: 60,
	Weekday:         5,
	Hour:            18,
	Minute:          0,
	DurationDays:    7,
	OutputDir:       "",
}

func init() {
	config.GlobalConfig.Register("log_export_setting", &logExportSetting)
}

func GetLogExportSetting() *LogExportSetting {
	// 支持环境变量覆盖
	if enabled, ok := os.LookupEnv("LOG_AUTO_EXPORT_ENABLED"); ok {
		if parsed, err := strconv.ParseBool(enabled); err == nil {
			logExportSetting.Enabled = parsed
		}
	}
	if interval := os.Getenv("LOG_AUTO_EXPORT_INTERVAL_MINUTES"); interval != "" {
		if parsed, err := strconv.Atoi(interval); err == nil && parsed > 0 {
			logExportSetting.IntervalMinutes = parsed
		}
	}
	if weekday := os.Getenv("LOG_AUTO_EXPORT_WEEKDAY"); weekday != "" {
		if parsed, err := strconv.Atoi(weekday); err == nil && parsed >= 0 && parsed <= 6 {
			logExportSetting.Weekday = parsed
		}
	}
	if hour := os.Getenv("LOG_AUTO_EXPORT_HOUR"); hour != "" {
		if parsed, err := strconv.Atoi(hour); err == nil && parsed >= 0 && parsed <= 23 {
			logExportSetting.Hour = parsed
		}
	}
	if minute := os.Getenv("LOG_AUTO_EXPORT_MINUTE"); minute != "" {
		if parsed, err := strconv.Atoi(minute); err == nil && parsed >= 0 && parsed <= 59 {
			logExportSetting.Minute = parsed
		}
	}
	if days := os.Getenv("LOG_AUTO_EXPORT_DURATION_DAYS"); days != "" {
		if parsed, err := strconv.Atoi(days); err == nil && parsed > 0 {
			logExportSetting.DurationDays = parsed
		}
	}
	if dir := os.Getenv("LOG_AUTO_EXPORT_OUTPUT_DIR"); dir != "" {
		logExportSetting.OutputDir = dir
	}
	return &logExportSetting
}
