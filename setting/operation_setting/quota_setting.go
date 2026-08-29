package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type QuotaSetting struct {
	EnableFreeModelPreConsume bool            `json:"enable_free_model_pre_consume"` // 是否对免费模型启用预消耗
	SettlementLeadMinutes     int             `json:"settlement_lead_minutes"`       // 周期结束前进入结算的分钟数
	SettlementPrompt          string          `json:"settlement_prompt"`             // 结算期间拒绝新请求的提示语
	TemporaryQuotaProjects    map[string]bool `json:"temporary_quota_projects"`      // 临时额度申请项目及可选状态
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
	SettlementLeadMinutes:     10,
	SettlementPrompt:          "本期额度正在结算，暂时无法发起新请求，请稍后重试",
	TemporaryQuotaProjects:    map[string]bool{},
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("quota_setting", &quotaSetting)
}

func GetQuotaSetting() *QuotaSetting {
	return &quotaSetting
}
