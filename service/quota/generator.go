package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

const (
	basisModeActual         = "actual"
	basisModeWeek           = "week"
	allocationCurrent       = "current"
	allocationCandidate     = "candidate"
	allocationConcentration = "concentration"
	allocationProduction    = "production"
)

// GenerateParams contains the reviewed rules used to create one immutable draft.
type GenerateParams struct {
	CycleID           int
	PlanType          model.QuotaPlanType
	StagePercent      int
	NextAdjustmentAt  *int64
	BasisMode         string
	EarlyReclaim      bool
	ReclaimCapPercent int
	UsageBonusPercent int
	ThoroughRelease   bool
	CreatedBy         string
}

// PlanSummary is the fund-conservation result returned with a generated plan.
type PlanSummary struct {
	TotalSpend      int64 `json:"-"`
	ManagedBalance  int64 `json:"-"`
	PlannedIncrease int64 `json:"-"`
	PlannedDecrease int64 `json:"-"`
	OccupiedBefore  int64 `json:"-"`
	OccupiedAfter   int64 `json:"-"`
	StageCap        int64 `json:"-"`
	AvailableAfter  int64 `json:"-"`
}

// PlanResult returns the persisted plan, item snapshots, and conserved totals.
type PlanResult struct {
	Plan    model.QuotaPlan   `json:"plan"`
	Items   []model.QuotaItem `json:"items"`
	Summary PlanSummary       `json:"summary"`
}

type planCalculation struct {
	Result   *PlanResult
	Cycle    model.QuotaCycle
	Snapshot snapshotParams
	Users    []userSnapshot
	Profiles map[int]DemandProfile
	Stats    map[int]spendStats
}

// PlanParameters is persisted as TEXT so regeneration uses the exact reviewed settings.
type PlanParameters struct {
	BasisMode               string `json:"basis_mode"`
	CalculationDaysHundred  int64  `json:"calculation_days_hundred"`
	TotalWorkdays           int    `json:"total_workdays"`
	RemainingWorkdays       int    `json:"remaining_workdays"`
	ConcentrationMultiplier int64  `json:"concentration_multiplier_basis_points"`
	EarlyReclaim            bool   `json:"early_reclaim"`
	ReclaimCapPercent       int    `json:"reclaim_cap_percent"`
	UsageBonusPercent       int    `json:"usage_bonus_percent"`
	ThoroughRelease         bool   `json:"thorough_release"`
	Manual                  bool   `json:"manual,omitempty"`
	RestoreCycleID          int    `json:"restore_cycle_id,omitempty"`
}

type snapshotParams struct {
	GenerateParams
	SnapshotAt             int64
	CalculationDaysHundred int64
	TotalWorkdays          int
	RemainingWorkdays      int
}

type userSnapshot struct {
	ID                 int
	Username           string
	DisplayName        string
	Email              string
	Status             int
	CreatedAt          int64
	ObservationStartAt int64
	Quota              int64
}

type spendStats struct {
	PeriodSpend int64
	RecentSpend int64
	LastUse     int64
}

type userAdjustment struct {
	UserID          int
	Username        string
	DisplayName     string
	Email           string
	Action          model.QuotaAdjustmentAction
	SnapshotBalance int64
	Adjustment      int64
	Retained        int64
	CalculationData map[string]interface{}
	BasisText       string
}

// GeneratePlan reads a bounded snapshot and saves the draft and all items atomically.
func GeneratePlan(params GenerateParams) (*PlanResult, error) {
	if common.BatchUpdateEnabled {
		return nil, errors.New("批量额度更新已启用，请关闭 BATCH_UPDATE_ENABLED 并等待队列落库后重试")
	}
	var result *PlanResult
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		generated, err := generatePlanInTransaction(tx, params, 0)
		if err != nil {
			return err
		}
		result = generated
		return nil
	}, generationTransactionOptions())
	return result, err
}

func normalizeGenerateParams(params GenerateParams, cycle model.QuotaCycle) GenerateParams {
	if params.PlanType == model.QuotaPlanTypeInitialization {
		params.EarlyReclaim = false
		params.ReclaimCapPercent = 0
		params.UsageBonusPercent = 0
	} else if params.StagePercent == 10_000 {
		params.EarlyReclaim = true
		params.ReclaimCapPercent = 100
		params.UsageBonusPercent = 0
	}
	if params.ThoroughRelease {
		nextAdjustment := cycle.CycleEndAt
		params.NextAdjustmentAt = &nextAdjustment
	} else if params.NextAdjustmentAt != nil {
		nextAdjustment := localDayAt(*params.NextAdjustmentAt, 0, 9)
		if dayStart(*params.NextAdjustmentAt).Equal(dayStart(cycle.CycleEndAt)) {
			nextAdjustment = cycle.CycleEndAt
		}
		params.NextAdjustmentAt = &nextAdjustment
	}
	return params
}

func generationTransactionOptions() *sql.TxOptions {
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
}

func generatePlanInTransaction(tx *gorm.DB, params GenerateParams, snapshotAt int64) (*PlanResult, error) {
	calculation, err := calculatePlanInTransaction(tx, params, snapshotAt, allocationProduction, true, 0)
	if err != nil {
		return nil, err
	}
	return calculation.Result, nil
}

