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
	// 环境变量仅在启动时解析一次；运行期配置由管理面板经 GlobalConfig 持久化修改。
	if enabled, ok := os.LookupEnv("LOG_AUTO_EXPORT_ENABLED"); ok {
		if parsed, err := strconv.ParseBool(enabled); err == nil {
			logExportSetting.Enabled = parsed
		}
	}
	config.GlobalConfig.Register("log_export_setting", &logExportSetting)
}

func GetLogExportSetting() *LogExportSetting {
	return &logExportSetting
}
