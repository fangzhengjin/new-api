package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// GenerateParams contains the reviewed rules used to create one immutable draft.
type GenerateParams struct {
	CycleID          int
	Name             string
	Purpose          string
	PlanType         model.QuotaPlanType
	NextAdjustmentAt *int64
	CreatedBy        string
}

const (
	PlanNameMaxRunes    = 64
	PlanPurposeMaxRunes = 200
)

// PlanResult returns the persisted plan and item snapshots.
type PlanResult struct {
	Plan  model.QuotaPlan   `json:"plan"`
	Items []model.QuotaItem `json:"items"`
}

type allocationExecutionStats struct {
	AllocationExecuted bool
	LatestExecutedAt   int64
	PlanIDs            []int
}

// PlanParameters records the inputs needed to classify and execute a draft.
type PlanParameters struct {
	ObservationStartAt      int64 `json:"observation_start_at"`
	ConcentrationMultiplier int64 `json:"concentration_multiplier_basis_points"`
	Manual                  bool  `json:"manual,omitempty"`
	TemporaryQuotaRequestID int   `json:"temporary_quota_request_id,omitempty"`
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
	CalculationData map[string]interface{}
	BasisText       string
}

// GeneratePlan reads a bounded snapshot and saves the draft and all items atomically.
func GeneratePlan(params GenerateParams) (*PlanResult, error) {
	if !model.CycleQuotaManagementEnabled() {
		return nil, errors.New("周期额度管理未启用")
	}
	if err := rejectBatchQuotaMutation(); err != nil {
		return nil, err
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
	params.Name = strings.TrimSpace(params.Name)
	params.Purpose = strings.TrimSpace(params.Purpose)
	if params.PlanType == model.QuotaPlanTypeInitialization {
		if params.Name == "" {
			params.Name = "首次发放"
		}
		if params.Purpose == "" {
			params.Purpose = "将受管账户补充到本周期首次额度"
		}
	} else {
		if params.Name == "" {
			params.Name = "额度调配"
		}
		if params.Purpose == "" {
			params.Purpose = "根据上一份已执行方案后的使用情况调整额度"
		}
	}
	if params.NextAdjustmentAt != nil {
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
	var cycle model.QuotaCycle
	if err := model.LockForUpdate(tx).First(&cycle, params.CycleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("周期不存在")
		}
		return nil, err
	}
	if snapshotAt == 0 {
		snapshotAt = time.Now().Unix()
	}
	if err := validateConcentrationMultiplier(cycle.ConcentrationMultiplier); err != nil {
		return nil, err
	}
	if params.NextAdjustmentAt == nil {
		return nil, errors.New("预计下次调整日期不能为空")
	}
	params = normalizeGenerateParams(params, cycle)
	snapshot, err := validateGenerationParams(params, cycle, snapshotAt)
	if err != nil {
		return nil, err
	}
	params = snapshot.GenerateParams
	executionStats, err := loadAllocationExecutionStats(tx, cycle.Id)
	if err != nil {
		return nil, err
	}
	if params.PlanType == model.QuotaPlanTypeInitialization && executionStats.AllocationExecuted {
		return nil, errors.New("当前周期已完成首次发放，不能重复生成初始化方案")
	}
	if params.PlanType == model.QuotaPlanTypeAdjustment && !executionStats.AllocationExecuted {
		return nil, errors.New("当前周期尚无已执行方案，请先完成初始化发放")
	}

	users, err := loadManagedUsers(tx)
	if err != nil {
		return nil, err
	}
	observationStart := cycle.CycleStartAt
	if executionStats.LatestExecutedAt > observationStart {
		observationStart = executionStats.LatestExecutedAt
	}
	for index := range users {
		if observationStart > users[index].ObservationStartAt {
			users[index].ObservationStartAt = observationStart
		}
	}
	stats := make(map[int]spendStats)
	profiles := make(map[int]DemandProfile, len(users))
	if params.PlanType == model.QuotaPlanTypeAdjustment {
		logDB := model.LOG_DB
		if logDB == model.DB {
			logDB = tx
		}
		stats, err = loadSpendSnapshot(logDB, users, cycle.CycleStartAt, observationStart, snapshotAt)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			profile, profileErr := calculateDemandProfile(
				user.Quota,
				stats[user.ID].RecentSpend,
				stats[user.ID].LastUse,
				observationStart,
				snapshotAt,
				user.CreatedAt,
				cycle.InitialGrantQuota,
			)
			if profileErr != nil {
				return nil, fmt.Errorf("用户 %d 需求计算失败: %w", user.ID, profileErr)
			}
			profiles[user.ID] = profile
		}
	}

	occupiedBefore, err := cycleAllocatedQuota(&cycle)
	if err != nil {
		return nil, err
	}
	ordinaryOccupiedBefore, err := ordinaryAllocatedQuota(tx, &cycle)
	if err != nil {
		return nil, err
	}
	requestedOrdinaryAllocationCap := cycle.BudgetQuota - cycle.TemporaryQuotaReserve
	if requestedOrdinaryAllocationCap < 0 {
		return nil, errors.New("临时额度预留超过周期预算")
	}

	var adjustments []userAdjustment
	if params.PlanType == model.QuotaPlanTypeInitialization {
		adjustments = generateInitializationItems(users, cycle.InitialGrantQuota)
	} else {
		lastPositiveAdjustments, err := loadLatestPositiveAdjustmentTimes(tx, executionStats.PlanIDs)
		if err != nil {
			return nil, err
		}
		adjustments, err = generateAdjustmentItems(
			users,
			profiles,
			stats,
			cycle.InitialGrantQuota,
			cycle.BudgetQuota,
			snapshot,
			requestedOrdinaryAllocationCap,
			ordinaryOccupiedBefore,
			lastPositiveAdjustments,
			cycle.ConcentrationMultiplier,
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
		return nil, errors.New("计划结果超过周期总额度")
	}
	ordinaryAllocationCap := requestedOrdinaryAllocationCap
	ordinaryAllocatedAfter, err := checkedSum(ordinaryOccupiedBefore, plannedIncrease, -plannedDecrease)
	if err != nil {
		return nil, err
	}
	ordinaryAllocatedAfter = max(int64(0), ordinaryAllocatedAfter)
	if plannedIncrease > 0 && ordinaryAllocatedAfter > ordinaryAllocationCap {
		return nil, errors.New("计划结果超过普通调配额度上限")
	}

	parameters := PlanParameters{
		ObservationStartAt:      observationStart,
		ConcentrationMultiplier: cycle.ConcentrationMultiplier,
	}
	parameterJSON, err := common.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	plannedDelta, err := checkedAdd(plannedIncrease, -plannedDecrease)
	if err != nil {
		return nil, err
	}
	plan := model.QuotaPlan{
		CycleId:               cycle.Id,
		Name:                  params.Name,
		Purpose:               params.Purpose,
		PlanType:              params.PlanType,
		SnapshotAt:            snapshotAt,
		NextAdjustmentAt:      params.NextAdjustmentAt,
		AlgorithmVersion:      AlgorithmVersion,
		Parameters:            string(parameterJSON),
		BudgetQuotaSnapshot:   cycle.BudgetQuota,
		PlannedDeltaQuota:     plannedDelta,
		AllocationBeforeQuota: &occupiedBefore,
		Status:                model.QuotaPlanStatusDraft,
		CreatedAt:             snapshotAt,
		CreatedBy:             params.CreatedBy,
	}
	if err := tx.Create(&plan).Error; err != nil {
		return nil, err
	}
	items, err := adjustmentsToItems(plan.Id, adjustments)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		if err := tx.Create(&items).Error; err != nil {
			return nil, err
		}
	}
	return &PlanResult{Plan: plan, Items: items}, nil
}