func calculatePlanInTransaction(tx *gorm.DB, params GenerateParams, snapshotAt int64, allocationMode string, persist bool, concentrationBasisPoints int64) (*planCalculation, error) {
	var cycle model.QuotaCycle
	cycleQuery := tx
	if persist {
		cycleQuery = model.LockForUpdate(tx)
	}
	if err := cycleQuery.First(&cycle, params.CycleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("周期不存在")
		}
		return nil, err
	}
	if snapshotAt == 0 {
		snapshotAt = time.Now().Unix()
	}
	if allocationMode == allocationProduction {
		allocationMode = cycleAllocationMode(&cycle)
		if allocationMode == allocationConcentration {
			concentrationBasisPoints = cycle.ConcentrationMultiplier
		}
	}
	if allocationMode == allocationConcentration {
		if err := validateConcentrationMultiplier(concentrationBasisPoints); err != nil {
			return nil, err
		}
	}
	if persist && params.ThoroughRelease &&
		!supportsThoroughRelease(&cycle) {
		return nil, errors.New("彻底释放需要先配置可用的小额恢复渠道")
	}
	if params.ReclaimCapPercent < 0 || params.ReclaimCapPercent > 100 ||
		params.UsageBonusPercent < 0 || params.UsageBonusPercent > 100 {
		return nil, errors.New("调减上限和按用量追加比例必须是0%至100%的整数")
	}
	if params.NextAdjustmentAt == nil {
		return nil, errors.New("预计下次调整日期不能为空")
	}
	params = normalizeGenerateParams(params, cycle)
	if allocationMode == allocationConcentration {
		params.UsageBonusPercent = 0
	}
	snapshot, err := validateGenerationParams(params, cycle, snapshotAt)
	if err != nil {
		return nil, err
	}
	if params.PlanType == model.QuotaPlanTypeAdjustment {
		var executedCount int64
		if err := tx.Model(&model.QuotaPlan{}).
			Where("cycle_id = ? AND status = ?", cycle.Id, model.QuotaPlanStatusExecuted).
			Count(&executedCount).Error; err != nil {
			return nil, err
		}
		if executedCount == 0 {
			return nil, errors.New("当前周期尚无已执行方案，请先完成初始化发放")
		}
	}

	users, err := loadManagedUsers(tx)
	if err != nil {
		return nil, err
	}
	observationStarts, err := loadLatestGrantTimes(tx, cycle.Id)
	if err != nil {
		return nil, err
	}
	for index := range users {
		if observationStarts[users[index].ID] > users[index].ObservationStartAt {
			users[index].ObservationStartAt = observationStarts[users[index].ID]
		}
	}
	managedBalance, err := managedBalanceTotal(users)
	if err != nil {
		return nil, err
	}
	logDB := model.LOG_DB
	if logDB == model.DB {
		logDB = tx
	}
	totalSpend, stats, err := loadSpendSnapshot(tx, logDB, users, cycle.Id, cycle.CycleStartAt, snapshotAt)
	if err != nil {
		return nil, err
	}
	historicalDecreases, err := loadHistoricalOrdinaryDecreases(tx, cycle.Id, snapshotAt)
	if err != nil {
		return nil, err
	}

	profiles := make(map[int]DemandProfile, len(users))
	for _, user := range users {
		profile, err := calculateDemandProfile(
			user.Quota,
			stats[user.ID].PeriodSpend,
			stats[user.ID].RecentSpend,
			stats[user.ID].LastUse,
			cycle.CycleStartAt,
			snapshotAt,
			user.CreatedAt,
			cycle.InitialGrantQuota,
		)
		if err != nil {
			return nil, fmt.Errorf("用户 %d 需求计算失败: %w", user.ID, err)
		}
		profiles[user.ID] = profile
	}

	occupiedBefore, err := checkedAdd(totalSpend, managedBalance)
	if err != nil {
		return nil, err
	}
	requestedStageCap := cycle.BudgetQuota - cycle.RecoveryReserveQuota
	if requestedStageCap < 0 {
		return nil, errors.New("小额恢复池超过周期预算")
	}
	if params.PlanType == model.QuotaPlanTypeAdjustment {
		requestedStageCap, err = regularStageCap(cycle.BudgetQuota, cycle.RecoveryReserveQuota, params.StagePercent)
		if err != nil {
			return nil, err
		}
	}

	var adjustments []userAdjustment
	if params.PlanType == model.QuotaPlanTypeInitialization {
		adjustments = generateInitializationItems(users, cycle.InitialGrantQuota)
	} else {
		lastPositiveAdjustments := map[int]int64{}
		if allocationMode == allocationCandidate || allocationMode == allocationConcentration {
			lastPositiveAdjustments, err = loadLatestPositiveAdjustmentTimes(tx, cycle.Id)
			if err != nil {
				return nil, err
			}
		}
		adjustments, err = generateAdjustmentItems(
			users,
			profiles,
			stats,
			historicalDecreases,
			cycle.InitialGrantQuota,
			cycle.BudgetQuota,
			snapshot,
			requestedStageCap,
			occupiedBefore,
			allocationMode,
			lastPositiveAdjustments,
			concentrationBasisPoints,
		)
		if err != nil {
			return nil, err
		}
	}

	plannedIncrease, plannedDecrease, err := adjustmentTotals(adjustments)
	if err != nil {
		return nil, err
	}
	occupiedAfter, err := checkedAdd(occupiedBefore, plannedIncrease)
	if err != nil {
		return nil, err
	}
	occupiedAfter, err = checkedAdd(occupiedAfter, -plannedDecrease)
	if err != nil {
		return nil, err
	}
	if occupiedAfter > cycle.BudgetQuota {
		return nil, errors.New("计划结果超过公司采购总额")
	}
	effectiveStagePercent := params.StagePercent
	if params.PlanType == model.QuotaPlanTypeInitialization {
		effectiveStagePercent = 0
		if occupiedAfter > 0 {
			effectiveStagePercent64, err := bigRatio(
				[]int64{occupiedAfter, 10_000},
				[]int64{cycle.BudgetQuota},
				true,
			)
			if err != nil {
				return nil, err
			}
			effectiveStagePercent = int(effectiveStagePercent64)
		}
	}
	stageCap := requestedStageCap
	if plannedIncrease > 0 && occupiedAfter > stageCap {
		return nil, errors.New("计划结果超过当前阶段累计释放上限")
	}

	parameters := PlanParameters{
		BasisMode:               snapshot.BasisMode,
		CalculationDaysHundred:  snapshot.CalculationDaysHundred,
		TotalWorkdays:           snapshot.TotalWorkdays,
		RemainingWorkdays:       snapshot.RemainingWorkdays,
		ConcentrationMultiplier: cycle.ConcentrationMultiplier,
		EarlyReclaim:            snapshot.EarlyReclaim,
		ReclaimCapPercent:       snapshot.ReclaimCapPercent,
		UsageBonusPercent:       snapshot.UsageBonusPercent,
		ThoroughRelease:         snapshot.ThoroughRelease,
	}
	parameterJSON, err := common.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	plannedDelta, err := checkedAdd(plannedIncrease, -plannedDecrease)
	if err != nil {
		return nil, err
	}
	algorithmVersion := LegacyAlgorithmVersion
	switch allocationMode {
	case allocationCandidate:
		algorithmVersion = CandidateAlgorithmVersion
	case allocationConcentration:
		algorithmVersion = ConcentrationAlgorithmVersion
	}
	plan := model.QuotaPlan{
		CycleId:             cycle.Id,
		PlanType:            params.PlanType,
		StagePercent:        effectiveStagePercent,
		SnapshotAt:          snapshotAt,
		NextAdjustmentAt:    params.NextAdjustmentAt,
		AlgorithmVersion:    algorithmVersion,
		Parameters:          string(parameterJSON),
		BudgetQuotaSnapshot: cycle.BudgetQuota,
		TotalSpendQuota:     totalSpend,
		ManagedBalanceQuota: managedBalance,
		PlannedDeltaQuota:   plannedDelta,
		Status:              model.QuotaPlanStatusDraft,
		CreatedAt:           snapshotAt,
		CreatedBy:           params.CreatedBy,
	}
	if persist {
		if err := tx.Create(&plan).Error; err != nil {
			return nil, err
		}
	}
	items, err := adjustmentsToItems(plan.Id, adjustments)
	if err != nil {
		return nil, err
	}
	if persist && len(items) > 0 {
		if err := tx.Create(&items).Error; err != nil {
			return nil, err
		}
	}
	result := &PlanResult{
		Plan:  plan,
		Items: items,
		Summary: PlanSummary{
			TotalSpend:      totalSpend,
			ManagedBalance:  managedBalance,
			PlannedIncrease: plannedIncrease,
			PlannedDecrease: plannedDecrease,
			OccupiedBefore:  occupiedBefore,
			OccupiedAfter:   occupiedAfter,
			StageCap:        stageCap,
			AvailableAfter:  maxQuota(0, stageCap-occupiedAfter),
		},
	}
	return &planCalculation{Result: result, Cycle: cycle, Snapshot: snapshot, Users: users, Profiles: profiles, Stats: stats}, nil
}

