package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionsBulkRejectsInvalidJSONBeforeWriting(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "SMTPServer", Value: "old.example.com"}).Error)
	DB = db
	t.Cleanup(func() { DB = previousDB })

	err = UpdateOptionsBulk(map[string]string{
		"SMTPServer": "new.example.com",
		"ModelPrice": "{invalid",
	})
	require.Error(t, err)

	var option Option
	require.NoError(t, db.First(&option, "key = ?", "SMTPServer").Error)
	assert.Equal(t, "old.example.com", option.Value)
	assert.ErrorIs(t, db.First(&Option{}, "key = ?", "ModelPrice").Error, gorm.ErrRecordNotFound)
}
func TestInitOptionMapConvertsLegacyRequestHeaderRulesWithoutWritingDatabase(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&[]Option{
		{Key: operation_setting.LegacyRequestHeaderIgnoredHeadersKey, Value: "X-Noise-*"},
		{Key: operation_setting.LegacyRequestHeaderBlockedHeadersKey, Value: "X-Noise-*\nX-Block-*"},
	}).Error)
	DB = db
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	InitOptionMap()
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[operation_setting.RequestHeaderRulesOptionKey]
	common.OptionMapRWMutex.RUnlock()
	rules, err := operation_setting.ParseRequestHeaderRules(value)
	require.NoError(t, err)
	assert.Equal(t, []operation_setting.RequestHeaderRule{
		{Name: "X-Noise-*", Record: false, Forward: false},
		{Name: "X-Block-*", Record: true, Forward: false},
		{Name: "CF-*", Record: false, Forward: false},
		{Name: "EO-*", Record: false, Forward: false},
		{Name: "Ali-*", Record: false, Forward: false},
		{Name: "ESA-*", Record: false, Forward: false},
		{Name: "TLS-Hash", Record: false, Forward: false},
		{Name: "TLS-JA3", Record: false, Forward: false},
		{Name: "TLS-JA4", Record: false, Forward: false},
	}, rules)
	assert.ErrorIs(t, db.First(&Option{}, "key = ?", operation_setting.RequestHeaderRulesOptionKey).Error, gorm.ErrRecordNotFound)
}

func TestValidateOptionValueRejectsLegacyRequestHeaderRuleWrites(t *testing.T) {
	for _, key := range []string{
		operation_setting.LegacyRequestHeaderIgnoredHeadersKey,
		operation_setting.LegacyRequestHeaderBlockedHeadersKey,
	} {
		require.Error(t, validateOptionValue(key, "X-Test"), key)
	}
}
func TestValidateOptionValueRejectsReadOnlyDefaults(t *testing.T) {
	for _, key := range []string{
		operation_setting.RequestHeaderRulesDefaultOptionKey,
		operation_setting.RequestHeaderCDNRuleGroupsOptionKey,
		operation_setting.RequestHeaderSystemRulesOptionKey,
	} {
		require.Error(t, validateOptionValue(key, "changed"), key)
	}
}
