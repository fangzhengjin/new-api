package model

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordQuotaSettlementIsIdempotentAndIncludesWhitelistUsage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota-mode.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &QuotaCycle{}, &QuotaCycleSettlement{}))
	previousDB, previousLogDB := DB, LOG_DB
	previousMode := operation_setting.CompanyQuotaModeEnabled
	DB, LOG_DB = db, db
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		operation_setting.CompanyQuotaModeEnabled = previousMode
	})
	now := time.Now().Unix()
	cycle := QuotaCycle{CycleStartAt: now - 60, CycleEndAt: now + 60, BudgetQuota: 1_000_000, InitialGrantQuota: 100, Status: QuotaCycleStatusActive}
	require.NoError(t, db.Create(&cycle).Error)
	require.NoError(t, RecordQuotaSettlement("request-1", 1, 700, now))
	require.NoError(t, RecordQuotaSettlement("request-1", 1, 400, now))
	require.NoError(t, RecordQuotaSettlement("whitelist-request", 2, 300, now))
	var total int64
	require.NoError(t, db.Model(&QuotaCycleSettlement{}).Where("cycle_id = ?", cycle.Id).Select("COALESCE(SUM(quota), 0)").Scan(&total).Error)
	assert.Equal(t, int64(700), total)
	assert.Equal(t, int64(700), func() int64 { value, _ := SumQuotaCycleSettlement(db, cycle.Id, now); return value }())
}

func TestPersonalQuotaSourceIsRejectedInCompanyMode(t *testing.T) {
	previous := operation_setting.CompanyQuotaModeEnabled
	operation_setting.CompanyQuotaModeEnabled = true
	t.Cleanup(func() { operation_setting.CompanyQuotaModeEnabled = previous })
	user := &User{Id: 99001, Username: "company-mode-source", AffQuota: 1000}
	err := user.TransferAffQuotaToQuota(100)
	require.ErrorIs(t, err, ErrCompanyQuotaMode)
	assert.Equal(t, 1000, user.AffQuota)
}

func TestQuotaWhitelistMigrationMarksLegacyAliasesOnlyOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "quota-whitelist.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	users := []User{
		{Username: "demo", AffCode: "demo-migration", Status: 1},
		{Username: "alice", AffCode: "alice-migration", Status: 1},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Model(&User{}).Where("id = ?", users[1].Id).UpdateColumn("quota_whitelist", nil).Error)
	require.NoError(t, migrateQuotaWhitelistUsers(true))

	var migrated []User
	require.NoError(t, db.Order("id").Find(&migrated).Error)
	assert.True(t, migrated[0].QuotaWhitelist)
	assert.False(t, migrated[1].QuotaWhitelist)

	require.NoError(t, db.Model(&User{}).Where("id = ?", users[0].Id).Update("quota_whitelist", false).Error)
	require.NoError(t, migrateQuotaWhitelistUsers(false))
	require.NoError(t, db.First(&migrated[0], users[0].Id).Error)
	assert.False(t, migrated[0].QuotaWhitelist)
}