func validateGenerationParams(params GenerateParams, cycle model.QuotaCycle, snapshotAt int64) (snapshotParams, error) {
	if cycle.Status != model.QuotaCycleStatusActive {
		return snapshotParams{}, errors.New("只有活跃周期可以生成调配方案")
	}
	if cycle.BudgetQuota <= 0 {
		return snapshotParams{}, errors.New("周期预算不正确")
	}
	if cycle.ConcentrationMultiplier != 0 && !validConcentrationMultiplier(cycle.ConcentrationMultiplier) {
		return snapshotParams{}, errors.New("周期自动分配上限倍率不正确")
	}
	if err := validateInitialGrantQuota(cycle.InitialGrantQuota); err != nil {
		return snapshotParams{}, err
	}
	if params.PlanType == model.QuotaPlanTypeInitialization {
		if params.StagePercent != 0 {
			return snapshotParams{}, errors.New("初始化方案的阶段由实际发放占比自动计算")
		}
	} else if params.PlanType == model.QuotaPlanTypeAdjustment {
		if params.StagePercent != 7_500 && params.StagePercent != 8_500 &&
			params.StagePercent != 9_500 && params.StagePercent != 10_000 {
			return snapshotParams{}, errors.New("动态调配阶段必须是75%、85%、95%或100%")
		}
	} else {
		return snapshotParams{}, errors.New("方案类型不正确")
	}
	if params.ThoroughRelease && (params.PlanType != model.QuotaPlanTypeAdjustment || params.StagePercent != 10_000) {
		return snapshotParams{}, errors.New("彻底释放只能使用100%动态调配阶段")
	}
	if params.ThoroughRelease && snapshotAt < finalAdjustmentTime(cycle.CycleStartAt, cycle.CycleEndAt) {
		return snapshotParams{}, errors.New("彻底释放只能在最终调配窗口生成")
	}
	if snapshotAt < cycle.CycleStartAt || snapshotAt >= cycle.CycleEndAt {
		return snapshotParams{}, errors.New("数据快照时间必须位于当前周期内")
	}
	if params.NextAdjustmentAt == nil || *params.NextAdjustmentAt <= snapshotAt || *params.NextAdjustmentAt > cycle.CycleEndAt {
		return snapshotParams{}, errors.New("预计下次调整日期必须晚于快照且不晚于周期结束日期")
	}
	if params.BasisMode != basisModeActual && params.BasisMode != basisModeWeek {
		return snapshotParams{}, errors.New("调增时间口径不正确")
	}
	calculationDaysHundred := int64(700)
	if params.BasisMode == basisModeActual {
		difference := *params.NextAdjustmentAt - snapshotAt
		numerator := new(big.Int).Mul(big.NewInt(difference), big.NewInt(100))
		numerator.Add(numerator, big.NewInt(daySeconds/2))
		numerator.Quo(numerator, big.NewInt(daySeconds))
		if !numerator.IsInt64() {
			return snapshotParams{}, errQuotaOverflow
		}
		calculationDaysHundred = numerator.Int64()
	}
	totalWorkdays := CountWeekdays(cycle.CycleStartAt, cycle.CycleEndAt, false)
	remainingWorkdays := CountWeekdays(snapshotAt, cycle.CycleEndAt, true)
	if calculationDaysHundred <= 0 || totalWorkdays <= 0 || remainingWorkdays < 0 {
		return snapshotParams{}, errors.New("调配天数和工作日参数不正确")
	}
	return snapshotParams{
		GenerateParams:         params,
		SnapshotAt:             snapshotAt,
		CalculationDaysHundred: calculationDaysHundred,
		TotalWorkdays:          totalWorkdays,
		RemainingWorkdays:      remainingWorkdays,
	}, nil
}

func loadManagedUsers(tx *gorm.DB) ([]userSnapshot, error) {
	var users []model.User
	if err := tx.Unscoped().Where("quota_whitelist = ? OR quota_whitelist IS NULL", false).Order("id").Find(&users).Error; err != nil {
		return nil, err
	}
	snapshots := make([]userSnapshot, 0, len(users))
	for _, user := range users {
		if int64(user.Quota) < int64(common.MinQuota) || int64(user.Quota) > int64(common.MaxQuota) {
			return nil, fmt.Errorf("用户 %d 的当前余额超出可支持范围", user.Id)
		}
		status := user.Status
		if user.DeletedAt.Valid {
			status = common.UserStatusDisabled
		}
		snapshots = append(snapshots, userSnapshot{
			ID:                 user.Id,
			Username:           user.Username,
			DisplayName:        user.DisplayName,
			Email:              user.Email,
			Status:             status,
			CreatedAt:          user.CreatedAt,
			ObservationStartAt: user.CreatedAt,
			Quota:              int64(user.Quota),
		})
	}
	return snapshots, nil
}

