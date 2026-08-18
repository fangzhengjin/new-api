// Package quota implements the quota-cycle allocation workflow imported from ai-quota-tool.
package quota

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const (
	// AlgorithmVersion rejects stale drafts after allocation rules change.
	LegacyAlgorithmVersion          = "1.8.0"
	AlgorithmVersion                = LegacyAlgorithmVersion
	CandidateAlgorithmVersion       = "2.0.0"
	defaultQuotaPerUnit       int64 = 500_000
	daySeconds                int64 = 86_400

	exhaustionIdleSeconds        = daySeconds
	minimumObservationWorkdays   = 3
	initialUsageBufferPercent    = 120
	initialStabilityFloorPercent = 80
)

func cycleAlgorithmVersion(cycle *model.QuotaCycle) string {
	if cycle != nil && cycle.AllocationAlgorithmVersion == CandidateAlgorithmVersion {
		return CandidateAlgorithmVersion
	}
	return LegacyAlgorithmVersion
}

func cycleAllocationMode(cycle *model.QuotaCycle) string {
	if cycleAlgorithmVersion(cycle) == CandidateAlgorithmVersion {
		return allocationCandidate
	}
	return allocationCurrent
}

var (
	errQuotaOverflow = errors.New("quota calculation exceeds int64")
	shanghaiLocation = mustLoadShanghaiLocation()
)

func mustLoadShanghaiLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return location
}

func minQuota(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxQuota(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func quotaPerUnit() int64 {
	unit := int64(common.QuotaRound(common.QuotaPerUnit))
	if unit <= 0 {
		return defaultQuotaPerUnit
	}
	return unit
}

func quotaPerCent() int64 {
	return maxQuota(1, quotaPerUnit()/100)
}

func checkedAdd(left int64, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, errQuotaOverflow
	}
	return left + right, nil
}

func checkedSum(values ...int64) (int64, error) {
	var total int64
	for _, value := range values {
		var err error
		total, err = checkedAdd(total, value)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func bigRatio(numerators []int64, denominators []int64, roundUp bool) (int64, error) {
	numerator := big.NewInt(1)
	for _, value := range numerators {
		if value < 0 {
			return 0, errors.New("quota ratio numerator cannot be negative")
		}
		numerator.Mul(numerator, big.NewInt(value))
	}
	denominator := big.NewInt(1)
	for _, value := range denominators {
		if value <= 0 {
			return 0, errors.New("quota ratio denominator must be positive")
		}
		denominator.Mul(denominator, big.NewInt(value))
	}
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if roundUp && remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, errQuotaOverflow
	}
	return quotient.Int64(), nil
}

func ceilDiv(value int64, divisor int64) (int64, error) {
	return bigRatio([]int64{value}, []int64{divisor}, true)
}

func roundUpCent(quota int64) (int64, error) {
	step := quotaPerCent()
	cents, err := ceilDiv(quota, step)
	if err != nil {
		return 0, err
	}
	return bigRatio([]int64{cents, step}, nil, false)
}

func roundDownCent(quota int64) int64 {
	step := quotaPerCent()
	return quota / step * step
}

func regularStageCap(budgetQuota int64, recoveryReserveQuota int64, stagePercent int) (int64, error) {
	stageCap, err := bigRatio([]int64{budgetQuota, int64(stagePercent)}, []int64{10_000}, false)
	if err != nil {
		return 0, err
	}
	if recoveryReserveQuota > stageCap {
		return 0, errors.New("小额恢复池超过当前普通调配阶段上限")
	}
	return stageCap - recoveryReserveQuota, nil
}

// ParsePositiveQuota validates a positive raw quota value supplied as a decimal string.
func ParsePositiveQuota(value string, label string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	quota, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || quota <= 0 {
		return 0, fmt.Errorf("%s必须是正整数额度", label)
	}
	return quota, nil
}

func ParseNonNegativeQuota(value string, label string) (int64, error) {
	parsed, err := ParseNonNegativeQuotaTotal(value, label)
	if err != nil || parsed > int64(common.MaxQuota) {
		return 0, fmt.Errorf("%s必须是0至%d之间的整数", label, common.MaxQuota)
	}
	return parsed, nil
}

// ParseNonNegativeQuotaTotal validates an aggregate quota amount without a per-user ceiling.
func ParseNonNegativeQuotaTotal(value string, label string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s必须是非负整数额度", label)
	}
	return parsed, nil
}

// FormatQuota renders raw quota with the site's configured new-api quota display.
func FormatQuota(quota int64) string {
	return logger.FormatQuota(int(quota))
}

func dayStart(timestamp int64) time.Time {
	local := time.Unix(timestamp, 0).In(shanghaiLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghaiLocation)
}

func localDayAt(timestamp int64, offsetDays int, hour int) int64 {
	day := dayStart(timestamp).AddDate(0, 0, offsetDays)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, shanghaiLocation).Unix()
}

