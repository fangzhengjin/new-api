package quota

import (
	"database/sql"
	"errors"
	"math/big"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// FairnessMetrics summarizes worst-case coverage for one allocation result.
type FairnessMetrics struct {
	Population                 int
	MinimumCoverageBasisPoints int64
	P10CoverageBasisPoints     int64
	P50CoverageBasisPoints     int64
	P90CoverageBasisPoints     int64
	MinimumSafetyBasisPoints   int64
	SafetyUnmet                int
	NewUserCount               int
	NewUserCoverageBasisPoints *int64
	ReclaimedQuota             int64
	RecoveryReserveQuota       int64
	OccupiedAfterQuota         int64
}

// FairnessShadowItem compares current and candidate deltas for one managed user.
type FairnessShadowItem struct {
	UserID                       int
	Username                     string
	CurrentBalanceQuota          int64
	SafetyTargetQuota            int64
	DemandTargetQuota            int64
	TargetQuota                  int64
	CurrentAdjustmentQuota       int64
	CandidateAdjustmentQuota     int64
	CurrentAfterQuota            int64
	CandidateAfterQuota          int64
	CurrentCoverageBasisPoints   int64
	CandidateCoverageBasisPoints int64
}

// ConcentrationShadowItem exposes one user's authoritative spend and capped shadow outcome.
type ConcentrationShadowItem struct {
	UserID                       int
	Username                     string
	PeriodSpendQuota             int64
	CurrentBalanceQuota          int64
	CurrentPositionQuota         int64
	SpendShareBasisPoints        int64
	SafetyTargetQuota            int64
	RawTargetQuota               int64
	EffectiveTargetQuota         int64
	PositionCeilingQuota         int64
	AdjustmentQuota              int64
	AfterBalanceQuota            int64
	AfterPositionQuota           int64
	CappedQuota                  int64
	RawCoverageBasisPoints       int64
	EffectiveCoverageBasisPoints int64
}

// ConcentrationShadowVariant summarizes one fixed multiplier on the shared snapshot.
type ConcentrationShadowVariant struct {
	MultiplierBasisPoints    int64
	Population               int
	PositionCeilingQuota     int64
	MaximumPositionShare     int64
	CappedUsers              int
	CappedQuota              int64
	MinimumRawCoverage       int64
	P10RawCoverage           int64
	P50RawCoverage           int64
	MinimumEffectiveCoverage int64
	P10EffectiveCoverage     int64
	P50EffectiveCoverage     int64
	MinimumSafetyCoverage    int64
	SafetyUnmet              int
	PlannedIncreaseQuota     int64
	ReclaimedQuota           int64
	OccupiedAfterQuota       int64
	UnallocatedStageQuota    int64
	Items                    []ConcentrationShadowItem
}

// FairnessShadowComparison is a read-only current/candidate calculation on one snapshot.
type FairnessShadowComparison struct {
	SnapshotAt                int64
	StageCapQuota             int64
	CurrentAlgorithmVersion   string
	CandidateAlgorithmVersion string
	CandidateQualified        bool
	Current                   FairnessMetrics
	Candidate                 FairnessMetrics
	Items                     []FairnessShadowItem
	ConcentrationVariants     []ConcentrationShadowVariant
}

// CompareFairness calculates both algorithms without creating a quota plan or item.
func CompareFairness(params GenerateParams) (*FairnessShadowComparison, error) {
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	if params.PlanType != model.QuotaPlanTypeAdjustment {
		return nil, errors.New("公平性影子比较只支持动态调配方案")
	}
	return compareFairnessAt(params, common.GetTimestamp())
}

func compareFairnessAt(params GenerateParams, snapshotAt int64) (*FairnessShadowComparison, error) {
	var current, candidate *planCalculation
	concentrationCalculations := make([]*planCalculation, 0, len(concentrationMultipliers))
	options := generationTransactionOptions()
	if options == nil {
		options = &sql.TxOptions{ReadOnly: true}
	} else {
		options = &sql.TxOptions{Isolation: options.Isolation, ReadOnly: true}
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		current, err = calculatePlanInTransaction(tx, params, snapshotAt, allocationCurrent, false, 0)
		if err != nil {
			return err
		}
		candidate, err = calculatePlanInTransaction(tx, params, snapshotAt, allocationCandidate, false, 0)
		if err != nil {
			return err
		}
		for _, multiplier := range concentrationMultipliers {
			calculation, calculationErr := calculatePlanInTransaction(
				tx, params, snapshotAt, allocationConcentration, false, multiplier,
			)
			if calculationErr != nil {
				return calculationErr
			}
			concentrationCalculations = append(concentrationCalculations, calculation)
		}
		return nil
	}, options)
	if err != nil {
		return nil, err
	}
	currentMetrics, currentTargets, err := fairnessMetrics(current)
	if err != nil {
		return nil, err
	}
	candidateMetrics, candidateTargets, err := fairnessMetrics(candidate)
	if err != nil {
		return nil, err
	}
	items, err := fairnessShadowItems(current, candidate, currentTargets, candidateTargets)
	if err != nil {
		return nil, err
	}
	qualified := candidateMetrics.MinimumCoverageBasisPoints >= currentMetrics.MinimumCoverageBasisPoints &&
		candidateMetrics.P10CoverageBasisPoints >= currentMetrics.P10CoverageBasisPoints &&
		candidateMetrics.MinimumSafetyBasisPoints >= currentMetrics.MinimumSafetyBasisPoints &&
		candidateMetrics.SafetyUnmet <= currentMetrics.SafetyUnmet &&
		candidate.Result.Summary.StageCap == current.Result.Summary.StageCap &&
		candidate.Result.Summary.OccupiedAfter <= candidate.Result.Summary.StageCap
	concentrationVariants := make([]ConcentrationShadowVariant, 0, len(concentrationCalculations))
	for index, calculation := range concentrationCalculations {
		variant, variantErr := concentrationShadowVariant(calculation, concentrationMultipliers[index])
		if variantErr != nil {
			return nil, variantErr
		}
		concentrationVariants = append(concentrationVariants, variant)
	}
	return &FairnessShadowComparison{
		SnapshotAt: snapshotAt, StageCapQuota: current.Result.Summary.StageCap,
		CurrentAlgorithmVersion:   current.Result.Plan.AlgorithmVersion,
		CandidateAlgorithmVersion: candidate.Result.Plan.AlgorithmVersion,
		CandidateQualified:        qualified, Current: currentMetrics, Candidate: candidateMetrics, Items: items,
		ConcentrationVariants: concentrationVariants,
	}, nil
}

