package model

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const quotaToolLegacyLogPrefix = "quota-tool-log:"

// migrateQuotaToolData finishes the PostgreSQL-only import started by
// bin/migration_quota_tool_to_new_api.sql.
func migrateQuotaToolData() error {
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) ||
		!DB.Migrator().HasColumn(&QuotaCycle{}, "initial_stage_percent") {
		return nil
	}

	type cycleWindow struct {
		Id           int
		CycleStartAt int64
		CycleEndAt   int64
	}

	var migrationAt int64
	var migratedCount int64
	var migratedSpend int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '10s'").Error; err != nil {
			return fmt.Errorf("设置旧额度工具迁移锁超时失败: %w", err)
		}
		if err := tx.Exec(`LOCK TABLE
			tool_quota_cycles,
			tool_quota_adjustment_plans,
			tool_quota_adjustment_items,
			tool_quota_cycle_settlements,
			logs
			IN ACCESS EXCLUSIVE MODE`).Error; err != nil {
			return fmt.Errorf("锁定旧额度工具迁移表失败: %w", err)
		}
		migrationAt = time.Now().Unix()

		constraintStatements := []struct {
			operation string
			statement string
		}{
			{"移除旧方案类型约束", "ALTER TABLE tool_quota_adjustment_plans DROP CONSTRAINT IF EXISTS tool_quota_adjustment_plans_plan_type_check"},
			{"创建方案类型约束", "ALTER TABLE tool_quota_adjustment_plans ADD CONSTRAINT tool_quota_adjustment_plans_plan_type_check CHECK (plan_type IN ('initialization', 'adjustment', 'settlement'))"},
			{"移除旧调整动作约束", "ALTER TABLE tool_quota_adjustment_items DROP CONSTRAINT IF EXISTS tool_quota_adjustment_items_action_check"},
			{"创建调整动作约束", "ALTER TABLE tool_quota_adjustment_items ADD CONSTRAINT tool_quota_adjustment_items_action_check CHECK (action IN ('initialize', 'increase', 'decrease', 'grant', 'reclaim', 'restore'))"},
		}
		for _, constraint := range constraintStatements {
			if err := tx.Exec(constraint.statement).Error; err != nil {
				return fmt.Errorf("%s失败: %w", constraint.operation, err)
			}
		}

		var overlapCount int64
		if err := tx.Table("tool_quota_cycles AS left_cycle").
			Joins(`JOIN tool_quota_cycles AS right_cycle
				ON left_cycle.id < right_cycle.id
				AND left_cycle.cycle_start_at < right_cycle.cycle_end_at
				AND right_cycle.cycle_start_at < left_cycle.cycle_end_at`).
			Count(&overlapCount).Error; err != nil {
			return fmt.Errorf("检查旧额度周期重叠失败: %w", err)
		}
		if overlapCount != 0 {
			return fmt.Errorf("存在 %d 组重叠的旧额度周期，无法自动归属历史消费", overlapCount)
		}

		var cycles []cycleWindow
		if err := tx.Model(&QuotaCycle{}).
			Select("id", "cycle_start_at", "cycle_end_at").
			Order("cycle_start_at").Scan(&cycles).Error; err != nil {
			return fmt.Errorf("读取旧额度周期失败: %w", err)
		}
		for _, cycle := range cycles {
			var invalidSettlementCount int64
			if err := tx.Model(&QuotaCycleSettlement{}).
				Where("cycle_id = ? AND (billing_at < ? OR billing_at >= ?)", cycle.Id, cycle.CycleStartAt, cycle.CycleEndAt).
				Count(&invalidSettlementCount).Error; err != nil {
				return fmt.Errorf("检查周期 %d 结算时间失败: %w", cycle.Id, err)
			}
			if invalidSettlementCount != 0 {
				return fmt.Errorf("周期 %d 存在 %d 条越界结算记录", cycle.Id, invalidSettlementCount)
			}

			var firstCurrentSettlement sql.NullInt64
			if err := tx.Model(&QuotaCycleSettlement{}).
				Select("MIN(billing_at)").
				Where("cycle_id = ? AND business_key NOT LIKE ?", cycle.Id, quotaToolLegacyLogPrefix+"%").
				Scan(&firstCurrentSettlement).Error; err != nil {
				return fmt.Errorf("读取周期 %d 新账本起点失败: %w", cycle.Id, err)
			}

			candidateLogWhere := "logs.type = ? AND logs.created_at >= ? AND logs.created_at < ? AND logs.created_at <= ?"
			candidateLogArgs := []interface{}{LogTypeConsume, cycle.CycleStartAt, cycle.CycleEndAt, migrationAt}
			if firstCurrentSettlement.Valid {
				candidateLogWhere += " AND logs.created_at < ?"
				candidateLogArgs = append(candidateLogArgs, firstCurrentSettlement.Int64)
			}
			var invalidLogCount int64
			if err := tx.Model(&Log{}).
				Where(candidateLogWhere, candidateLogArgs...).
				Where("logs.quota < 0 OR logs.user_id <= 0").
				Count(&invalidLogCount).Error; err != nil {
				return fmt.Errorf("检查周期 %d 旧消费日志失败: %w", cycle.Id, err)
			}
			if invalidLogCount != 0 {
				return fmt.Errorf("周期 %d 存在 %d 条负额度或无效用户的旧消费日志", cycle.Id, invalidLogCount)
			}

			insertSQL := `INSERT INTO tool_quota_cycle_settlements
				(business_key, cycle_id, user_id, billing_at, quota, updated_at)
				SELECT 'quota-tool-log:' || logs.id::TEXT, ?, logs.user_id, logs.created_at, logs.quota, ?
				FROM logs
				WHERE ` + candidateLogWhere
			insertArgs := append([]interface{}{cycle.Id, migrationAt}, candidateLogArgs...)
			insertSQL += " ON CONFLICT (business_key) DO NOTHING"
			if err := tx.Exec(insertSQL, insertArgs...).Error; err != nil {
				return fmt.Errorf("补录周期 %d 旧消费失败: %w", cycle.Id, err)
			}

			var logSpend int64
			if err := tx.Model(&Log{}).
				Where(candidateLogWhere, candidateLogArgs...).
				Select("COALESCE(SUM(quota), 0)").Scan(&logSpend).Error; err != nil {
				return fmt.Errorf("汇总周期 %d 旧消费失败: %w", cycle.Id, err)
			}
			var settlementSpend int64
			if err := tx.Model(&QuotaCycleSettlement{}).
				Select("COALESCE(SUM(quota), 0)").
				Where("cycle_id = ? AND business_key LIKE ?", cycle.Id, quotaToolLegacyLogPrefix+"%").
				Scan(&settlementSpend).Error; err != nil {
				return fmt.Errorf("汇总周期 %d 旧消费补录失败: %w", cycle.Id, err)
			}
			if logSpend != settlementSpend {
				return fmt.Errorf("周期 %d 旧消费总额不一致：旧日志 %d，补录账本 %d", cycle.Id, logSpend, settlementSpend)
			}

			userMismatchSQL := `WITH log_by_user AS (
					SELECT user_id, SUM(quota) AS spend
					FROM logs
					WHERE ` + candidateLogWhere + `
					GROUP BY user_id
				), settlement_by_user AS (
					SELECT user_id, SUM(quota) AS spend
					FROM tool_quota_cycle_settlements
					WHERE cycle_id = ? AND business_key LIKE ?
					GROUP BY user_id
				)
				SELECT COUNT(*)
				FROM log_by_user
				FULL OUTER JOIN settlement_by_user USING (user_id)
				WHERE COALESCE(log_by_user.spend, 0) <> COALESCE(settlement_by_user.spend, 0)`
			userMismatchArgs := append(candidateLogArgs, cycle.Id, quotaToolLegacyLogPrefix+"%")
			var userMismatchCount int64
			if err := tx.Raw(userMismatchSQL, userMismatchArgs...).Scan(&userMismatchCount).Error; err != nil {
				return fmt.Errorf("核对周期 %d 逐用户消费失败: %w", cycle.Id, err)
			}
			if userMismatchCount != 0 {
				return fmt.Errorf("周期 %d 仍有 %d 个用户的新旧消费不一致", cycle.Id, userMismatchCount)
			}
		}

		var invalidLegacyCount int64
		if err := tx.Raw(`SELECT COUNT(*)
			FROM tool_quota_cycle_settlements AS settlement
			LEFT JOIN logs AS source_log
				ON settlement.business_key = 'quota-tool-log:' || source_log.id::TEXT
			LEFT JOIN tool_quota_cycles AS cycle ON cycle.id = settlement.cycle_id
			WHERE settlement.business_key LIKE 'quota-tool-log:%'
				AND (
					source_log.id IS NULL
					OR cycle.id IS NULL
					OR source_log.type <> ?
					OR source_log.created_at < cycle.cycle_start_at
					OR source_log.created_at >= cycle.cycle_end_at
					OR settlement.user_id <> source_log.user_id
					OR settlement.billing_at <> source_log.created_at
					OR settlement.quota <> source_log.quota
				)`, LogTypeConsume).Scan(&invalidLegacyCount).Error; err != nil {
			return fmt.Errorf("核对旧消费补录来源失败: %w", err)
		}
		if invalidLegacyCount != 0 {
			return fmt.Errorf("存在 %d 条无法追溯到原日志的旧消费补录", invalidLegacyCount)
		}

		if err := tx.Model(&QuotaCycleSettlement{}).
			Where("business_key LIKE ?", quotaToolLegacyLogPrefix+"%").
			Count(&migratedCount).Error; err != nil {
			return fmt.Errorf("统计旧消费补录数量失败: %w", err)
		}
		if err := tx.Model(&QuotaCycleSettlement{}).
			Select("COALESCE(SUM(quota), 0)").
			Where("business_key LIKE ?", quotaToolLegacyLogPrefix+"%").
			Scan(&migratedSpend).Error; err != nil {
			return fmt.Errorf("统计旧消费补录总额失败: %w", err)
		}
		if err := tx.Exec("ALTER TABLE tool_quota_cycles DROP COLUMN IF EXISTS initial_stage_percent").Error; err != nil {
			return fmt.Errorf("删除旧额度工具迁移标记失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	common.SysLog(fmt.Sprintf("旧额度工具消费迁移完成：%d 条，合计 %d", migratedCount, migratedSpend))
	return nil
}
