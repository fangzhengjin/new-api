package model

import "github.com/QuantumNous/new-api/setting/operation_setting"

const CycleQuotaManagementOptionKey = "CycleQuotaManagementEnabled"

func CycleQuotaManagementEnabled() bool {
	return operation_setting.CycleQuotaManagementEnabled
}