func concentrationShadowVariant(calculation *planCalculation, multiplierBasisPoints int64) (ConcentrationShadowVariant, error) {
	adjustments := make(map[int]int64, len(calculation.Result.Items))
	reclaimed := int64(0)
	for _, item := range calculation.Result.Items {
		adjustments[item.UserId] = item.AdjustmentQuota
		if item.AdjustmentQuota < 0 {
			var err error
			reclaimed, err = checkedAdd(reclaimed, -item.AdjustmentQuota)
			if err != nil {
				return ConcentrationShadowVariant{}, err
			}
		}
	}

	population := 0
	for _, user := range calculation.Users {
		if user.Status == common.UserStatusEnabled {
			population++
		}
	}
	positionCeiling, err := concentrationPositionCeiling(
		calculation.Result.Summary.StageCap, population, multiplierBasisPoints,
	)
	if err != nil {
		return ConcentrationShadowVariant{}, err
	}
	safetyTarget, err := equalSafetyTarget(calculation.Cycle.InitialGrantQuota, calculation.Snapshot.TotalWorkdays)
	if err != nil {
		return ConcentrationShadowVariant{}, err
	}

	rawCoverageValues := make([]int64, 0, population)
	effectiveCoverageValues := make([]int64, 0, population)
	safetyCoverageValues := make([]int64, 0, population)
	items := make([]ConcentrationShadowItem, 0, population)
	cappedUsers := 0
	cappedQuota := int64(0)
	maximumPositionShare := int64(0)
	safetyUnmet := 0
	for _, user := range calculation.Users {
		if user.Status != common.UserStatusEnabled {
			continue
		}
		stats := calculation.Stats[user.ID]
		position, err := checkedAdd(stats.PeriodSpend, user.Quota)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		rawDemandTarget, err := targetForDays(
			calculation.Profiles[user.ID].WeeklyDemand,
			calculation.Snapshot.BasisMode,
			calculation.Snapshot.CalculationDaysHundred,
		)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		rawTarget := maxQuota(safetyTarget, rawDemandTarget)
		safetyTopUp, err := roundUpCent(maxQuota(0, safetyTarget-user.Quota))
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		positionAfterSafety, err := checkedAdd(position, safetyTopUp)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		demandGap := maxQuota(0, rawDemandTarget-user.Quota-safetyTopUp)
		demandTopUp := roundDownCent(minQuota(demandGap, maxQuota(0, positionCeiling-positionAfterSafety)))
		effectiveTarget, err := checkedSum(user.Quota, safetyTopUp, demandTopUp)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		capped := maxQuota(0, demandGap-demandTopUp)
		if capped > 0 {
			cappedUsers++
			cappedQuota, err = checkedAdd(cappedQuota, capped)
			if err != nil {
				return ConcentrationShadowVariant{}, err
			}
		}
		afterBalance, err := checkedAdd(user.Quota, adjustments[user.ID])
		if err != nil {
			return ConcentrationShadowVariant{}, errors.New("集中度影子结果产生无效用户余额")
		}
		afterPosition, err := checkedAdd(stats.PeriodSpend, afterBalance)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		rawCoverage, err := coverageBasisPoints(afterBalance, rawTarget)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		effectiveCoverage, err := coverageBasisPoints(afterBalance, effectiveTarget)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		safetyCoverage, err := coverageBasisPoints(afterBalance, safetyTarget)
		if err != nil {
			return ConcentrationShadowVariant{}, err
		}
		positionShare := int64(0)
		if calculation.Result.Summary.StageCap > 0 {
			positionShare, err = signedBasisPoints(afterPosition, calculation.Result.Summary.StageCap)
			if err != nil {
				return ConcentrationShadowVariant{}, err
			}
		}
		if len(items) == 0 || positionShare > maximumPositionShare {
			maximumPositionShare = positionShare
		}
		spendShare := int64(0)
		if calculation.Result.Summary.TotalSpend > 0 {
			spendShare, err = bigRatio(
				[]int64{stats.PeriodSpend, 10_000},
				[]int64{calculation.Result.Summary.TotalSpend},
				false,
			)
			if err != nil {
				return ConcentrationShadowVariant{}, err
			}
		}
		if afterBalance < safetyTarget {
			safetyUnmet++
		}
		rawCoverageValues = append(rawCoverageValues, rawCoverage)
		effectiveCoverageValues = append(effectiveCoverageValues, effectiveCoverage)
		safetyCoverageValues = append(safetyCoverageValues, safetyCoverage)
		items = append(items, ConcentrationShadowItem{
			UserID: user.ID, Username: user.Username, PeriodSpendQuota: stats.PeriodSpend,
			CurrentBalanceQuota: user.Quota, CurrentPositionQuota: position, SpendShareBasisPoints: spendShare,
			SafetyTargetQuota: safetyTarget, RawTargetQuota: rawTarget, EffectiveTargetQuota: effectiveTarget,
			PositionCeilingQuota: positionCeiling, AdjustmentQuota: adjustments[user.ID],
			AfterBalanceQuota: afterBalance, AfterPositionQuota: afterPosition, CappedQuota: capped,
			RawCoverageBasisPoints: rawCoverage, EffectiveCoverageBasisPoints: effectiveCoverage,
		})
	}
	sort.Slice(rawCoverageValues, func(left int, right int) bool { return rawCoverageValues[left] < rawCoverageValues[right] })
	sort.Slice(effectiveCoverageValues, func(left int, right int) bool { return effectiveCoverageValues[left] < effectiveCoverageValues[right] })
	sort.Slice(safetyCoverageValues, func(left int, right int) bool { return safetyCoverageValues[left] < safetyCoverageValues[right] })
	sort.Slice(items, func(left int, right int) bool { return items[left].UserID < items[right].UserID })
	variant := ConcentrationShadowVariant{
		MultiplierBasisPoints: multiplierBasisPoints, Population: population,
		PositionCeilingQuota: positionCeiling, MaximumPositionShare: maximumPositionShare,
		CappedUsers: cappedUsers, CappedQuota: cappedQuota, SafetyUnmet: safetyUnmet,
		PlannedIncreaseQuota: calculation.Result.Summary.PlannedIncrease, ReclaimedQuota: reclaimed,
		OccupiedAfterQuota:    calculation.Result.Summary.OccupiedAfter,
		UnallocatedStageQuota: calculation.Result.Summary.AvailableAfter, Items: items,
	}
	if population > 0 {
		variant.MinimumRawCoverage = rawCoverageValues[0]
		variant.P10RawCoverage = nearestRank(rawCoverageValues, 10)
		variant.P50RawCoverage = nearestRank(rawCoverageValues, 50)
		variant.MinimumEffectiveCoverage = effectiveCoverageValues[0]
		variant.P10EffectiveCoverage = nearestRank(effectiveCoverageValues, 10)
		variant.P50EffectiveCoverage = nearestRank(effectiveCoverageValues, 50)
		variant.MinimumSafetyCoverage = safetyCoverageValues[0]
	}
	return variant, nil
}