func isWeekday(timestamp int64) bool {
	weekday := dayStart(timestamp).Weekday()
	return weekday != time.Saturday && weekday != time.Sunday
}

// CountWeekdays counts Monday through Friday before the end date.
func CountWeekdays(start int64, end int64, skipStart bool) int {
	day := dayStart(start)
	if skipStart {
		day = day.AddDate(0, 0, 1)
	}
	endDay := dayStart(end)
	count := 0
	for day.Before(endDay) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			count++
		}
		day = day.AddDate(0, 0, 1)
	}
	return count
}

func nextMondayAfter(timestamp int64) int64 {
	result := localDayAt(timestamp, 1, 9)
	for dayStart(result).Weekday() != time.Monday {
		result = localDayAt(result, 1, 9)
	}
	return result
}

func finalAdjustmentTime(cycleStart int64, cycleEnd int64) int64 {
	result := localDayAt(cycleEnd, -1, 9)
	for result > cycleStart && (!isWeekday(result) || CountWeekdays(result, cycleEnd, true) < 3) {
		result = localDayAt(result, -1, 9)
	}
	return maxQuota(result, cycleStart)
}

// ScheduleEvent is one recommended cumulative release milestone.
type ScheduleEvent struct {
	Time    int64  `json:"time"`
	Percent int    `json:"percent"`
	Label   string `json:"label"`
}

// Schedule contains the current and next recommended release milestones.
type Schedule struct {
	Events  []ScheduleEvent `json:"events"`
	Current ScheduleEvent   `json:"current"`
	Next    ScheduleEvent   `json:"next"`
	Note    string          `json:"note"`
}

// RecommendSchedule reproduces the source tool's Monday and final-window schedule.
func RecommendSchedule(cycleStart int64, cycleEnd int64, snapshot int64) Schedule {
	firstMonday := nextMondayAfter(cycleStart)
	regular := make([]ScheduleEvent, 0, 4)
	for index, percent := range []int{75, 85, 95} {
		eventTime := localDayAt(firstMonday, index*7, 9)
		if eventTime < cycleEnd {
			regular = append(regular, ScheduleEvent{
				Time: eventTime, Percent: percent, Label: fmt.Sprintf("第%d次调配", index+1),
			})
		}
	}
	finalTime := finalAdjustmentTime(cycleStart, cycleEnd)
	if len(regular) > 0 {
		last := &regular[len(regular)-1]
		if finalTime <= last.Time || CountWeekdays(last.Time, finalTime, false) <= 2 {
			last.Time = minQuota(last.Time, finalTime)
			last.Percent = 100
			last.Label = "第三次调配与最终调配合并"
		} else if finalTime < cycleEnd {
			regular = append(regular, ScheduleEvent{Time: finalTime, Percent: 100, Label: "最终调配"})
		}
	} else if finalTime < cycleEnd {
		regular = append(regular, ScheduleEvent{Time: finalTime, Percent: 100, Label: "最终调配"})
	}
	events := append([]ScheduleEvent{{Time: cycleStart, Percent: 0, Label: "初始化"}}, regular...)
	sort.Slice(events, func(left int, right int) bool {
		if events[left].Time == events[right].Time {
			return events[left].Percent < events[right].Percent
		}
		return events[left].Time < events[right].Time
	})
	current := events[0]
	for _, event := range events {
		if event.Time <= snapshot && event.Percent >= current.Percent {
			current = event
		}
	}
	next := ScheduleEvent{Time: cycleEnd, Percent: current.Percent, Label: "周期结束"}
	for _, event := range events {
		if event.Time > snapshot {
			next = event
			break
		}
	}
	note := "按自然周一和剩余使用窗口推荐"
	switch {
	case snapshot < cycleStart:
		note = "当前快照早于周期开始"
	case snapshot >= cycleEnd:
		note = "当前快照晚于周期结束"
	case current.Percent == next.Percent:
		note = "当前已是最终阶段"
	}
	return Schedule{Events: events, Current: current, Next: next, Note: note}
}

