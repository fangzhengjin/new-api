package setting

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// maxRateLimitDurationSeconds is the largest window the count cap is computed
// against (24h). Token-bucket capacity is count*duration; this keeps that
// product inside int64 when the window is at most a day.
const maxRateLimitDurationSeconds = 24 * 60 * 60

// maxModelRequestRateLimitCount is math.MaxInt64 / maxRateLimitDurationSeconds.
// It is the largest count that cannot overflow int64(count)*duration for a
// window of at most 24 hours.
const maxModelRequestRateLimitCount int64 = math.MaxInt64 / maxRateLimitDurationSeconds

const MaxModelRequestRateLimitCount = maxModelRequestRateLimitCount

const (
	RateLimitSourceGlobal = "global"
	RateLimitSourceGroup  = "group"
	RateLimitSourceUser   = "user"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestIPRateLimitCount = 0
var ModelRequestIPRateLimitSuccessCount = 0
var ModelRequestRateLimitSuccessCount = 0
var ModelRequestRateLimitGroup = map[string][2]int{}
var ModelRequestRateLimitMutex sync.RWMutex

// CheckModelRequestRateLimitCount validates a persisted model request count limit.
func CheckModelRequestRateLimitCount(value string, minCount int) error {
	count, err := strconv.Atoi(value)
	if err != nil || count < minCount || int64(count) > maxModelRequestRateLimitCount {
		return fmt.Errorf("模型请求数限制必须为 %d 至 %d 的整数", minCount, maxModelRequestRateLimitCount)
	}
	return nil
}

// CheckModelRequestRateLimitDurationMinutes validates the shared model request limit window.
func CheckModelRequestRateLimitDurationMinutes(value string) error {
	duration, err := strconv.Atoi(value)
	if err != nil || duration < 1 || duration > maxRateLimitDurationSeconds/60 {
		return fmt.Errorf("模型请求限流周期必须为 1 至 %d 分钟的整数", maxRateLimitDurationSeconds/60)
	}
	return nil
}

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	updated := make(map[string][2]int)
	if err := common.Unmarshal([]byte(jsonStr), &updated); err != nil {
		return err
	}
	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()

	ModelRequestRateLimitGroup = updated
	return nil
}

func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

type ModelRequestRateLimits struct {
	TotalCount    int
	SuccessCount  int
	TotalSource   string
	SuccessSource string
}

// ResolveModelRequestRateLimits applies the persisted precedence user → group → global.
func ResolveModelRequestRateLimits(group string, userSetting dto.UserSetting) ModelRequestRateLimits {
	limits := ModelRequestRateLimits{
		TotalCount:    ModelRequestRateLimitCount,
		SuccessCount:  ModelRequestRateLimitSuccessCount,
		TotalSource:   RateLimitSourceGlobal,
		SuccessSource: RateLimitSourceGlobal,
	}
	if totalCount, successCount, found := GetGroupRateLimit(group); found {
		limits.TotalCount = totalCount
		limits.SuccessCount = successCount
		limits.TotalSource = RateLimitSourceGroup
		limits.SuccessSource = RateLimitSourceGroup
	}
	if value := userSetting.ModelRequestRateLimitCount; value != nil && *value >= 0 && int64(*value) <= MaxModelRequestRateLimitCount {
		limits.TotalCount = *value
		limits.TotalSource = RateLimitSourceUser
	}
	if value := userSetting.ModelRequestRateLimitSuccessCount; value != nil && *value >= 0 && int64(*value) <= MaxModelRequestRateLimitCount {
		limits.SuccessCount = *value
		limits.SuccessSource = RateLimitSourceUser
	}
	return limits
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	checkModelRequestRateLimitGroup := make(map[string][2]int)
	err := common.Unmarshal([]byte(jsonStr), &checkModelRequestRateLimitGroup)
	if err != nil {
		return err
	}
	for group, limits := range checkModelRequestRateLimitGroup {
		if limits[0] < 0 || limits[1] < 0 {
			return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", group, limits[0], limits[1])
		}
		if int64(limits[0]) > maxModelRequestRateLimitCount || int64(limits[1]) > maxModelRequestRateLimitCount {
			return fmt.Errorf("group %s [%d, %d] exceeds max rate limit %d", group, limits[0], limits[1], maxModelRequestRateLimitCount)
		}
	}

	return nil
}