func loadLatestGrantTimes(tx *gorm.DB, cycleID int) (map[int]int64, error) {
	rows := make([]struct {
		UserID     int
		ExecutedAt int64
	}, 0)
	if err := tx.Table(model.QuotaItem{}.TableName()+" AS item").
		Select("item.user_id, MAX(plan.executed_at) AS executed_at").
		Joins("JOIN "+model.QuotaPlan{}.TableName()+" AS plan ON plan.id = item.plan_id").
		Where("plan.cycle_id = ? AND plan.status = ? AND item.action = ?", cycleID, model.QuotaPlanStatusExecuted, model.QuotaAdjustmentActionGrant).
		Group("item.user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[int]int64, len(rows))
	for _, row := range rows {
		result[row.UserID] = row.ExecutedAt
	}
	return result, nil
}

func loadLatestPositiveAdjustmentTimes(tx *gorm.DB, cycleID int) (map[int]int64, error) {
	rows := make([]struct {
		UserID     int
		ExecutedAt int64
	}, 0)
	if err := tx.Table(model.QuotaItem{}.TableName()+" AS item").
		Select("item.user_id, MAX(plan.executed_at) AS executed_at").
		Joins("JOIN "+model.QuotaPlan{}.TableName()+" AS plan ON plan.id = item.plan_id").
		Where("plan.cycle_id = ? AND plan.status = ? AND item.adjustment_quota > 0", cycleID, model.QuotaPlanStatusExecuted).
		Group("item.user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[int]int64, len(rows))
	for _, row := range rows {
		result[row.UserID] = row.ExecutedAt
	}
	return result, nil
}

func managedBalanceTotal(users []userSnapshot) (int64, error) {
	values := make([]int64, 0, len(users))
	for _, user := range users {
		values = append(values, user.Quota)
	}
	return checkedSum(values...)
}

func loadSpendSnapshot(tx *gorm.DB, logDB *gorm.DB, users []userSnapshot, cycleID int, cycleStart int64, snapshotAt int64) (int64, map[int]spendStats, error) {
	if model.CompanyQuotaModeEnabled() {
		return loadSettlementSnapshot(tx, users, cycleID, snapshotAt)
	}
	if logDB == nil {
		return 0, nil, errors.New("日志数据库未初始化")
	}
	var totalSpend int64
	if err := logDB.Model(&model.Log{}).
		Select("COALESCE(SUM(quota), 0)").
		Where("type = ? AND created_at >= ? AND created_at <= ?", model.LogTypeConsume, cycleStart, snapshotAt).
		Scan(&totalSpend).Error; err != nil {
		return 0, nil, err
	}
	if totalSpend < 0 {
		return 0, nil, errors.New("本期消费总额不能为负数")
	}
	stats := make(map[int]spendStats, len(users))
	if len(users) == 0 {
		return totalSpend, stats, nil
	}
	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	type statsRow struct {
		UserID      int
		PeriodSpend int64
		RecentSpend int64
		LastUse     int64
	}
	var rows []statsRow
	recentStart := maxQuota(cycleStart, snapshotAt-7*daySeconds)
	if err := logDB.Model(&model.Log{}).
		Select("user_id, COALESCE(SUM(quota), 0) AS period_spend, COALESCE(SUM(CASE WHEN created_at > ? THEN quota ELSE 0 END), 0) AS recent_spend, MAX(created_at) AS last_use", recentStart).
		Where("type = ? AND created_at >= ? AND created_at <= ? AND user_id IN ?", model.LogTypeConsume, cycleStart, snapshotAt, userIDs).
		Group("user_id").
		Scan(&rows).Error; err != nil {
		return 0, nil, err
	}
	for _, row := range rows {
		if row.PeriodSpend < 0 || row.RecentSpend < 0 {
			return 0, nil, fmt.Errorf("用户 %d 的消费统计不能为负数", row.UserID)
		}
		stats[row.UserID] = spendStats{
			PeriodSpend: row.PeriodSpend,
			RecentSpend: row.RecentSpend,
			LastUse:     row.LastUse,
		}
	}
	return totalSpend, stats, nil
}

func loadSettlementSnapshot(tx *gorm.DB, users []userSnapshot, cycleID int, snapshotAt int64) (int64, map[int]spendStats, error) {
	totalSpend, err := model.SumQuotaCycleSettlement(tx, cycleID, snapshotAt)
	if err != nil {
		return 0, nil, err
	}
	stats := make(map[int]spendStats, len(users))
	if len(users) == 0 {
		return totalSpend, stats, nil
	}
	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	recentStart := maxQuota(0, snapshotAt-7*daySeconds)
	type statsRow struct {
		UserID      int
		PeriodSpend int64
		RecentSpend int64
		LastUse     int64
	}
	var rows []statsRow
	if err := tx.Model(&model.QuotaCycleSettlement{}).
		Select("user_id, COALESCE(SUM(quota), 0) AS period_spend, COALESCE(SUM(CASE WHEN billing_at > ? THEN quota ELSE 0 END), 0) AS recent_spend, MAX(billing_at) AS last_use", recentStart).
		Where("cycle_id = ? AND billing_at <= ? AND user_id IN ?", cycleID, snapshotAt, userIDs).
		Group("user_id").Scan(&rows).Error; err != nil {
		return 0, nil, err
	}
	for _, row := range rows {
		if row.PeriodSpend < 0 || row.RecentSpend < 0 {
			return 0, nil, fmt.Errorf("用户 %d 的消费统计不能为负数", row.UserID)
		}
		stats[row.UserID] = spendStats{PeriodSpend: row.PeriodSpend, RecentSpend: row.RecentSpend, LastUse: row.LastUse}
	}
	return totalSpend, stats, nil
}

func loadHistoricalOrdinaryDecreases(tx *gorm.DB, cycleID int, snapshotAt int64) (map[int]int64, error) {
	var items []model.QuotaItem
	if err := tx.Model(&model.QuotaItem{}).
		Joins("JOIN tool_quota_adjustment_plans ON tool_quota_adjustment_plans.id = tool_quota_adjustment_items.plan_id").
		Where("tool_quota_adjustment_plans.cycle_id = ? AND tool_quota_adjustment_plans.status = ? AND tool_quota_adjustment_plans.executed_at <= ? AND tool_quota_adjustment_items.action = ? AND tool_quota_adjustment_items.adjustment_quota < 0",
			cycleID, model.QuotaPlanStatusExecuted, snapshotAt, model.QuotaAdjustmentActionDecrease).
		Find(&items).Error; err != nil {
		return nil, err
	}
	result := make(map[int]int64)
	for _, item := range items {
		var data struct {
			DecreaseKind string `json:"decrease_kind"`
		}
		if err := common.Unmarshal([]byte(item.CalculationData), &data); err != nil {
			return nil, fmt.Errorf("方案明细 %d 的计算数据无效: %w", item.Id, err)
		}
		if data.DecreaseKind != "ordinary" {
			continue
		}
		amount := -item.AdjustmentQuota
		var err error
		result[item.UserId], err = checkedAdd(result[item.UserId], amount)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func generateInitializationItems(users []userSnapshot, initialGrant int64) []userAdjustment {
	items := make([]userAdjustment, 0, len(users))
	for _, user := range users {
		if user.Status != common.UserStatusEnabled {
			if user.Quota > 0 {
				items = append(items, userAdjustment{
					UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
					Action: model.QuotaAdjustmentActionReclaim, SnapshotBalance: user.Quota,
					Adjustment: -user.Quota, BasisText: "账户当前处于停用状态，停用期间不保留可用额度",
					CalculationData: map[string]interface{}{"decrease_kind": "disabled"},
				})
			}
			continue
		}
		netAdjustment := initialGrant - user.Quota
		if netAdjustment == 0 {
			continue
		}
		items = append(items, userAdjustment{
			UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
			Action: model.QuotaAdjustmentActionInitialize, SnapshotBalance: user.Quota, Adjustment: netAdjustment,
			CalculationData: map[string]interface{}{
				"initial_grant_quota": strconv.FormatInt(initialGrant, 10),
				"previous_balance":    strconv.FormatInt(user.Quota, 10),
			},
			BasisText: fmt.Sprintf("按当前余额 %s 与本期首次额度 %s 核定差额", FormatQuota(user.Quota), FormatQuota(initialGrant)),
		})
	}
	return items
}

type ordinaryDecrease struct {
	UserID    int
	Amount    int64
	Retained  int64
	BasisText string
}

func generateAdjustmentItems(
	users []userSnapshot,
	profiles map[int]DemandProfile,
	stats map[int]spendStats,
	historicalDecreases map[int]int64,
	initialGrant int64,
	budget int64,
	params snapshotParams,
	stageCap int64,
	occupiedBefore int64,
	allocationMode string,
	lastPositiveAdjustments map[int]int64,
	concentrationBasisPoints int64,
) ([]userAdjustment, error) {
	minimumOrdinaryAdjustment := quotaPerUnit()
	finalLowUsageThreshold := 10 * minimumOrdinaryAdjustment
	items := make([]userAdjustment, 0, len(users))
	activeUsers := make([]userSnapshot, 0, len(users))
	usersByID := make(map[int]userSnapshot, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
		if user.Status == common.UserStatusEnabled {
			activeUsers = append(activeUsers, user)
		}
	}
	finalStage := params.StagePercent >= 10_000
	basicSafetyTarget := int64(0)
	positionCeiling := int64(0)
	if allocationMode == allocationConcentration {
		var err error
		basicSafetyTarget, err = equalSafetyTarget(initialGrant, params.TotalWorkdays)
		if err != nil {
			return nil, err
		}
		positionCeiling, err = concentrationPositionCeiling(stageCap, len(activeUsers), concentrationBasisPoints)
		if err != nil {
			return nil, err
		}
	}
	finalLowUsageIDs := make(map[int]bool)
	if finalStage {
		for _, user := range activeUsers {
			profile := profiles[user.ID]
			userStats := stats[user.ID]
			if !profile.SuspectedExhausted && userStats.PeriodSpend <= finalLowUsageThreshold && userStats.RecentSpend == 0 {
				finalLowUsageIDs[user.ID] = true
			}
		}
	}

	supplement, err := bigRatio(
		[]int64{initialGrant, int64(params.RemainingWorkdays)},
		[]int64{int64(params.TotalWorkdays)},
		false,
	)
	if err != nil {
		return nil, err
	}
	supplement = roundDownCent(supplement)
	requests := make([]requestRow, 0, len(activeUsers))
	increaseNeedIDs := make(map[int]bool)
	requestBasisTexts := make(map[int]string)
	for _, user := range activeUsers {
		if finalLowUsageIDs[user.ID] {
			continue
		}
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		demandTarget, err := targetForDays(profile.WeeklyDemand, params.BasisMode, params.CalculationDaysHundred)
		if err != nil {
			return nil, fmt.Errorf("用户 %d 调增目标计算失败: %w", user.ID, err)
		}
		if allocationMode == allocationConcentration {
			safetyTopUp, err := roundUpCent(maxQuota(0, basicSafetyTarget-user.Quota))
			if err != nil {
				return nil, err
			}
			positionAfterSafety, err := checkedSum(userStats.PeriodSpend, user.Quota, safetyTopUp)
			if err != nil {
				return nil, err
			}
			demandHeadroom := maxQuota(0, positionCeiling-positionAfterSafety)
			demandGap := maxQuota(0, demandTarget-user.Quota-safetyTopUp)
			demandTopUp := roundDownCent(minQuota(demandGap, demandHeadroom))
			requested, err := checkedAdd(safetyTopUp, demandTopUp)
			if err != nil {
				return nil, err
			}
			if !isMaterialAdjustment(requested, minimumOrdinaryAdjustment) {
				continue
			}
			target, err := checkedAdd(user.Quota, requested)
			if err != nil {
				return nil, err
			}
			increaseNeedIDs[user.ID] = true
			requestBasisTexts[user.ID] = "先补足基础可用额度，再按预计需求和本周期个人自动分配上限核定增加额度"
			requests = append(requests, requestRow{
				UserID: user.ID, Requested: requested, ContinuityRequested: safetyTopUp,
				Balance: user.Quota, SafetyTarget: basicSafetyTarget, Target: target,
				LastPositiveAt: lastPositiveAdjustments[user.ID],
			})
			continue
		}
		minimumTarget := int64(0)
		if user.Quota == 0 {
			minimumTarget = supplement
		}
		target := maxQuota(demandTarget, minimumTarget)
		safetyTarget, _, fairTarget, err := fairTargets(
			profile.WeeklyDemand, initialGrant, params.TotalWorkdays,
			params.BasisMode, params.CalculationDaysHundred,
		)
		if err != nil {
			return nil, err
		}
		if allocationMode == allocationCandidate {
			target = fairTarget
		}
		if target <= user.Quota {
			continue
		}
		requested, err := roundUpCent(target - user.Quota)
		if err != nil {
			return nil, err
		}
		if !isMaterialAdjustment(requested, minimumOrdinaryAdjustment) {
			continue
		}
		purpose := "调增"
		if user.Quota == 0 {
			purpose = "补发"
		}
		basisText := demandBasis(userStats.PeriodSpend, user.Quota, profile, purpose)
		if minimumTarget >= demandTarget && minimumTarget > 0 {
			if userStats.PeriodSpend == 0 {
				basisText = fmt.Sprintf(
					"账户当前正常且余额为0；按首次实际发放额度 %s 及剩余 %d 个工作日核定补发额度",
					FormatQuota(initialGrant), params.RemainingWorkdays,
				)
			} else {
				basisText = "账户当前正常且余额为0；按剩余工作日基础额度与历史用量取较高值核定补发额度"
			}
		}
		requestBasisTexts[user.ID] = basisText
		continuity := int64(0)
		if user.Quota == 0 || profile.SuspectedExhausted {
			continuity, err = continuityTopUp(user.Quota, profile.WeeklyDemand, initialGrant, params.TotalWorkdays)
			if err != nil {
				return nil, err
			}
			continuity = minQuota(requested, continuity)
		}
		bonusTarget, err := calculateUsageBuffer(
			target,
			profile.WeeklyDemand,
			initialGrant,
			int64(params.UsageBonusPercent)*100,
		)
		if err != nil {
			return nil, err
		}
		increaseNeedIDs[user.ID] = true
		requests = append(requests, requestRow{
			UserID: user.ID, Requested: requested, ContinuityRequested: continuity, BonusRequested: bonusTarget,
			Balance: user.Quota, SafetyTarget: safetyTarget, Target: target,
			LastPositiveAt: lastPositiveAdjustments[user.ID],
		})
	}

	baseRetained, err := bigRatio(
		[]int64{initialGrant, int64(params.RemainingWorkdays)},
		[]int64{int64(params.TotalWorkdays)},
		false,
	)
	if err != nil {
		return nil, err
	}
	baseRetained, err = roundUpCent(baseRetained)
	if err != nil {
		return nil, err
	}
	adjustmentNumber := adjustmentStageNumber(params.StagePercent)
	cumulativePercent := cumulativeReclaimPercent(params.StagePercent, params.ReclaimCapPercent)
	cumulativeCap, err := bigRatio(
		[]int64{initialGrant, int64(cumulativePercent)},
		[]int64{100},
		false,
	)
	if err != nil {
		return nil, err
	}
	cumulativeCap = roundDownCent(cumulativeCap)
	lowUsageIDs := make(map[int]bool, len(activeUsers))
	for _, user := range activeUsers {
		lowUsageIDs[user.ID] = isOrdinaryLowUsage(stats[user.ID].RecentSpend, profiles[user.ID].WeeklyDemand, initialGrant)
	}
	ordinaryPotential := make([]ordinaryDecrease, 0, len(activeUsers))
	ordinaryCandidateIDs := make(map[int]bool)
	for _, user := range activeUsers {
		if increaseNeedIDs[user.ID] || finalLowUsageIDs[user.ID] ||
			!hasObservationWindow(user.ObservationStartAt, params.SnapshotAt) || !lowUsageIDs[user.ID] {
			continue
		}
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		retained, err := retainedForRemainingWorkdays(baseRetained, profile.WeeklyDemand, params.RemainingWorkdays)
		if err != nil {
			return nil, err
		}
		reclaimable := maxQuota(0, user.Quota-retained)
		historicalDecrease := historicalDecreases[user.ID]
		remainingCumulativeCap := budget
		if !params.ThoroughRelease {
			remainingCumulativeCap = maxQuota(0, cumulativeCap-historicalDecrease)
		}
		amount := roundDownCent(minQuota(reclaimable, remainingCumulativeCap))
		if !isMaterialAdjustment(amount, minimumOrdinaryAdjustment) {
			continue
		}
		usage := "本期无使用记录"
		if userStats.PeriodSpend > 0 {
			recent := fmt.Sprintf("最近7日累计使用 %s", FormatQuota(userStats.RecentSpend))
			if userStats.RecentSpend == 0 {
				recent = "最近7日无使用记录"
			}
			usage = fmt.Sprintf("本期累计使用 %s，%s", FormatQuota(userStats.PeriodSpend), recent)
		}
		rule := fmt.Sprintf("按本期、最近7日及额度耗尽修正后的用量较高值核定保留额度为 %s", FormatQuota(retained))
		if baseRetained >= retained {
			rule = fmt.Sprintf("按首次发放额度及剩余工作日核定保留额度为 %s", FormatQuota(retained))
		}
		cumulativeRule := "本次为彻底释放，回收保留额度以上的闲置余额"
		if !params.ThoroughRelease {
			cumulativeRule = fmt.Sprintf(
				"当前为第%d次调配，累计调减上限为首次额度的%d%%，本期此前已调减 %s",
				adjustmentNumber, cumulativePercent, FormatQuota(historicalDecrease),
			)
		}
		ordinaryPotential = append(ordinaryPotential, ordinaryDecrease{
			UserID: user.ID, Amount: amount, Retained: retained,
			BasisText: fmt.Sprintf("%s；%s；%s", usage, cumulativeRule, rule),
		})
		ordinaryCandidateIDs[user.ID] = true
	}

	disabledItems := make([]userAdjustment, 0)
	for _, user := range users {
		if user.Status == common.UserStatusEnabled || user.Quota <= 0 {
			continue
		}
		disabledItems = append(disabledItems, userAdjustment{
			UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
			Action: model.QuotaAdjustmentActionReclaim, SnapshotBalance: user.Quota, Adjustment: -user.Quota,
			CalculationData: map[string]interface{}{"decrease_kind": "disabled"},
			BasisText:       "账户当前处于停用状态，停用期间不保留可用额度",
		})
	}
	finalLowItems := make([]userAdjustment, 0)
	if finalStage {
		for _, user := range activeUsers {
			if !finalLowUsageIDs[user.ID] || user.Quota <= 0 || !isMaterialAdjustment(user.Quota, minimumOrdinaryAdjustment) {
				continue
			}
			userStats := stats[user.ID]
			basisText := "本期无使用记录；最终调配按未使用账户全额回收"
			if userStats.PeriodSpend > 0 {
				basisText = fmt.Sprintf(
					"本期累计使用 %s，最近7日无使用记录；本期用量不超过 %s，最终调配全额回收",
					FormatQuota(userStats.PeriodSpend), FormatQuota(finalLowUsageThreshold),
				)
			}
			finalLowItems = append(finalLowItems, userAdjustment{
				UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
				Action: model.QuotaAdjustmentActionDecrease, SnapshotBalance: user.Quota, Adjustment: -user.Quota,
				CalculationData: map[string]interface{}{
					"period_spend":  strconv.FormatInt(userStats.PeriodSpend, 10),
					"recent_spend":  strconv.FormatInt(userStats.RecentSpend, 10),
					"decrease_kind": "final_low",
				},
				BasisText: basisText,
			})
		}
	}

	selectedOrdinary := []ordinaryDecrease(nil)
	if params.EarlyReclaim || finalStage {
		selectedOrdinary = ordinaryPotential
	}
	for _, candidate := range selectedOrdinary {
		user := usersByID[candidate.UserID]
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		items = append(items, userAdjustment{
			UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
			Action: model.QuotaAdjustmentActionDecrease, SnapshotBalance: user.Quota,
			Adjustment: -candidate.Amount, Retained: candidate.Retained,
			CalculationData: map[string]interface{}{
				"weekly_demand":              strconv.FormatInt(profile.WeeklyDemand, 10),
				"period_spend":               strconv.FormatInt(userStats.PeriodSpend, 10),
				"recent_spend":               strconv.FormatInt(userStats.RecentSpend, 10),
				"historical_decrease":        strconv.FormatInt(historicalDecreases[user.ID], 10),
				"cumulative_reclaim_percent": cumulativePercent,
				"decrease_kind":              "ordinary",
			},
			BasisText: candidate.BasisText,
		})
	}
	items = append(items, disabledItems...)
	items = append(items, finalLowItems...)

	requestTotal, err := sumRequests(requests, func(row requestRow) int64 { return row.Requested })
	if err != nil {
		return nil, err
	}
	bonusRequestTotal, err := sumRequests(requests, func(row requestRow) int64 { return row.BonusRequested })
	if err != nil {
		return nil, err
	}
	forcedTotal, err := negativeAdjustmentTotal(append(append([]userAdjustment{}, disabledItems...), finalLowItems...))
	if err != nil {
		return nil, err
	}
	ordinaryAmounts := make([]int64, 0, len(selectedOrdinary))
	for _, candidate := range selectedOrdinary {
		ordinaryAmounts = append(ordinaryAmounts, candidate.Amount)
	}
	ordinaryTotal, err := checkedSum(ordinaryAmounts...)
	if err != nil {
		return nil, err
	}
	decreaseTotal, err := checkedAdd(forcedTotal, ordinaryTotal)
	if err != nil {
		return nil, err
	}
	increaseCap := availableIncreaseCap(stageCap, occupiedBefore, decreaseTotal)
	var baseAllocations map[int]int64
	if allocationMode == allocationCandidate || allocationMode == allocationConcentration {
		baseAllocations, err = allocateFairRequests(requests, minQuota(requestTotal, increaseCap), minimumOrdinaryAdjustment)
	} else {
		baseAllocations, err = allocateBaseRequests(requests, minQuota(requestTotal, increaseCap), minimumOrdinaryAdjustment)
	}
	if err != nil {
		return nil, err
	}
	baseTotal, err := allocationTotal(baseAllocations)
	if err != nil {
		return nil, err
	}
	bonusCap := int64(0)
	if allocationMode == allocationCurrent && !finalStage && baseTotal == requestTotal {
		bonusCap = minQuota(bonusRequestTotal, maxQuota(0, increaseCap-baseTotal))
	}
	bonusAllocations, err := allocateBonusRequests(requests, bonusCap)
	if err != nil {
		return nil, err
	}
	bonusTotal, err := allocationTotal(bonusAllocations)
	if err != nil {
		return nil, err
	}

	decreaseIDs := make(map[int]bool)
	for _, candidate := range selectedOrdinary {
		decreaseIDs[candidate.UserID] = true
	}
	for _, item := range disabledItems {
		decreaseIDs[item.UserID] = true
	}
	for _, item := range finalLowItems {
		decreaseIDs[item.UserID] = true
	}
	poolCandidates := make([]userSnapshot, 0, len(activeUsers))
	urgentCandidates := make([]userSnapshot, 0, len(activeUsers))
	for _, user := range activeUsers {
		if finalLowUsageIDs[user.ID] || lowUsageIDs[user.ID] || ordinaryCandidateIDs[user.ID] ||
			decreaseIDs[user.ID] || profiles[user.ID].WeeklyDemand <= 0 {
			continue
		}
		poolCandidates = append(poolCandidates, user)
		if increaseNeedIDs[user.ID] || profiles[user.ID].SuspectedExhausted {
			urgentCandidates = append(urgentCandidates, user)
		}
	}
	weightedPoolRecipients := poolCandidates
	if params.ThoroughRelease && len(urgentCandidates) > 0 {
		weightedPoolRecipients = urgentCandidates
	}
	weightedRecipients := make([]weightedRecipient, 0, len(weightedPoolRecipients))
	for _, user := range weightedPoolRecipients {
		weightedRecipients = append(weightedRecipients, weightedRecipient{UserID: user.ID, Weight: profiles[user.ID].WeeklyDemand})
	}
	weightedPoolCap := maxQuota(0, increaseCap-baseTotal-bonusTotal)
	if allocationMode == allocationCandidate && baseTotal != requestTotal {
		weightedPoolCap = 0
	}
	if allocationMode == allocationConcentration {
		weightedPoolCap = 0
	}
	weightedPoolAllocations, err := allocateByWeight(weightedRecipients, weightedPoolCap, minimumOrdinaryAdjustment)
	if err != nil {
		return nil, err
	}
	weightedPoolTotal, err := allocationTotal(weightedPoolAllocations)
	if err != nil {
		return nil, err
	}

	for _, user := range activeUsers {
		baseAmount := baseAllocations[user.ID]
		bonusAmount := bonusAllocations[user.ID]
		weightedPoolAmount := weightedPoolAllocations[user.ID]
		totalIncrease, err := checkedSum(baseAmount, bonusAmount, weightedPoolAmount)
		if err != nil {
			return nil, err
		}
		if totalIncrease <= 0 {
			continue
		}
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		requestBasis, hasRequestBasis := requestBasisTexts[user.ID]
		basisText := requestBasis
		if !hasRequestBasis {
			basisText = demandBasis(userStats.PeriodSpend, user.Quota, profile, "调增")
		}
		if bonusAmount > 0 {
			basisText += fmt.Sprintf("，其中 %s 为按用量比例追加额度", FormatQuota(bonusAmount))
		}
		if weightedPoolAmount > 0 {
			basisText += fmt.Sprintf("，其中 %s 为当前阶段剩余额度按用量权重分配", FormatQuota(weightedPoolAmount))
		}
		action := model.QuotaAdjustmentActionIncrease
		if user.Quota == 0 && hasRequestBasis {
			action = model.QuotaAdjustmentActionGrant
		}
		items = append(items, userAdjustment{
			UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
			Action: action, SnapshotBalance: user.Quota, Adjustment: totalIncrease,
			CalculationData: map[string]interface{}{
				"weekly_demand":       strconv.FormatInt(profile.WeeklyDemand, 10),
				"source":              profile.Source,
				"period_spend":        strconv.FormatInt(userStats.PeriodSpend, 10),
				"recent_spend":        strconv.FormatInt(userStats.RecentSpend, 10),
				"base_quota":          strconv.FormatInt(baseAmount, 10),
				"bonus_quota":         strconv.FormatInt(bonusAmount, 10),
				"weighted_pool_quota": strconv.FormatInt(weightedPoolAmount, 10),
			},
			BasisText: basisText,
		})
	}

	increaseTotal, err := checkedSum(baseTotal, bonusTotal, weightedPoolTotal)
	if err != nil {
		return nil, err
	}
	occupiedAfter, err := checkedSum(occupiedBefore, increaseTotal, -decreaseTotal)
	if err != nil {
		return nil, err
	}
	if occupiedAfter > stageCap {
		return nil, errors.New("计划结果超过当前阶段累计释放上限")
	}
	if allocationMode != allocationConcentration && len(weightedPoolRecipients) > 0 && stageCap-occupiedAfter > minimumOrdinaryAdjustment {
		return nil, errors.New("当前阶段仍有可分配额度未释放")
	}
	return items, nil
}

func sumRequests(requests []requestRow, value func(requestRow) int64) (int64, error) {
	values := make([]int64, 0, len(requests))
	for _, request := range requests {
		values = append(values, value(request))
	}
	return checkedSum(values...)
}

func allocationTotal(allocations map[int]int64) (int64, error) {
	values := make([]int64, 0, len(allocations))
	for _, amount := range allocations {
		values = append(values, amount)
	}
	return checkedSum(values...)
}

func negativeAdjustmentTotal(items []userAdjustment) (int64, error) {
	values := make([]int64, 0, len(items))
	for _, item := range items {
		if item.Adjustment < 0 {
			values = append(values, -item.Adjustment)
		}
	}
	return checkedSum(values...)
}

func adjustmentTotals(items []userAdjustment) (int64, int64, error) {
	increases := make([]int64, 0, len(items))
	decreases := make([]int64, 0, len(items))
	for _, item := range items {
		if item.Adjustment > 0 {
			increases = append(increases, item.Adjustment)
		} else if item.Adjustment < 0 {
			decreases = append(decreases, -item.Adjustment)
		}
	}
	increase, err := checkedSum(increases...)
	if err != nil {
		return 0, 0, err
	}
	decrease, err := checkedSum(decreases...)
	return increase, decrease, err
}

func adjustmentsToItems(planID int, adjustments []userAdjustment) ([]model.QuotaItem, error) {
	items := make([]model.QuotaItem, 0, len(adjustments))
	for _, adjustment := range adjustments {
		after, err := checkedAdd(adjustment.SnapshotBalance, adjustment.Adjustment)
		if err != nil {
			return nil, err
		}
		if after < int64(common.MinQuota) || after > int64(common.MaxQuota) {
			return nil, fmt.Errorf("用户 %d 调整后余额超出可支持范围", adjustment.UserID)
		}
		calculationJSON, err := common.Marshal(adjustment.CalculationData)
		if err != nil {
			return nil, err
		}
		items = append(items, model.QuotaItem{
			PlanId:               planID,
			UserId:               adjustment.UserID,
			Username:             adjustment.Username,
			DisplayName:          adjustment.DisplayName,
			Email:                adjustment.Email,
			Action:               adjustment.Action,
			SnapshotBalanceQuota: adjustment.SnapshotBalance,
			AdjustmentQuota:      adjustment.Adjustment,
			RetainedQuota:        adjustment.Retained,
			CalculationData:      string(calculationJSON),
			BasisText:            adjustment.BasisText,
			LogStatus:            model.QuotaNotificationStatusPending,
			EmailStatus:          model.QuotaNotificationStatusPending,
		})
		if adjustment.Action == model.QuotaAdjustmentActionReclaim {
			items[len(items)-1].EmailStatus = model.QuotaNotificationStatusSkipped
		}
	}
	sort.Slice(items, func(left int, right int) bool { return items[left].UserId < items[right].UserId })
	return items, nil
}
