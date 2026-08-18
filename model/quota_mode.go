package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCompanyQuotaMode = errors.New("公司额度管理模式已开启，个人额度获取功能已停用")

func CompanyQuotaModeEnabled() bool {
	return operation_setting.CompanyQuotaModeEnabled
}

// RejectPersonalQuotaSource rejects user-controlled balance sources in company mode.
func RejectPersonalQuotaSource() error {
	if CompanyQuotaModeEnabled() {
		return ErrCompanyQuotaMode
	}
	return nil
}

// QuotaCycleSettlement stores the final net quota for one billing business key.
type QuotaCycleSettlement struct {
	Id          int    `json:"id" gorm:"primaryKey"`
	BusinessKey string `json:"business_key" gorm:"type:varchar(191);uniqueIndex;not null"`
	CycleId     int    `json:"cycle_id" gorm:"index;not null"`
	UserId      int    `json:"user_id" gorm:"index;not null"`
	BillingAt   int64  `json:"billing_at" gorm:"bigint;index;not null"`
	Quota       int64  `json:"quota" gorm:"bigint;not null"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;not null"`
}

func (QuotaCycleSettlement) TableName() string { return "tool_quota_cycle_settlements" }

func RecordQuotaSettlement(businessKey string, userId int, quota int64, billingAt int64) error {
	if !CompanyQuotaModeEnabled() && !DB.Migrator().HasTable(&QuotaCycle{}) {
		return nil
	}
	if businessKey == "" || userId <= 0 || quota < 0 || billingAt <= 0 {
		if !CompanyQuotaModeEnabled() {
			return nil
		}
		return errors.New("结算消费记录参数无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var cycle QuotaCycle
		err := tx.Where("cycle_start_at <= ? AND cycle_end_at > ?", billingAt, billingAt).
			Order("cycle_start_at DESC").First(&cycle).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) && !CompanyQuotaModeEnabled() {
				return nil
			}
			return err
		}
		record := QuotaCycleSettlement{BusinessKey: businessKey, CycleId: cycle.Id, UserId: userId, BillingAt: billingAt, Quota: quota, UpdatedAt: time.Now().Unix()}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "business_key"}},
			DoNothing: true,
		}).Create(&record)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			return nil
		}
		if err := lockForUpdate(tx).Where("business_key = ?", businessKey).First(&record).Error; err != nil {
			return err
		}
		if record.CycleId != cycle.Id || record.UserId != userId || record.BillingAt != billingAt {
			return errors.New("结算业务键重复且归属不一致")
		}
		return tx.Model(&record).Updates(map[string]interface{}{"quota": quota, "updated_at": time.Now().Unix()}).Error
	})
}

// EnsureQuotaSettlementCycle verifies that company-mode billing has one cycle owner.
func EnsureQuotaSettlementCycle(billingAt int64) error {
	if !CompanyQuotaModeEnabled() {
		return nil
	}
	var count int64
	if err := DB.Model(&QuotaCycle{}).Where("cycle_start_at <= ? AND cycle_end_at > ?", billingAt, billingAt).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errors.New("计费时间不属于唯一额度周期")
	}
	return nil
}

// SumQuotaCycleSettlement returns the authoritative net spend for a cycle snapshot.
func SumQuotaCycleSettlement(tx *gorm.DB, cycleId int, snapshotAt int64) (int64, error) {
	var total int64
	query := tx.Model(&QuotaCycleSettlement{}).Where("cycle_id = ?", cycleId)
	if snapshotAt > 0 {
		query = query.Where("billing_at <= ?", snapshotAt)
	}
	if err := query.Select("COALESCE(SUM(quota), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	if total < 0 {
		return 0, errors.New("本期消费总额不能为负数")
	}
	return total, nil
}
