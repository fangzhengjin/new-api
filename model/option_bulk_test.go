package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionsBulkPricingTransaction(t *testing.T) {
	for _, name := range []string{"mixed success", "mode and expression together", "negative price", "null map", "missing expression", "ordinary write failure", "pricing write failure", "ordinary only"} {
		t.Run(name, func(t *testing.T) {
			previousDB, previousOptions := DB, common.OptionMap
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			configured := config.GlobalConfig.ExportAllConfigs()
			previousPricing := map[string]string{
				"ModelPrice": ratio_setting.ModelPrice2JSONString(), "ModelRatio": ratio_setting.ModelRatio2JSONString(),
				"CompletionRatio": ratio_setting.CompletionRatio2JSONString(), "CacheRatio": ratio_setting.CacheRatio2JSONString(),
				"CreateCacheRatio": ratio_setting.CreateCacheRatio2JSONString(), "ImageRatio": ratio_setting.ImageRatio2JSONString(),
				"AudioRatio": ratio_setting.AudioRatio2JSONString(), "AudioCompletionRatio": ratio_setting.AudioCompletionRatio2JSONString(),
				"billing_setting.billing_mode": configured["billing_setting.billing_mode"],
				"billing_setting.billing_expr": configured["billing_setting.billing_expr"],
			}
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&Option{}, &Channel{}, &Ability{}, &Model{}, &Vendor{}))
			DB, common.OptionMap = db, make(map[string]string)
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, previousLogType)
			t.Cleanup(func() {
				for key, raw := range previousPricing {
					require.NoError(t, updateOptionMap(key, raw))
				}
				DB, common.OptionMap = previousDB, previousOptions
				common.SetDatabaseTypes(previousMainType, previousLogType)
				InvalidatePricingCache()
			})
			require.NoError(t, db.Create(&Option{Key: "BulkTestMarker", Value: "old"}).Error)
			common.OptionMap["BulkTestMarker"] = "old"
			updates := map[string]string{"BulkTestMarker": "new"}
			if name == "ordinary only" {
				require.NoError(t, UpdateOptionsBulk(updates))
				var rows []Option
				require.NoError(t, db.Find(&rows).Error)
				assert.Equal(t, []Option{{Key: "BulkTestMarker", Value: "new"}}, rows)
				return
			}
			for _, key := range modelPricingOptionKeys {
				raw := "{}"
				if key == "ModelPrice" {
					raw = `{"bulk-test-model":1}`
				}
				if name == "missing expression" {
					if key == "billing_setting.billing_mode" {
						raw = `{"bulk-test-model":"tiered_expr"}`
					}
					if key == "billing_setting.billing_expr" {
						raw = `{"bulk-test-model":"p * 2"}`
					}
				}
				require.NoError(t, db.Create(&Option{Key: key, Value: raw}).Error)
				require.NoError(t, updateOptionMap(key, raw))
			}
			var before []Option
			require.NoError(t, db.Order(commonKeyCol).Find(&before).Error)
			snapshot, err := GetModelPricingSnapshot([]string{"bulk-test-model"})
			require.NoError(t, err)
			updates["ModelPrice"] = `{"bulk-test-model":2}`
			switch name {
			case "mode and expression together":
				updates["billing_setting.billing_mode"] = `{"bulk-test-model":"tiered_expr"}`
				updates["billing_setting.billing_expr"] = `{"bulk-test-model":"tier(\"base\", p * 2)"}`
			case "negative price":
				updates["ModelPrice"] = `{"bulk-test-model":-1}`
			case "null map":
				updates["ModelPrice"] = `null`
			case "missing expression":
				updates["billing_setting.billing_expr"] = `{}`
			case "ordinary write failure":
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_bulk_marker BEFORE UPDATE ON options WHEN NEW.key = 'BulkTestMarker' BEGIN SELECT RAISE(ABORT, 'test ordinary write failure'); END`).Error)
			case "pricing write failure":
				require.NoError(t, db.Exec(`CREATE TRIGGER reject_bulk_price BEFORE UPDATE ON options WHEN NEW.key = 'ModelPrice' BEGIN SELECT RAISE(ABORT, 'test pricing write failure'); END`).Error)
			}
			err = UpdateOptionsBulk(updates)
			if name != "mixed success" && name != "mode and expression together" {
				require.Error(t, err)
				var after []Option
				require.NoError(t, db.Order(commonKeyCol).Find(&after).Error)
				assert.Equal(t, before, after)
				assert.Equal(t, "old", common.OptionMap["BulkTestMarker"])
				assert.JSONEq(t, `{"bulk-test-model":1}`, ratio_setting.ModelPrice2JSONString())
				return
			}
			require.NoError(t, err)
			var marker Option
			require.NoError(t, db.Where(&Option{Key: "BulkTestMarker"}).First(&marker).Error)
			assert.Equal(t, "new", marker.Value)
			assert.Equal(t, "new", common.OptionMap["BulkTestMarker"])
			assert.JSONEq(t, `{"bulk-test-model":2}`, ratio_setting.ModelPrice2JSONString())
			if name == "mode and expression together" {
				current, err := GetModelPricingSnapshot([]string{"bulk-test-model"})
				require.NoError(t, err)
				require.Len(t, current.Entries, 1)
				assert.Equal(t, "tiered_expr", current.Entries[0].Configured["billing_setting.billing_mode"])
				assert.Equal(t, `tier("base", p * 2)`, current.Entries[0].Configured["billing_setting.billing_expr"])
			}
			err = UpdateModelPricing([]ModelPricingChange{{
				ModelName: "bulk-test-model", ExpectedVersion: snapshot.Entries[0].Version,
				Pricing: PricingValues{"ModelPrice": float64(3)},
			}})
			require.ErrorIs(t, err, ErrModelPricingConflict)
			assert.JSONEq(t, `{"bulk-test-model":2}`, ratio_setting.ModelPrice2JSONString())
		})
	}
}

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
	require.NoError(t, db.First(&option, map[string]interface{}{"key": "SMTPServer"}).Error)
	assert.Equal(t, "old.example.com", option.Value)
	assert.ErrorIs(t, db.First(&Option{}, map[string]interface{}{"key": "ModelPrice"}).Error, gorm.ErrRecordNotFound)
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
