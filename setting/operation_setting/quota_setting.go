package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type QuotaSetting struct {
	EnableFreeModelPreConsume      bool   `json:"enable_free_model_pre_consume"`     // 是否对免费模型启用预消耗
	AutoExecuteQuotaInitialization bool   `json:"auto_execute_quota_initialization"` // 是否自动执行系统生成的初始化方案
	QuotaInitializationTime        string `json:"quota_initialization_time"`         // 初始化草稿自动生成时间（上海时区 HH:mm）
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume:      true,
	AutoExecuteQuotaInitialization: false,
	QuotaInitializationTime:        "09:00",
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("quota_setting", &quotaSetting)
}

func GetQuotaSetting() *QuotaSetting {
	return &quotaSetting
}