// NextAdjustmentForStage finds the next milestone after a manually selected stage.
func NextAdjustmentForStage(events []ScheduleEvent, selectedPercent int, currentTime int64, cycleEnd int64) ScheduleEvent {
	for _, event := range events {
		if event.Percent > selectedPercent && event.Time > currentTime {
			return event
		}
	}
	return ScheduleEvent{Time: cycleEnd, Percent: selectedPercent, Label: "周期结束"}
}

// StageName returns the source tool's Chinese stage name for persisted audit text.
func StageName(percent int) string {
	switch percent {
	case 0:
		return "初始化"
	case 75:
		return "第一次调配"
	case 85:
		return "第二次调配"
	case 95:
		return "第三次调配"
	case 100:
		return "最终调配"
	default:
		return "自定义调配阶段"
	}
}

// DemandProfile is the normalized seven-day demand used by every allocation rule.
type DemandProfile struct {
	PeriodWeek         int64
	RecentWeek         int64
	ExhaustionWeek     int64
	WeeklyDemand       int64
	Source             string
	SuspectedExhausted bool
}

func recommendInitialGrant(totalSpend int64, userCount int, previousInitial int64) (int64, error) {
	if totalSpend <= 0 || userCount <= 0 {
		return previousInitial, nil
	}
	average, err := ceilDiv(totalSpend, int64(userCount))
	if err != nil {
		return 0, err
	}
	buffered, err := bigRatio([]int64{average, initialUsageBufferPercent}, []int64{100}, false)
	if err != nil {
		return 0, err
	}
	buffered, err = roundUpCent(buffered)
	if err != nil {
		return 0, err
	}
	stabilityFloor, err := bigRatio([]int64{previousInitial, initialStabilityFloorPercent}, []int64{100}, false)
	if err != nil {
		return 0, err
	}
	stabilityFloor, err = roundUpCent(stabilityFloor)
	if err != nil {
		return 0, err
	}
	return maxQuota(buffered, stabilityFloor), nil
}

func calculateDemandProfile(
	balance int64,
	periodSpend int64,
	recentSpend int64,
	lastUse int64,
	cycleStart int64,
	snapshot int64,
	createdAt int64,
	initialGrant int64,
) (DemandProfile, error) {
	elapsed := maxQuota(1, snapshot-cycleStart)
	periodWeek, err := bigRatio([]int64{periodSpend, 7, daySeconds}, []int64{elapsed}, true)
	if err != nil {
		return DemandProfile{}, err
	}

	availableStart := maxQuota(cycleStart, createdAt)
	var rawExhaustionWeek int64
	if periodSpend > 0 && lastUse > availableStart {
		rawExhaustionWeek, err = bigRatio(
			[]int64{periodSpend, 7, daySeconds},
			[]int64{lastUse - availableStart},
			true,
		)
		if err != nil {
			return DemandProfile{}, err
		}
	}

	suspectedExhausted := rawExhaustionWeek > 0 &&
		snapshot-lastUse >= exhaustionIdleSeconds &&
		balance <= 2*quotaPerUnit()
	exhaustionWeek := int64(0)
	if suspectedExhausted {
		exhaustionWeek = minQuota(rawExhaustionWeek, initialGrant)
	}
	ordinaryDemand := maxQuota(periodWeek, recentSpend)
	weeklyDemand := maxQuota(ordinaryDemand, exhaustionWeek)
	source := "period"
	if exhaustionWeek > ordinaryDemand {
		source = "exhaustion"
	} else if recentSpend >= periodWeek {
		source = "recent"
	}

	return DemandProfile{
		PeriodWeek:         periodWeek,
		RecentWeek:         recentSpend,
		ExhaustionWeek:     exhaustionWeek,
		WeeklyDemand:       weeklyDemand,
		Source:             source,
		SuspectedExhausted: suspectedExhausted,
	}, nil
}

