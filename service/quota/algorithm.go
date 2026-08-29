// Package quota implements cycle quota allocation and settlement.
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
)

const (
	// AlgorithmVersion rejects stale drafts after allocation rules change.
	AlgorithmVersion          = "3.0.0"
	defaultQuotaPerUnit int64 = 500_000
	daySeconds          int64 = 86_400

	exhaustionIdleSeconds        = daySeconds
	minimumObservationWorkdays   = 3
	initialUsageBufferPercent    = 120
	initialStabilityFloorPercent = 80
)

var concentrationMultipliers = [...]int64{15_000, 20_000, 30_000}

func validConcentrationMultiplier(multiplier int64) bool {
	for _, allowed := range concentrationMultipliers {
		if multiplier == allowed {
			return true
		}
	}
	return false
}

func validateConcentrationMultiplier(multiplier int64) error {
	if !validConcentrationMultiplier(multiplier) {
		return errors.New("自动分配上限倍率必须是1.5、2或3")
	}
	return nil
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

func quotaPerUnit() int64 {
	unit := int64(common.QuotaRound(common.QuotaPerUnit))
	if unit <= 0 {
		return defaultQuotaPerUnit
	}
	return unit
}

func quotaPerCent() int64 {
	return max(int64(1), quotaPerUnit()/100)
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

func equalSafetyTarget(initialGrant int64, totalWorkdays int) (int64, error) {
	if totalWorkdays <= 0 {
		return 0, errors.New("总工作日必须大于0")
	}
	target, err := ceilDiv(initialGrant, int64(totalWorkdays))
	if err != nil {
		return 0, err
	}
	return roundUpCent(target)
}

func concentrationPositionCeiling(ordinaryAllocationCap int64, population int, multiplierBasisPoints int64) (int64, error) {
	if population <= 0 {
		return 0, nil
	}
	if multiplierBasisPoints <= 0 {
		return 0, errors.New("集中度倍数必须是正整数基点")
	}
	ceiling, err := bigRatio(
		[]int64{ordinaryAllocationCap, multiplierBasisPoints},
		[]int64{int64(population), 10_000},
		false,
	)
	if err != nil {
		return 0, err
	}
	return roundDownCent(ceiling), nil
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

// DemandProfile is the normalized seven-day demand used by every allocation rule.
type DemandProfile struct {
	WeeklyDemand int64
	Source       string
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
	return max(buffered, stabilityFloor), nil
}

func calculateDemandProfile(
	balance int64,
	observationSpend int64,
	lastUse int64,
	observationStart int64,
	snapshot int64,
	createdAt int64,
	initialGrant int64,
) (DemandProfile, error) {
	elapsed := max(int64(1), snapshot-observationStart)
	observationWeek, err := bigRatio([]int64{observationSpend, 7, daySeconds}, []int64{elapsed}, true)
	if err != nil {
		return DemandProfile{}, err
	}

	availableStart := max(observationStart, createdAt)
	var rawExhaustionWeek int64
	if observationSpend > 0 && lastUse > availableStart {
		rawExhaustionWeek, err = bigRatio(
			[]int64{observationSpend, 7, daySeconds},
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
		exhaustionWeek = min(rawExhaustionWeek, initialGrant)
	}
	ordinaryDemand := max(observationWeek, observationSpend)
	weeklyDemand := max(ordinaryDemand, exhaustionWeek)
	source := "observation_rate"
	if exhaustionWeek > ordinaryDemand {
		source = "exhaustion"
	} else if observationSpend >= observationWeek {
		source = "observation_total"
	}

	return DemandProfile{
		WeeklyDemand: weeklyDemand,
		Source:       source,
	}, nil
}

func hasObservationWindow(createdAt int64, snapshot int64) bool {
	return createdAt <= 0 || CountWeekdays(createdAt, snapshot, true) >= minimumObservationWorkdays
}

func targetForDays(weeklyDemand int64, calculationDaysHundred int64) (int64, error) {
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
	return max(baseRetained, predicted), nil
}

func availableIncreaseCap(ordinaryAllocationCap int64, occupiedBefore int64, reclaimed int64) int64 {
	afterReclaim := occupiedBefore - reclaimed
	if afterReclaim < 0 {
		afterReclaim = 0
	}
	if afterReclaim < ordinaryAllocationCap {
		return ordinaryAllocationCap - afterReclaim
	}
	return 0
}

func isOrdinaryLowUsage(recentSpend int64, weeklyDemand int64, initialGrant int64) bool {
	return recentSpend == 0 || weeklyDemand <= initialGrant/10
}

func isMaterialAdjustment(amount int64, minimumAdjustment int64) bool {
	return amount > minimumAdjustment
}

type requestRow struct {
	UserID         int
	Requested      int64
	Balance        int64
	SafetyTarget   int64
	Target         int64
	LastPositiveAt int64
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
			levelBalance = min(layerTarget, levelBalance)
			if levelBalance <= current {
				continue
			}
			amount, err := roundUpCent(levelBalance - current)
			if err != nil {
				return nil, 0, err
			}
			maximum := request.Requested - allocations[request.UserID]
			amount = min(amount, maximum)
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
	comparison := new(big.Int).Mul(leftBalance, big.NewInt(max(int64(1), rightTarget))).Cmp(
		new(big.Int).Mul(rightBalance, big.NewInt(max(int64(1), leftTarget))),
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