func validateGenerationParams(params GenerateParams, cycle model.QuotaCycle, snapshotAt int64) (snapshotParams, error) {
	if cycle.Status != model.QuotaCycleStatusActive {
		return snapshotParams{}, errors.New("只有活跃周期可以生成调配方案")
	}
	if cycle.BudgetQuota <= 0 {
		return snapshotParams{}, errors.New("周期预算不正确")
	}
	if !validConcentrationMultiplier(cycle.ConcentrationMultiplier) {
		return snapshotParams{}, errors.New("周期自动分配上限倍率不正确")
	}
	if err := validateInitialGrantQuota(cycle.InitialGrantQuota); err != nil {
		return snapshotParams{}, err
	}
	if utf8.RuneCountInString(params.Name) > PlanNameMaxRunes || utf8.RuneCountInString(params.Purpose) > PlanPurposeMaxRunes {
		return snapshotParams{}, errors.New("方案名称或目的过长")
	}
	if params.PlanType != model.QuotaPlanTypeInitialization && params.PlanType != model.QuotaPlanTypeAdjustment {
		return snapshotParams{}, errors.New("方案类型不正确")
	}
	if snapshotAt < cycle.CycleStartAt || snapshotAt >= cycle.CycleEndAt {
		return snapshotParams{}, errors.New("数据快照时间必须位于当前周期内")
	}
	if params.NextAdjustmentAt == nil || *params.NextAdjustmentAt <= snapshotAt || *params.NextAdjustmentAt > cycle.CycleEndAt {
		return snapshotParams{}, errors.New("预计下次调整日期必须晚于快照且不晚于周期结束日期")
	}
	difference := *params.NextAdjustmentAt - snapshotAt
	numerator := new(big.Int).Mul(big.NewInt(difference), big.NewInt(100))
	numerator.Add(numerator, big.NewInt(daySeconds/2))
	numerator.Quo(numerator, big.NewInt(daySeconds))
	if !numerator.IsInt64() {
		return snapshotParams{}, errQuotaOverflow
	}
	calculationDaysHundred := numerator.Int64()
	totalWorkdays := CountWeekdays(cycle.CycleStartAt, cycle.CycleEndAt, false)
	remainingWorkdays := CountWeekdays(snapshotAt, *params.NextAdjustmentAt, true)
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
	if err := tx.Unscoped().Where("quota_whitelist = ?", false).Order("id").Find(&users).Error; err != nil {
		return nil, err
	}
	snapshots := make([]userSnapshot, 0, len(users))
	for _, user := range users {
		if int64(user.Quota) < -int64(common.MaxWalletQuota) || int64(user.Quota) > int64(common.MaxWalletQuota) {
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

func loadAllocationExecutionStats(tx *gorm.DB, cycleID int) (allocationExecutionStats, error) {
	var plans []model.QuotaPlan
	if err := tx.Select("id", "plan_type", "parameters", "executed_at").
		Where("cycle_id = ? AND status = ?", cycleID, model.QuotaPlanStatusExecuted).
		Find(&plans).Error; err != nil {
		return allocationExecutionStats{}, err
	}
	stats := allocationExecutionStats{PlanIDs: make([]int, 0, len(plans))}
	for _, plan := range plans {
		automatic := plan.PlanType == model.QuotaPlanTypeInitialization
		if plan.PlanType == model.QuotaPlanTypeAdjustment {
			var parameters PlanParameters
			if err := common.Unmarshal([]byte(plan.Parameters), &parameters); err != nil {
				return allocationExecutionStats{}, fmt.Errorf("方案 %d 参数无效: %w", plan.Id, err)
			}
			automatic = !parameters.Manual && parameters.TemporaryQuotaRequestID == 0 &&
				parameters.ObservationStartAt > 0
		}
		if !automatic {
			continue
		}
		if plan.PlanType == model.QuotaPlanTypeInitialization {
			stats.AllocationExecuted = true
		}
		stats.PlanIDs = append(stats.PlanIDs, plan.Id)
		if plan.ExecutedAt != nil && *plan.ExecutedAt > stats.LatestExecutedAt {
			stats.LatestExecutedAt = *plan.ExecutedAt
		}
	}
	return stats, nil
}

func loadLatestPositiveAdjustmentTimes(tx *gorm.DB, planIDs []int) (map[int]int64, error) {
	result := make(map[int]int64)
	if len(planIDs) == 0 {
		return result, nil
	}
	rows := make([]struct {
		UserID     int
		ExecutedAt int64
	}, 0)
	if err := tx.Table(model.QuotaItem{}.TableName()+" AS item").
		Select("item.user_id, MAX(plan.executed_at) AS executed_at").
		Joins("JOIN "+model.QuotaPlan{}.TableName()+" AS plan ON plan.id = item.plan_id").
		Where("plan.id IN ? AND item.adjustment_quota > 0", planIDs).
		Group("item.user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.UserID] = row.ExecutedAt
	}
	return result, nil
}

func loadSpendSnapshot(logDB *gorm.DB, users []userSnapshot, cycleStart int64, observationStart int64, snapshotAt int64) (map[int]spendStats, error) {
	if !common.LogConsumeEnabled {
		return nil, errors.New("消费日志未启用，不能生成或执行自动调配方案")
	}
	if logDB == nil {
		return nil, errors.New("日志数据库未初始化")
	}
	stats := make(map[int]spendStats, len(users))
	if len(users) == 0 {
		return stats, nil
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
	if err := logDB.Model(&model.Log{}).
		Select(`user_id,
			COALESCE(SUM(CASE WHEN type = ? THEN -quota ELSE quota END), 0) AS period_spend,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN CASE WHEN type = ? THEN -quota ELSE quota END ELSE 0 END), 0) AS recent_spend,
			COALESCE(MAX(CASE WHEN type = ? AND created_at >= ? THEN created_at ELSE 0 END), 0) AS last_use`,
			model.LogTypeRefund, observationStart, model.LogTypeRefund, model.LogTypeConsume, observationStart).
		Where("type IN ? AND created_at >= ? AND created_at <= ? AND user_id IN ?",
			[]int{model.LogTypeConsume, model.LogTypeRefund}, cycleStart, snapshotAt, userIDs).
		Group("user_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.PeriodSpend < 0 || row.RecentSpend < 0 {
			return nil, fmt.Errorf("用户 %d 的消费统计不能为负数", row.UserID)
		}
		stats[row.UserID] = spendStats{
			PeriodSpend: row.PeriodSpend,
			RecentSpend: row.RecentSpend,
			LastUse:     row.LastUse,
		}
	}
	return stats, nil
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
	BasisText string
}

func generateAdjustmentItems(
	users []userSnapshot,
	profiles map[int]DemandProfile,
	stats map[int]spendStats,
	initialGrant int64,
	budget int64,
	params snapshotParams,
	ordinaryAllocationCap int64,
	occupiedBefore int64,
	lastPositiveAdjustments map[int]int64,
	concentrationBasisPoints int64,
) ([]userAdjustment, error) {
	minimumOrdinaryAdjustment := quotaPerUnit()
	items := make([]userAdjustment, 0, len(users))
	activeUsers := make([]userSnapshot, 0, len(users))
	usersByID := make(map[int]userSnapshot, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
		if user.Status == common.UserStatusEnabled {
			activeUsers = append(activeUsers, user)
		}
	}
	basicSafetyTarget, err := equalSafetyTarget(initialGrant, params.TotalWorkdays)
	if err != nil {
		return nil, err
	}
	positionCeiling, err := concentrationPositionCeiling(ordinaryAllocationCap, len(activeUsers), concentrationBasisPoints)
	if err != nil {
		return nil, err
	}
	requests := make([]requestRow, 0, len(activeUsers))
	increaseNeedIDs := make(map[int]bool)
	requestBasisTexts := make(map[int]string)
	for _, user := range activeUsers {
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		demandTarget, err := targetForDays(profile.WeeklyDemand, params.CalculationDaysHundred)
		if err != nil {
			return nil, fmt.Errorf("用户 %d 调增目标计算失败: %w", user.ID, err)
		}
		positionBefore, err := checkedSum(userStats.PeriodSpend, user.Quota)
		if err != nil {
			return nil, err
		}
		walletHeadroom, err := checkedAdd(int64(common.MaxWalletQuota), -user.Quota)
		if err != nil {
			return nil, err
		}
		positionHeadroom, err := checkedAdd(positionCeiling, -positionBefore)
		if err != nil {
			// A mathematical headroom above int64 is still bounded by the wallet limit.
			positionHeadroom = walletHeadroom
		}
		positionHeadroom = roundDownCent(max(int64(0), min(positionHeadroom, walletHeadroom)))
		safetyTopUp, err := roundUpCent(max(int64(0), basicSafetyTarget-user.Quota))
		if err != nil {
			return nil, err
		}
		safetyTopUp = min(safetyTopUp, positionHeadroom)
		demandHeadroom := positionHeadroom - safetyTopUp
		demandGap := int64(0)
		if demandHeadroom > 0 {
			balanceAfterSafety, err := checkedAdd(user.Quota, safetyTopUp)
			if err != nil {
				return nil, err
			}
			demandGap, err = checkedAdd(demandTarget, -balanceAfterSafety)
			if err != nil {
				return nil, err
			}
			demandGap = max(int64(0), demandGap)
		}
		demandTopUp := roundDownCent(min(demandGap, demandHeadroom))
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
			UserID: user.ID, Requested: requested,
			Balance: user.Quota, SafetyTarget: basicSafetyTarget, Target: target,
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
	lowUsageIDs := make(map[int]bool, len(activeUsers))
	for _, user := range activeUsers {
		lowUsageIDs[user.ID] = isOrdinaryLowUsage(stats[user.ID].RecentSpend, profiles[user.ID].WeeklyDemand, initialGrant)
	}
	ordinaryPotential := make([]ordinaryDecrease, 0, len(activeUsers))
	for _, user := range activeUsers {
		if increaseNeedIDs[user.ID] ||
			!hasObservationWindow(user.ObservationStartAt, params.SnapshotAt) || !lowUsageIDs[user.ID] {
			continue
		}
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		retained, err := retainedForRemainingWorkdays(baseRetained, profile.WeeklyDemand, params.RemainingWorkdays)
		if err != nil {
			return nil, err
		}
		reclaimable := max(int64(0), user.Quota-retained)
		amount := roundDownCent(min(reclaimable, budget))
		if !isMaterialAdjustment(amount, minimumOrdinaryAdjustment) {
			continue
		}
		usage := "本期无使用记录"
		if userStats.PeriodSpend > 0 {
			recent := fmt.Sprintf("本次观察期近期累计使用 %s", FormatQuota(userStats.RecentSpend))
			if userStats.RecentSpend == 0 {
				recent = "本次观察期近期无使用记录"
			}
			usage = fmt.Sprintf("本期累计使用 %s，%s", FormatQuota(userStats.PeriodSpend), recent)
		}
		rule := fmt.Sprintf("按本次观察期用量及余额接近用完时的修正用量取较高值，核定保留额度为 %s", FormatQuota(retained))
		if baseRetained >= retained {
			rule = fmt.Sprintf("按首次发放额度及剩余工作日核定保留额度为 %s", FormatQuota(retained))
		}
		ordinaryPotential = append(ordinaryPotential, ordinaryDecrease{
			UserID: user.ID, Amount: amount,
			BasisText: fmt.Sprintf("%s\n自上一份已执行方案以来已完成观察\n%s", usage, rule),
		})
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
	selectedOrdinary := ordinaryPotential
	for _, candidate := range selectedOrdinary {
		user := usersByID[candidate.UserID]
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		items = append(items, userAdjustment{
			UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
			Action: model.QuotaAdjustmentActionDecrease, SnapshotBalance: user.Quota,
			Adjustment: -candidate.Amount,
			CalculationData: map[string]interface{}{
				"weekly_demand": strconv.FormatInt(profile.WeeklyDemand, 10),
				"period_spend":  strconv.FormatInt(userStats.PeriodSpend, 10),
				"recent_spend":  strconv.FormatInt(userStats.RecentSpend, 10),
				"decrease_kind": "ordinary",
			},
			BasisText: candidate.BasisText,
		})
	}
	items = append(items, disabledItems...)

	requestedAmounts := make([]int64, 0, len(requests))
	for _, request := range requests {
		requestedAmounts = append(requestedAmounts, request.Requested)
	}
	requestTotal, err := checkedSum(requestedAmounts...)
	if err != nil {
		return nil, err
	}
	forcedTotal, err := negativeAdjustmentTotal(disabledItems)
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
	increaseCap := availableIncreaseCap(ordinaryAllocationCap, occupiedBefore, decreaseTotal)
	baseAllocations, err := allocateFairRequests(requests, min(requestTotal, increaseCap), minimumOrdinaryAdjustment)
	if err != nil {
		return nil, err
	}
	for _, user := range activeUsers {
		baseAmount := baseAllocations[user.ID]
		if baseAmount <= 0 {
			continue
		}
		profile := profiles[user.ID]
		userStats := stats[user.ID]
		basisText := requestBasisTexts[user.ID]
		action := model.QuotaAdjustmentActionIncrease
		if user.Quota == 0 {
			action = model.QuotaAdjustmentActionGrant
		}
		items = append(items, userAdjustment{
			UserID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Email: user.Email,
			Action: action, SnapshotBalance: user.Quota, Adjustment: baseAmount,
			CalculationData: map[string]interface{}{
				"weekly_demand": strconv.FormatInt(profile.WeeklyDemand, 10),
				"source":        profile.Source,
				"period_spend":  strconv.FormatInt(userStats.PeriodSpend, 10),
				"recent_spend":  strconv.FormatInt(userStats.RecentSpend, 10),
				"base_quota":    strconv.FormatInt(baseAmount, 10),
			},
			BasisText: basisText,
		})
	}

	return items, nil
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
		if after < -int64(common.MaxWalletQuota) || after > int64(common.MaxWalletQuota) {
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
