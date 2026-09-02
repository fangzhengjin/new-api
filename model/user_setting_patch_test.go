package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func useUserSettingTestDB(t *testing.T, db *gorm.DB, databaseType common.DatabaseType) {
	t.Helper()
	previousDB := DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	DB = db
	common.RedisEnabled = false
	common.SetDatabaseTypes(databaseType, previousLogDatabaseType)
	t.Cleanup(func() {
		DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
	})
}

func assertPatchUserSettingPreservesUnrelatedValues(t *testing.T, db *gorm.DB) {
	t.Helper()

	original := dto.UserSetting{Language: "zh", BillingPreference: "subscription"}
	bytes, err := common.Marshal(original)
	require.NoError(t, err)
	user := User{Username: "limit-" + common.GetUUID()[:8], Role: common.RoleCommonUser, Setting: string(bytes)}
	require.NoError(t, db.Create(&user).Error)
	t.Cleanup(func() { _ = db.Unscoped().Delete(&user).Error })
	override := 25
	concurrencyOverride := 4

	require.NoError(t, PatchUserSetting(user.Id, func(current *dto.UserSetting) {
		current.ModelRequestRateLimitCount = &override
		current.ModelRequestConcurrencyLimit = &concurrencyOverride
	}))
	updated, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	settings := updated.GetSetting()
	assert.Equal(t, "zh", settings.Language)
	assert.Equal(t, "subscription", settings.BillingPreference)
	require.NotNil(t, settings.ModelRequestRateLimitCount)
	assert.Equal(t, 25, *settings.ModelRequestRateLimitCount)
	require.NotNil(t, settings.ModelRequestConcurrencyLimit)
	assert.Equal(t, 4, *settings.ModelRequestConcurrencyLimit)
}

func TestPatchUserSettingPreservesUnrelatedValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	useUserSettingTestDB(t, db, common.DatabaseTypeSQLite)
	assertPatchUserSettingPreservesUnrelatedValues(t, db)
}

func TestPatchUserSettingConfiguredDatabases(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		databaseType common.DatabaseType
		dialector    func(string) gorm.Dialector
	}{
		{name: "mysql", env: "TEST_MYSQL_DSN", databaseType: common.DatabaseTypeMySQL, dialector: mysql.Open},
		{name: "postgres", env: "TEST_POSTGRES_DSN", databaseType: common.DatabaseTypePostgreSQL, dialector: func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.env))
			if dsn == "" {
				t.Skip(test.env + " is not configured")
			}
			db, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.NoError(t, db.AutoMigrate(&User{}))
			useUserSettingTestDB(t, db, test.databaseType)
			assertPatchUserSettingPreservesUnrelatedValues(t, db)
		})
	}
}

func TestPatchUserSettingRejectsInvalidStoredJSONWithoutOverwritingIt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	useUserSettingTestDB(t, db, common.DatabaseTypeSQLite)

	user := User{Username: "invalid-setting-user", Role: common.RoleCommonUser, Setting: "not-json"}
	require.NoError(t, db.Create(&user).Error)
	err = PatchUserSetting(user.Id, func(current *dto.UserSetting) {
		current.Language = "en"
	})
	require.Error(t, err)
	updated, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "not-json", updated.Setting)
}