func demandBasis(periodSpend int64, balance int64, profile DemandProfile, purpose string) string {
	if profile.Source == "exhaustion" {
		return fmt.Sprintf(
			"本期累计使用 %s，当前余额 %s；余额已接近用完，按额度可用期间折算每7日 %s 核定%s额度",
			FormatQuota(periodSpend), FormatQuota(balance), FormatQuota(profile.ExhaustionWeek), purpose,
		)
	}
	reference := fmt.Sprintf("本期累计使用 %s，折算每7日 %s", FormatQuota(periodSpend), FormatQuota(profile.PeriodWeek))
	if profile.Source == "recent" {
		reference = fmt.Sprintf("最近7日累计使用 %s", FormatQuota(profile.RecentWeek))
	}
	return fmt.Sprintf("%s；按本期折算7日用量与最近7日用量取较高值核定%s额度", reference, purpose)
}

func calculateUsageBuffer(target int64, weeklyDemand int64, initialGrant int64, maximumBasisPoints int64) (int64, error) {
	if target <= 0 || weeklyDemand <= 0 || initialGrant <= 0 || maximumBasisPoints <= 0 {
		return 0, nil
	}
	buffer, err := bigRatio(
		[]int64{target, maximumBasisPoints, minQuota(weeklyDemand, initialGrant)},
		[]int64{10_000, initialGrant},
		true,
	)
	if err != nil {
		return 0, err
	}
	return roundUpCent(buffer)
}

func hasObservationWindow(createdAt int64, snapshot int64) bool {
	return createdAt <= 0 || CountWeekdays(createdAt, snapshot, true) >= minimumObservationWorkdays
}

func targetForDays(weeklyDemand int64, basisMode string, calculationDaysHundred int64) (int64, error) {
	if basisMode == "week" {
		return weeklyDemand, nil
	}
	return bigRatio([]int64{weeklyDemand, calculationDaysHundred}, []int64{700}, true)
}

func predictedForRemainingWorkdays(weeklyDemand int64, remainingWorkdays int) (int64, error) {
	predicted, err := bigRatio([]int64{weeklyDemand, int64(remainingWorkdays)}, []int64{7}, true)
	if err != nil {
		return 0, err
	}
	return roundUpCent(predicted)
}

func retainedForRemainingWorkdays(baseRetained int64, weeklyDemand int64, remainingWorkdays int) (int64, error) {
	predicted, err := predictedForRemainingWorkdays(weeklyDemand, remainingWorkdays)
	if err != nil {
		return 0, err
	}
	return maxQuota(baseRetained, predicted), nil
}

func continuityTarget(weeklyDemand int64, initialGrant int64, totalWorkdays int) (int64, error) {
	if weeklyDemand <= 0 || totalWorkdays <= 0 {
		return 0, nil
	}
	oneDayTarget, err := ceilDiv(weeklyDemand, 7)
	if err != nil {
		return 0, err
	}
	minimumDayTarget, err := ceilDiv(initialGrant, int64(totalWorkdays))
	if err != nil {
		return 0, err
	}
	target, err := roundUpCent(maxQuota(oneDayTarget, minimumDayTarget))
	if err != nil {
		return 0, err
	}
	return target, nil
}

func continuityTopUp(balance int64, weeklyDemand int64, initialGrant int64, totalWorkdays int) (int64, error) {
	target, err := continuityTarget(weeklyDemand, initialGrant, totalWorkdays)
	if err != nil {
		return 0, err
	}
	return maxQuota(0, target-balance), nil
}

func fairTargets(weeklyDemand int64, initialGrant int64, totalWorkdays int, basisMode string, calculationDaysHundred int64) (int64, int64, int64, error) {
	safetyTarget, err := continuityTarget(weeklyDemand, initialGrant, totalWorkdays)
	if err != nil {
		return 0, 0, 0, err
	}
	demandTarget, err := targetForDays(weeklyDemand, basisMode, calculationDaysHundred)
	if err != nil {
		return 0, 0, 0, err
	}
	return safetyTarget, demandTarget, maxQuota(safetyTarget, demandTarget), nil
}

