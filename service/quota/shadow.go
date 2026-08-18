package quota

import (
	"database/sql"
	"errors"
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
	options := generationTransactionOptions()
	if options == nil {
		options = &sql.TxOptions{ReadOnly: true}
	} else {
		options = &sql.TxOptions{Isolation: options.Isolation, ReadOnly: true}
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		current, err = calculatePlanInTransaction(tx, params, snapshotAt, allocationCurrent, false)
		if err != nil {
			return err
		}
		candidate, err = calculatePlanInTransaction(tx, params, snapshotAt, allocationCandidate, false)
		return err
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
	return &FairnessShadowComparison{
		SnapshotAt: snapshotAt, StageCapQuota: current.Result.Summary.StageCap,
		CurrentAlgorithmVersion:   current.Result.Plan.AlgorithmVersion,
		CandidateAlgorithmVersion: candidate.Result.Plan.AlgorithmVersion,
		CandidateQualified:        qualified, Current: currentMetrics, Candidate: candidateMetrics, Items: items,
	}, nil
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
		if err != nil || after < 0 {
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
	return bigRatio([]int64{balance, 10_000}, []int64{target}, false)
}

func nearestRank(sortedValues []int64, percentile int) int64 {
	index := (percentile*len(sortedValues) + 99) / 100
	return sortedValues[max(1, index)-1]
}
