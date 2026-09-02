package setting

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	AccessSourceLimitEnabledOptionKey           = "AccessSourceLimitEnabled"
	AccessSourceAssociationWindowHoursOptionKey = "AccessSourceAssociationWindowHours"
	AccessSourceMaxIPsPerUserOptionKey          = "AccessSourceMaxIPsPerUser"
	AccessSourceSwitchCooldownMinutesOptionKey  = "AccessSourceSwitchCooldownMinutes"
	AccessSourceMaxUsersPerIPOptionKey          = "AccessSourceMaxUsersPerIP"
	MaxAccessSourceAssociationWindowHours       = 7 * 24
	MaxAccessSourceSwitchCooldownMinutes        = 24 * 60
	MaxAccessSourceAssociations                 = 1000
)

var AccessSourceLimitEnabled = false
var AccessSourceAssociationWindowHours = 24
var AccessSourceMaxIPsPerUser = 0
var AccessSourceSwitchCooldownMinutes = 0
var AccessSourceMaxUsersPerIP = 0

type AccessSourceLimits struct {
	Enabled                bool
	AssociationWindowHours int
	MaxIPsPerUser          int
	SwitchCooldownMinutes  int
	MaxUsersPerIP          int
	MaxIPsPerUserSource    string
	SwitchCooldownSource   string
}

func CheckAccessSourceAssociationWindowHours(value string) error {
	hours, err := strconv.Atoi(value)
	if err != nil || hours < 1 || hours > MaxAccessSourceAssociationWindowHours {
		return fmt.Errorf("访问来源关联统计周期必须为 1 至 %d 小时的整数", MaxAccessSourceAssociationWindowHours)
	}
	return nil
}

func CheckAccessSourceAssociationCount(value string) error {
	count, err := strconv.Atoi(value)
	if err != nil || count < 0 || count > MaxAccessSourceAssociations {
		return fmt.Errorf("访问来源关联数量必须为 0 至 %d 的整数", MaxAccessSourceAssociations)
	}
	return nil
}

func CheckAccessSourceSwitchCooldownMinutes(value string) error {
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes < 0 || minutes > MaxAccessSourceSwitchCooldownMinutes {
		return fmt.Errorf("访问来源切换等待时间必须为 0 至 %d 分钟的整数", MaxAccessSourceSwitchCooldownMinutes)
	}
	return nil
}

// ResolveAccessSourceLimits applies user overrides without overloading zero, which means unlimited.
func ResolveAccessSourceLimits(userSetting dto.UserSetting) AccessSourceLimits {
	limits := AccessSourceLimits{
		Enabled:                AccessSourceLimitEnabled,
		AssociationWindowHours: AccessSourceAssociationWindowHours,
		MaxIPsPerUser:          AccessSourceMaxIPsPerUser,
		SwitchCooldownMinutes:  AccessSourceSwitchCooldownMinutes,
		MaxUsersPerIP:          AccessSourceMaxUsersPerIP,
		MaxIPsPerUserSource:    RateLimitSourceGlobal,
		SwitchCooldownSource:   RateLimitSourceGlobal,
	}
	if value := userSetting.AccessSourceMaxIPs; value != nil && *value >= 0 && *value <= MaxAccessSourceAssociations {
		limits.MaxIPsPerUser = *value
		limits.MaxIPsPerUserSource = RateLimitSourceUser
	}
	if value := userSetting.AccessSourceSwitchCooldownMinutes; value != nil && *value >= 0 && *value <= MaxAccessSourceSwitchCooldownMinutes {
		limits.SwitchCooldownMinutes = *value
		limits.SwitchCooldownSource = RateLimitSourceUser
	}
	return limits
}