func availableIncreaseCap(stageCap int64, occupiedBefore int64, reclaimed int64) int64 {
	afterReclaim := occupiedBefore - reclaimed
	if afterReclaim < stageCap {
		return stageCap - afterReclaim
	}
	return 0
}

func isOrdinaryLowUsage(recentSpend int64, weeklyDemand int64, initialGrant int64) bool {
	return recentSpend == 0 || weeklyDemand <= initialGrant/10
}

func adjustmentStageNumber(stageBasisPoints int) int {
	switch stageBasisPoints {
	case 7_500:
		return 1
	case 8_500:
		return 2
	case 9_500:
		return 3
	case 10_000:
		return 4
	default:
		return 0
	}
}

func cumulativeReclaimPercent(stageBasisPoints int, perStagePercent int) int {
	return min(100, adjustmentStageNumber(stageBasisPoints)*perStagePercent)
}

func isMaterialAdjustment(amount int64, minimumAdjustment int64) bool {
	return amount > minimumAdjustment
}

type requestRow struct {
	UserID              int
	Requested           int64
	ContinuityRequested int64
	BonusRequested      int64
	Balance             int64
	SafetyTarget        int64
	Target              int64
	LastPositiveAt      int64
}

const fairnessCoverageScale int64 = 1_000_000_000

func allocateFairRequests(requests []requestRow, cap int64, minimumAdjustment int64) (map[int]int64, error) {
	allocations := make(map[int]int64)
	if len(requests) == 0 || cap <= 0 {
		return allocations, nil
	}
	cap = roundDownCent(cap)
	minimumAllocation, err := checkedAdd(minimumAdjustment, quotaPerCent())
	if err != nil || cap < minimumAllocation {
		return allocations, err
	}
	eligible := append([]requestRow(nil), requests...)
	used, err := waterFillFairLayer(eligible, allocations, cap, func(request requestRow) int64 {
		return request.SafetyTarget
	})
	if err != nil {
		return nil, err
	}
	if !fairLayerSatisfied(eligible, allocations, func(request requestRow) int64 { return request.SafetyTarget }) {
		return materializeFairAllocations(eligible, allocations, cap, minimumAdjustment)
	}
	_, err = waterFillFairLayer(eligible, allocations, cap-used, func(request requestRow) int64 {
		return request.Target
	})
	if err != nil {
		return nil, err
	}
	return materializeFairAllocations(eligible, allocations, cap, minimumAdjustment)
}

func waterFillFairLayer(requests []requestRow, allocations map[int]int64, cap int64, target func(requestRow) int64) (int64, error) {
	cap = roundDownCent(cap)
	if cap <= 0 {
		return 0, nil
	}
	requiredAt := func(level int64) (map[int]int64, int64, error) {
		result := make(map[int]int64)
		var total int64
		for _, request := range requests {
			layerTarget := target(request)
			current, err := checkedAdd(request.Balance, allocations[request.UserID])
			if err != nil || layerTarget <= 0 || current >= layerTarget {
				if err != nil {
					return nil, 0, err
				}
				continue
			}
			levelBalance, err := bigRatio([]int64{layerTarget, level}, []int64{fairnessCoverageScale}, true)
			if err != nil {
				return nil, 0, err
			}
			levelBalance = minQuota(layerTarget, levelBalance)
			if levelBalance <= current {
				continue
			}
			amount, err := roundUpCent(levelBalance - current)
			if err != nil {
				return nil, 0, err
			}
			maximum := request.Requested - allocations[request.UserID]
			amount = minQuota(amount, maximum)
			if amount <= 0 {
				continue
			}
			result[request.UserID] = amount
			total, err = checkedAdd(total, amount)
			if err != nil {
				return nil, 0, err
			}
		}
		return result, total, nil
	}

	low, high := int64(0), fairnessCoverageScale
	for low < high {
		middle := low + (high-low+1)/2
		_, required, err := requiredAt(middle)
		if err != nil {
			return 0, err
		}
		if required <= cap {
			low = middle
		} else {
			high = middle - 1
		}
	}
	increments, used, err := requiredAt(low)
	if err != nil {
		return 0, err
	}
	for userID, amount := range increments {
		allocations[userID], err = checkedAdd(allocations[userID], amount)
		if err != nil {
			return 0, err
		}
	}
	remaining := cap - used
	if remaining < quotaPerCent() {
		return used, nil
	}
	ordered := append([]requestRow(nil), requests...)
	sort.Slice(ordered, func(left int, right int) bool {
		return fairRecipientLess(ordered[left], allocations, target(ordered[left]), ordered[right], allocations, target(ordered[right]))
	})
	for _, request := range ordered {
		if remaining < quotaPerCent() {
			break
		}
		if allocations[request.UserID] >= request.Requested {
			continue
		}
		current, addErr := checkedAdd(request.Balance, allocations[request.UserID])
		if addErr != nil {
			return 0, addErr
		}
		if current >= target(request) {
			continue
		}
		allocations[request.UserID] += quotaPerCent()
		used += quotaPerCent()
		remaining -= quotaPerCent()
	}
	return used, nil
}