type fairnessTarget struct {
	Safety int64
	Demand int64
	Target int64
}

func fairnessMetrics(calculation *planCalculation) (FairnessMetrics, map[int]fairnessTarget, error) {
	adjustments := make(map[int]int64, len(calculation.Result.Items))
	reclaimed := int64(0)
	for _, item := range calculation.Result.Items {
		adjustments[item.UserId] = item.AdjustmentQuota
		if item.AdjustmentQuota < 0 {
			var err error
			reclaimed, err = checkedAdd(reclaimed, -item.AdjustmentQuota)
			if err != nil {
				return FairnessMetrics{}, nil, err
			}
		}
	}
	coverageValues := make([]int64, 0, len(calculation.Users))
	safetyValues := make([]int64, 0, len(calculation.Users))
	newUserValues := make([]int64, 0)
	targets := make(map[int]fairnessTarget, len(calculation.Users))
	safetyUnmet := 0
	for _, user := range calculation.Users {
		if user.Status != common.UserStatusEnabled {
			continue
		}
		profile := calculation.Profiles[user.ID]
		safetyTarget, demandTarget, target, err := fairTargets(
			profile.WeeklyDemand, calculation.Cycle.InitialGrantQuota,
			calculation.Snapshot.TotalWorkdays, calculation.Snapshot.BasisMode,
			calculation.Snapshot.CalculationDaysHundred,
		)
		if err != nil {
			return FairnessMetrics{}, nil, err
		}
		targets[user.ID] = fairnessTarget{Safety: safetyTarget, Demand: demandTarget, Target: target}
		if target <= 0 {
			continue
		}
		after, err := checkedAdd(user.Quota, adjustments[user.ID])
		if err != nil {
			return FairnessMetrics{}, nil, errors.New("影子结果产生无效用户余额")
		}
		coverage, err := coverageBasisPoints(after, target)
		if err != nil {
			return FairnessMetrics{}, nil, err
		}
		safetyCoverage, err := coverageBasisPoints(after, safetyTarget)
		if err != nil {
			return FairnessMetrics{}, nil, err
		}
		coverageValues = append(coverageValues, coverage)
		safetyValues = append(safetyValues, safetyCoverage)
		if safetyTarget > 0 && after < safetyTarget {
			safetyUnmet++
		}
		if !hasObservationWindow(user.ObservationStartAt, calculation.Snapshot.SnapshotAt) {
			newUserValues = append(newUserValues, coverage)
		}
	}
	sort.Slice(coverageValues, func(left int, right int) bool { return coverageValues[left] < coverageValues[right] })
	sort.Slice(safetyValues, func(left int, right int) bool { return safetyValues[left] < safetyValues[right] })
	sort.Slice(newUserValues, func(left int, right int) bool { return newUserValues[left] < newUserValues[right] })
	metrics := FairnessMetrics{
		Population: len(coverageValues), SafetyUnmet: safetyUnmet, NewUserCount: len(newUserValues),
		ReclaimedQuota: reclaimed, RecoveryReserveQuota: calculation.Cycle.RecoveryReserveQuota,
		OccupiedAfterQuota: calculation.Result.Summary.OccupiedAfter,
	}
	if len(coverageValues) > 0 {
		metrics.MinimumCoverageBasisPoints = coverageValues[0]
		metrics.P10CoverageBasisPoints = nearestRank(coverageValues, 10)
		metrics.P50CoverageBasisPoints = nearestRank(coverageValues, 50)
		metrics.P90CoverageBasisPoints = nearestRank(coverageValues, 90)
		metrics.MinimumSafetyBasisPoints = safetyValues[0]
	}
	if len(newUserValues) > 0 {
		value := newUserValues[0]
		metrics.NewUserCoverageBasisPoints = &value
	}
	return metrics, targets, nil
}

