package quota

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	requiredQualifiedShadowCycles = 2
	enableCandidateConfirmation   = "确认启用候选额度算法"
	rollbackLegacyConfirmation    = "确认回退现行额度算法"
)

// AlgorithmStatus exposes the evidence-backed production-switch gate.
type AlgorithmStatus struct {
	LegacyVersion           string
	CurrentVersion          string
	CandidateVersion        string
	EnableConfirmation      string
	RollbackConfirmation    string
	RequiredQualifiedCycles int
	QualifiedCycleIDs       []int
	ActiveCycleID           *int
	RecoveryReady           bool
	DraftCount              int64
	CanSwitch               bool
	RollbackAllowed         bool
	CanRecordEvidence       bool
	Blockers                []string
}

// RecordShadowEvidence persists a non-PII final-window result without creating a quota plan.
func RecordShadowEvidence(params GenerateParams, createdBy string) (*model.QuotaShadowEvidence, error) {
	if params.StagePercent != 10_000 || !params.ThoroughRelease {
		return nil, errors.New("切换证据必须使用100%最终阶段和彻底释放参数")
	}
	comparison, err := CompareFairness(params)
	if err != nil {
		return nil, err
	}
	parameters, err := common.Marshal(map[string]interface{}{
		"cycle_id": params.CycleID, "stage_percent": params.StagePercent,
		"next_adjustment_at": params.NextAdjustmentAt, "basis_mode": params.BasisMode,
		"early_reclaim": params.EarlyReclaim, "reclaim_cap_percent": params.ReclaimCapPercent,
		"usage_bonus_percent": params.UsageBonusPercent, "thorough_release": params.ThoroughRelease,
	})
	if err != nil {
		return nil, err
	}
	metrics, err := common.Marshal(map[string]interface{}{
		"stage_cap_quota": comparison.StageCapQuota, "current": comparison.Current,
		"candidate": comparison.Candidate,
	})
	if err != nil {
		return nil, err
	}
	evidence := model.QuotaShadowEvidence{
		CycleId: params.CycleID, SnapshotAt: comparison.SnapshotAt,
		CurrentAlgorithmVersion:   comparison.CurrentAlgorithmVersion,
		CandidateAlgorithmVersion: comparison.CandidateAlgorithmVersion,
		StagePercent:              params.StagePercent, Parameters: string(parameters), Metrics: string(metrics),
		Qualified: comparison.CandidateQualified, CreatedAt: time.Now().Unix(), CreatedBy: createdBy,
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var cycle model.QuotaCycle
		if err := model.LockForUpdate(tx).First(&cycle, params.CycleID).Error; err != nil {
			return err
		}
		if cycle.Status != model.QuotaCycleStatusActive || comparison.SnapshotAt >= cycle.CycleEndAt {
			return errors.New("只有活跃周期可以记录影子切换证据")
		}
		if cycleAlgorithmVersion(&cycle) != LegacyAlgorithmVersion {
			return errors.New("候选算法已进入生产，无需继续记录切换证据")
		}
		if comparison.SnapshotAt < finalAdjustmentTime(cycle.CycleStartAt, cycle.CycleEndAt) {
			return errors.New("影子切换证据只能在最终调配窗口记录")
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "cycle_id"}, {Name: "candidate_algorithm_version"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"snapshot_at", "current_algorithm_version", "stage_percent", "parameters",
				"metrics", "qualified", "created_at", "created_by",
			}),
		}).Create(&evidence).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	if err := model.DB.Where("cycle_id = ? AND candidate_algorithm_version = ?", params.CycleID, CandidateAlgorithmVersion).First(&evidence).Error; err != nil {
		return nil, err
	}
	return &evidence, nil
}

func qualifiedShadowCycleIDs(tx *gorm.DB) ([]int, error) {
	ids := make([]int, 0)
	err := tx.Model(&model.QuotaShadowEvidence{}).
		Select("tool_quota_shadow_evidence.cycle_id").
		Joins("JOIN tool_quota_cycles ON tool_quota_cycles.id = tool_quota_shadow_evidence.cycle_id").
		Where("tool_quota_shadow_evidence.qualified = ? AND tool_quota_shadow_evidence.candidate_algorithm_version = ? AND tool_quota_cycles.status = ?",
			true, CandidateAlgorithmVersion, model.QuotaCycleStatusClosed).
		Order("tool_quota_cycles.cycle_end_at DESC").Scan(&ids).Error
	return ids, err
}