func fairRecipientLess(left requestRow, leftAllocations map[int]int64, leftTarget int64, right requestRow, rightAllocations map[int]int64, rightTarget int64) bool {
	leftBalance := new(big.Int).Add(big.NewInt(left.Balance), big.NewInt(leftAllocations[left.UserID]))
	rightBalance := new(big.Int).Add(big.NewInt(right.Balance), big.NewInt(rightAllocations[right.UserID]))
	comparison := new(big.Int).Mul(leftBalance, big.NewInt(maxQuota(1, rightTarget))).Cmp(
		new(big.Int).Mul(rightBalance, big.NewInt(maxQuota(1, leftTarget))),
	)
	if comparison != 0 {
		return comparison < 0
	}
	if left.LastPositiveAt != right.LastPositiveAt {
		if left.LastPositiveAt == 0 {
			return true
		}
		if right.LastPositiveAt == 0 {
			return false
		}
		return left.LastPositiveAt < right.LastPositiveAt
	}
	return left.UserID < right.UserID
}

func fairLayerSatisfied(requests []requestRow, allocations map[int]int64, target func(requestRow) int64) bool {
	for _, request := range requests {
		if request.Balance+allocations[request.UserID] < target(request) {
			return false
		}
	}
	return true
}

func materializeFairAllocations(requests []requestRow, raw map[int]int64, cap int64, minimumAdjustment int64) (map[int]int64, error) {
	allocations := make(map[int]int64, len(raw))
	for userID, amount := range raw {
		if isMaterialAdjustment(amount, minimumAdjustment) {
			allocations[userID] = amount
		}
	}
	used, err := allocationTotal(allocations)
	if err != nil {
		return nil, err
	}
	remaining := roundDownCent(cap - used)
	minimumAllocation, err := checkedAdd(minimumAdjustment, quotaPerCent())
	if err != nil {
		return nil, err
	}
	for remaining >= minimumAllocation {
		safetyCandidates := make([]requestRow, 0)
		targetCandidates := make([]requestRow, 0)
		safetyUnmet := false
		targetUnmet := false
		for _, request := range requests {
			current := request.Balance + allocations[request.UserID]
			if current < request.SafetyTarget {
				safetyUnmet = true
			}
			if current < request.Target {
				targetUnmet = true
			}
			if allocations[request.UserID] > 0 || request.Requested < minimumAllocation {
				continue
			}
			if request.Balance < request.SafetyTarget {
				safetyCandidates = append(safetyCandidates, request)
			}
			if request.Balance < request.Target {
				targetCandidates = append(targetCandidates, request)
			}
		}
		candidates := targetCandidates
		target := func(request requestRow) int64 { return request.Target }
		if safetyUnmet {
			candidates = safetyCandidates
			target = func(request requestRow) int64 { return request.SafetyTarget }
		}
		if !safetyUnmet && !targetUnmet || len(candidates) == 0 {
			break
		}
		sort.Slice(candidates, func(left int, right int) bool {
			return fairRecipientLess(candidates[left], allocations, target(candidates[left]), candidates[right], allocations, target(candidates[right]))
		})
		allocations[candidates[0].UserID] = minimumAllocation
		remaining -= minimumAllocation
	}

	funded := make([]requestRow, 0, len(allocations))
	for _, request := range requests {
		if allocations[request.UserID] > 0 {
			funded = append(funded, request)
		}
	}
	added, err := waterFillFairLayer(funded, allocations, remaining, func(request requestRow) int64 {
		return request.SafetyTarget
	})
	if err != nil {
		return nil, err
	}
	remaining -= added
	if !fairLayerSatisfied(requests, allocations, func(request requestRow) int64 { return request.SafetyTarget }) {
		return allocations, nil
	}
	_, err = waterFillFairLayer(funded, allocations, remaining, func(request requestRow) int64 {
		return request.Target
	})
	return allocations, err
}

