package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaCycleActiveKeyAllowsManyScheduledButOnlyOneActive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&QuotaCycle{}, &QuotaPlan{}, &QuotaItem{}))

	firstScheduled := QuotaCycle{CycleStartAt: 1, CycleEndAt: 2, BudgetQuota: 1, Status: QuotaCycleStatusScheduled}
	secondScheduled := QuotaCycle{CycleStartAt: 3, CycleEndAt: 4, BudgetQuota: 1, Status: QuotaCycleStatusScheduled}
	require.NoError(t, db.Create(&firstScheduled).Error)
	require.NoError(t, db.Create(&secondScheduled).Error)
	assert.Nil(t, firstScheduled.ActiveKey)
	assert.Nil(t, secondScheduled.ActiveKey)

	firstActive := QuotaCycle{CycleStartAt: 5, CycleEndAt: 6, BudgetQuota: 1, Status: QuotaCycleStatusActive}
	secondActive := QuotaCycle{CycleStartAt: 7, CycleEndAt: 8, BudgetQuota: 1, Status: QuotaCycleStatusActive}
	require.NoError(t, db.Create(&firstActive).Error)
	err = db.Create(&secondActive).Error
	require.Error(t, err)
}
