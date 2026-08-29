package operation_setting

import (
	"fmt"
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

type QuotaSetting struct {
	EnableFreeModelPreConsume bool            `json:"enable_free_model_pre_consume"` // 是否对免费模型启用预消耗
	TrustQuotaUSD             float64         `json:"trust_quota_usd"`               // 钱包免预扣门槛，0 表示禁用
	PreConsumeMultiplier      float64         `json:"pre_consume_multiplier"`        // 预计输入费用的预扣倍率，仅影响预留
	SettlementLeadMinutes     int             `json:"settlement_lead_minutes"`       // 周期结束前进入结算的分钟数
	SettlementPrompt          string          `json:"settlement_prompt"`             // 结算期间拒绝新请求的提示语
	TemporaryQuotaProjects    map[string]bool `json:"temporary_quota_projects"`      // 临时额度申请项目及可选状态
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
	TrustQuotaUSD:             10,
	PreConsumeMultiplier:      1,
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

// ValidateQuotaOption validates reservation settings before they are persisted.
func ValidateQuotaOption(key, value string) error {
	if key != "quota_setting.trust_quota_usd" && key != "quota_setting.pre_consume_multiplier" {
		return nil
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return fmt.Errorf("%s must be a finite non-negative number", key)
	}
	if key == "quota_setting.pre_consume_multiplier" && number == 0 {
		return fmt.Errorf("%s must be greater than zero", key)
	}
	return nil
}

// InputPreConsumeMultiplier rejects invalid runtime settings as well as invalid saves.
func InputPreConsumeMultiplier() (float64, error) {
	multiplier := quotaSetting.PreConsumeMultiplier
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, fmt.Errorf("pre-consume multiplier must be a finite number greater than zero")
	}
	return multiplier, nil
}