type weightedRecipient struct {
	UserID int
	Weight int64
}

func allocateByWeight(recipients []weightedRecipient, amount int64, minimumAdjustment int64) (map[int]int64, error) {
	eligible := make([]weightedRecipient, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient.Weight > 0 {
			eligible = append(eligible, recipient)
		}
	}
	for len(eligible) > 0 {
		result, err := allocateByWeightOnce(eligible, amount)
		if err != nil {
			return nil, err
		}
		retained := eligible[:0]
		for _, recipient := range eligible {
			if result[recipient.UserID] > minimumAdjustment {
				retained = append(retained, recipient)
			}
		}
		if len(retained) == len(eligible) {
			return result, nil
		}
		eligible = retained
	}
	return map[int]int64{}, nil
}

func allocateByWeightOnce(recipients []weightedRecipient, amount int64) (map[int]int64, error) {
	result := make(map[int]int64)
	step := quotaPerCent()
	cents := roundDownCent(amount) / step
	if cents <= 0 {
		return result, nil
	}
	totalWeight := new(big.Int)
	for _, recipient := range recipients {
		totalWeight.Add(totalWeight, big.NewInt(recipient.Weight))
	}
	if totalWeight.Sign() <= 0 {
		return result, nil
	}
	type share struct {
		UserID    int
		Cents     int64
		Remainder *big.Int
	}
	shares := make([]share, 0, len(recipients))
	var allocatedCents int64
	for _, recipient := range recipients {
		product := new(big.Int).Mul(big.NewInt(recipient.Weight), big.NewInt(cents))
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(product, totalWeight, remainder)
		if !quotient.IsInt64() {
			return nil, errQuotaOverflow
		}
		shareCents := quotient.Int64()
		allocatedCents += shareCents
		shares = append(shares, share{UserID: recipient.UserID, Cents: shareCents, Remainder: remainder})
	}
	remaining := cents - allocatedCents
	sort.Slice(shares, func(left int, right int) bool {
		comparison := shares[left].Remainder.Cmp(shares[right].Remainder)
		if comparison == 0 {
			return shares[left].UserID < shares[right].UserID
		}
		return comparison > 0
	})
	for index := range shares {
		if remaining > 0 {
			shares[index].Cents++
			remaining--
		}
		if shares[index].Cents > 0 {
			result[shares[index].UserID] = shares[index].Cents * step
		}
	}
	return result, nil
}

func allocateProportionally(requests []requestRow, cap int64) (map[int]int64, error) {
	if len(requests) == 0 || cap <= 0 {
		return map[int]int64{}, nil
	}
	requested := make([]int64, 0, len(requests))
	recipients := make([]weightedRecipient, 0, len(requests))
	for _, request := range requests {
		requested = append(requested, request.Requested)
		recipients = append(recipients, weightedRecipient{UserID: request.UserID, Weight: request.Requested})
	}
	totalRequested, err := checkedSum(requested...)
	if err != nil {
		return nil, err
	}
	if totalRequested == 0 {
		return map[int]int64{}, nil
	}
	return allocateByWeight(recipients, minQuota(cap, totalRequested), 0)
}