func fairnessShadowItems(current *planCalculation, candidate *planCalculation, currentTargets map[int]fairnessTarget, candidateTargets map[int]fairnessTarget) ([]FairnessShadowItem, error) {
	currentAdjustments := make(map[int]int64, len(current.Result.Items))
	candidateAdjustments := make(map[int]int64, len(candidate.Result.Items))
	for _, item := range current.Result.Items {
		currentAdjustments[item.UserId] = item.AdjustmentQuota
	}
	for _, item := range candidate.Result.Items {
		candidateAdjustments[item.UserId] = item.AdjustmentQuota
	}
	items := make([]FairnessShadowItem, 0, len(current.Users))
	for _, user := range current.Users {
		target := candidateTargets[user.ID]
		if target.Target == 0 {
			target = currentTargets[user.ID]
		}
		currentAfter, err := checkedAdd(user.Quota, currentAdjustments[user.ID])
		if err != nil {
			return nil, err
		}
		candidateAfter, err := checkedAdd(user.Quota, candidateAdjustments[user.ID])
		if err != nil {
			return nil, err
		}
		currentCoverage, err := coverageBasisPoints(currentAfter, target.Target)
		if err != nil {
			return nil, err
		}
		candidateCoverage, err := coverageBasisPoints(candidateAfter, target.Target)
		if err != nil {
			return nil, err
		}
		items = append(items, FairnessShadowItem{
			UserID: user.ID, Username: user.Username, CurrentBalanceQuota: user.Quota,
			SafetyTargetQuota: target.Safety, DemandTargetQuota: target.Demand, TargetQuota: target.Target,
			CurrentAdjustmentQuota: currentAdjustments[user.ID], CandidateAdjustmentQuota: candidateAdjustments[user.ID],
			CurrentAfterQuota: currentAfter, CandidateAfterQuota: candidateAfter,
			CurrentCoverageBasisPoints: currentCoverage, CandidateCoverageBasisPoints: candidateCoverage,
		})
	}
	sort.Slice(items, func(left int, right int) bool { return items[left].UserID < items[right].UserID })
	return items, nil
}

func coverageBasisPoints(balance int64, target int64) (int64, error) {
	if target <= 0 || balance >= target {
		return 10_000, nil
	}
	return signedBasisPoints(balance, target)
}

func signedBasisPoints(value int64, total int64) (int64, error) {
	if total <= 0 {
		return 0, errors.New("比例分母必须大于0")
	}
	numerator := new(big.Int).Mul(big.NewInt(value), big.NewInt(10_000))
	quotient := new(big.Int).Quo(numerator, big.NewInt(total))
	if !quotient.IsInt64() {
		return 0, errQuotaOverflow
	}
	return quotient.Int64(), nil
}

func nearestRank(sortedValues []int64, percentile int) int64 {
	index := (percentile*len(sortedValues) + 99) / 100
	return sortedValues[max(1, index)-1]
}