// GetAlgorithmStatus evaluates the switch and rollback gates from persisted evidence.
func GetAlgorithmStatus() (*AlgorithmStatus, error) {
	status := &AlgorithmStatus{
		LegacyVersion: LegacyAlgorithmVersion, CurrentVersion: LegacyAlgorithmVersion, CandidateVersion: CandidateAlgorithmVersion,
		EnableConfirmation: enableCandidateConfirmation, RollbackConfirmation: rollbackLegacyConfirmation,
		RequiredQualifiedCycles: requiredQualifiedShadowCycles, QualifiedCycleIDs: []int{}, Blockers: []string{},
	}
	ids, err := qualifiedShadowCycleIDs(model.DB)
	if err != nil {
		return nil, err
	}
	status.QualifiedCycleIDs = ids
	var active model.QuotaCycle
	now := time.Now().Unix()
	err = model.DB.Where("status = ? AND cycle_start_at <= ? AND cycle_end_at > ?", model.QuotaCycleStatusActive, now, now).First(&active).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		status.ActiveCycleID = &active.Id
		status.CurrentVersion = cycleAlgorithmVersion(&active)
		status.RecoveryReady = active.RecoveryReserveQuota > 0
		status.RollbackAllowed = status.CurrentVersion == CandidateAlgorithmVersion && active.LegacyRollbackAllowed
		status.CanRecordEvidence = status.CurrentVersion == LegacyAlgorithmVersion && now >= finalAdjustmentTime(active.CycleStartAt, active.CycleEndAt)
	} else {
		var latest model.QuotaCycle
		if latestErr := model.DB.Order("cycle_start_at DESC").First(&latest).Error; latestErr == nil {
			status.CurrentVersion = cycleAlgorithmVersion(&latest)
		} else if !errors.Is(latestErr, gorm.ErrRecordNotFound) {
			return nil, latestErr
		}
	}
	if err := model.DB.Model(&model.QuotaPlan{}).Where("status = ?", model.QuotaPlanStatusDraft).Count(&status.DraftCount).Error; err != nil {
		return nil, err
	}
	if !model.CompanyQuotaModeEnabled() {
		status.Blockers = append(status.Blockers, "公司额度模式未启用")
	}
	if status.ActiveCycleID == nil {
		status.Blockers = append(status.Blockers, "当前没有活跃周期")
	}
	if !status.RecoveryReady {
		status.Blockers = append(status.Blockers, "当前周期未配置正数小额恢复池")
	}
	if len(status.QualifiedCycleIDs) < requiredQualifiedShadowCycles {
		status.Blockers = append(status.Blockers, fmt.Sprintf("仍需%d个已完成周期的合格影子证据", requiredQualifiedShadowCycles-len(status.QualifiedCycleIDs)))
	}
	if status.DraftCount > 0 {
		status.Blockers = append(status.Blockers, fmt.Sprintf("仍有%d份待执行草稿", status.DraftCount))
	}
	status.CanSwitch = status.CurrentVersion == LegacyAlgorithmVersion && len(status.Blockers) == 0
	return status, nil
}

// SwitchProductionAlgorithm changes active and scheduled cycles without touching balances.
func SwitchProductionAlgorithm(targetVersion, confirmation, operator string) (*AlgorithmStatus, error) {
	if targetVersion != LegacyAlgorithmVersion && targetVersion != CandidateAlgorithmVersion {
		return nil, errors.New("目标算法版本不正确")
	}
	expectedConfirmation := enableCandidateConfirmation
	if targetVersion == LegacyAlgorithmVersion {
		expectedConfirmation = rollbackLegacyConfirmation
	}
	if confirmation != expectedConfirmation {
		return nil, errors.New("确认短语不正确")
	}
	if !model.CompanyQuotaModeEnabled() {
		return nil, errors.New("公司额度模式未启用")
	}
	now := time.Now().Unix()
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var active model.QuotaCycle
		if err := model.LockForUpdate(tx).
			Where("status = ? AND cycle_start_at <= ? AND cycle_end_at > ?", model.QuotaCycleStatusActive, now, now).
			First(&active).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("当前没有活跃周期")
			}
			return err
		}
		current := cycleAlgorithmVersion(&active)
		if current == targetVersion {
			return nil
		}
		if targetVersion == CandidateAlgorithmVersion {
			if active.RecoveryReserveQuota <= 0 {
				return errors.New("当前周期未配置正数小额恢复池")
			}
			ids, err := qualifiedShadowCycleIDs(tx)
			if err != nil {
				return err
			}
			if len(ids) < requiredQualifiedShadowCycles {
				return errors.New("候选算法尚未取得两个已完成周期的合格影子证据")
			}
			var drafts int64
			if err := tx.Model(&model.QuotaPlan{}).Where("status = ?", model.QuotaPlanStatusDraft).Count(&drafts).Error; err != nil {
				return err
			}
			if drafts > 0 {
				return fmt.Errorf("仍有%d份待执行草稿，请先取消后再切换", drafts)
			}
			if err := tx.Model(&model.QuotaCycle{}).Where("id = ?", active.Id).Updates(map[string]interface{}{
				"allocation_algorithm_version": CandidateAlgorithmVersion, "legacy_rollback_allowed": true,
				"updated_at": now, "updated_by": operator,
			}).Error; err != nil {
				return err
			}
			return tx.Model(&model.QuotaCycle{}).Where("status = ?", model.QuotaCycleStatusScheduled).Updates(map[string]interface{}{
				"allocation_algorithm_version": CandidateAlgorithmVersion, "legacy_rollback_allowed": false,
				"updated_at": now, "updated_by": operator,
			}).Error
		}
		if !active.LegacyRollbackAllowed {
			return errors.New("现行算法回退窗口已结束")
		}
		if err := tx.Model(&model.QuotaPlan{}).
			Where("status = ? AND algorithm_version = ?", model.QuotaPlanStatusDraft, CandidateAlgorithmVersion).
			Updates(map[string]interface{}{
				"status": model.QuotaPlanStatusCancelled, "cancelled_at": now,
				"cancelled_by": operator, "cancel_reason": "生产算法回退，候选算法草稿作废",
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.QuotaCycle{}).Where("id = ?", active.Id).Updates(map[string]interface{}{
			"allocation_algorithm_version": LegacyAlgorithmVersion, "legacy_rollback_allowed": false,
			"updated_at": now, "updated_by": operator,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.QuotaCycle{}).Where("status = ?", model.QuotaCycleStatusScheduled).Updates(map[string]interface{}{
			"allocation_algorithm_version": LegacyAlgorithmVersion, "legacy_rollback_allowed": false,
			"updated_at": now, "updated_by": operator,
		}).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	return GetAlgorithmStatus()
}