func allocateWithContinuity(requests []requestRow, cap int64) (map[int]int64, error) {
	result := make(map[int]int64)
	if len(requests) == 0 || cap <= 0 {
		return result, nil
	}
	continuityRequests := make([]requestRow, 0, len(requests))
	for _, request := range requests {
		if request.ContinuityRequested > 0 {
			request.Requested = minQuota(request.Requested, request.ContinuityRequested)
			continuityRequests = append(continuityRequests, request)
		}
	}
	continuityValues := make([]int64, 0, len(continuityRequests))
	for _, request := range continuityRequests {
		continuityValues = append(continuityValues, request.Requested)
	}
	continuityTotal, err := checkedSum(continuityValues...)
	if err != nil {
		return nil, err
	}
	continuityAllocations, err := allocateProportionally(continuityRequests, minQuota(cap, continuityTotal))
	if err != nil {
		return nil, err
	}
	allocatedValues := make([]int64, 0, len(continuityAllocations))
	for userID, value := range continuityAllocations {
		result[userID] = value
		allocatedValues = append(allocatedValues, value)
	}
	continuityAllocated, err := checkedSum(allocatedValues...)
	if err != nil {
		return nil, err
	}
	remainingCap := cap - continuityAllocated
	if remainingCap <= 0 {
		return result, nil
	}
	remainingRequests := make([]requestRow, 0, len(requests))
	for _, request := range requests {
		request.Requested -= continuityAllocations[request.UserID]
		if request.Requested > 0 {
			remainingRequests = append(remainingRequests, request)
		}
	}
	remainingValues := make([]int64, 0, len(remainingRequests))
	for _, request := range remainingRequests {
		remainingValues = append(remainingValues, request.Requested)
	}
	remainingTotal, err := checkedSum(remainingValues...)
	if err != nil {
		return nil, err
	}
	remainingAllocations, err := allocateProportionally(remainingRequests, minQuota(remainingCap, remainingTotal))
	if err != nil {
		return nil, err
	}
	for userID, value := range remainingAllocations {
		result[userID], err = checkedAdd(result[userID], value)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func allocateBaseRequests(requests []requestRow, cap int64, minimumAdjustment int64) (map[int]int64, error) {
	if len(requests) == 0 || cap <= 0 {
		return map[int]int64{}, nil
	}
	values := make([]int64, 0, len(requests))
	for _, request := range requests {
		values = append(values, request.Requested)
	}
	requestTotal, err := checkedSum(values...)
	if err != nil {
		return nil, err
	}
	if minimumAdjustment <= 0 || requestTotal <= cap {
		return allocateWithContinuity(requests, cap)
	}
	minimumAllocation := minimumAdjustment + quotaPerCent()
	eligible := make([]requestRow, 0, len(requests))
	for _, request := range requests {
		if request.Requested >= minimumAllocation {
			eligible = append(eligible, request)
		}
	}
	sort.Slice(eligible, func(left int, right int) bool {
		leftContinuity := eligible[left].ContinuityRequested > 0
		rightContinuity := eligible[right].ContinuityRequested > 0
		if leftContinuity != rightContinuity {
			return leftContinuity
		}
		if eligible[left].ContinuityRequested != eligible[right].ContinuityRequested {
			return eligible[left].ContinuityRequested > eligible[right].ContinuityRequested
		}
		if eligible[left].Requested != eligible[right].Requested {
			return eligible[left].Requested > eligible[right].Requested
		}
		return eligible[left].UserID < eligible[right].UserID
	})
	recipientLimit := min(len(eligible), int(cap/minimumAllocation))
	selected := eligible[:recipientLimit]
	result := make(map[int]int64, len(selected))
	for _, request := range selected {
		result[request.UserID] = minimumAllocation
	}
	remainingCap := cap - minimumAllocation*int64(len(selected))
	remainingRequests := make([]requestRow, 0, len(selected))
	for _, request := range selected {
		request.Requested -= minimumAllocation
		request.ContinuityRequested = maxQuota(0, request.ContinuityRequested-minimumAllocation)
		if request.Requested > 0 {
			remainingRequests = append(remainingRequests, request)
		}
	}
	remainingAllocations, err := allocateWithContinuity(remainingRequests, remainingCap)
	if err != nil {
		return nil, err
	}
	for userID, value := range remainingAllocations {
		result[userID], err = checkedAdd(result[userID], value)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func allocateBonusRequests(requests []requestRow, cap int64) (map[int]int64, error) {
	bonusRows := make([]requestRow, 0, len(requests))
	for _, request := range requests {
		if request.BonusRequested > 0 {
			request.Requested = request.BonusRequested
			bonusRows = append(bonusRows, request)
		}
	}
	return allocateProportionally(bonusRows, cap)
}
