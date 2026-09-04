package model

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestChannelValidateSettingsRejectsInvalidHTTPTransport(t *testing.T) {
	tests := []struct {
		name    string
		setting dto.ChannelSettings
		wantErr string
	}{
		{
			name:    "auto with shards is valid",
			setting: dto.ChannelSettings{HTTPProtocol: "auto", HTTP2ConnectionShards: 4},
		},
		{
			name:    "http1 with shards greater than one rejected",
			setting: dto.ChannelSettings{HTTPProtocol: "http1", HTTP2ConnectionShards: 2},
			wantErr: "http2_connection_shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetSetting(tt.setting)
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestChannelValidateSettingsRejectsInvalidConcurrency(t *testing.T) {
	timeout := dto.MaxConcurrencyWaitTimeoutSeconds + 1
	channel := &Channel{}
	channel.SetSetting(dto.ChannelSettings{
		MaxConcurrency:                3,
		ConcurrencyWaitTimeoutSeconds: &timeout,
	})

	err := channel.ValidateSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concurrency_wait_timeout_seconds")
}

func TestChannelValidateSettingsRejectsInvalidSystemPromptMode(t *testing.T) {
	channel := &Channel{}
	channel.SetSetting(dto.ChannelSettings{SystemPromptMode: "replace"})

	err := channel.ValidateSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system_prompt_mode")
}

func TestChannelValidateSettingsRejectsInvalidUserHiddenModelMappingsType(t *testing.T) {
	setting := `{"user_hidden_model_mappings":["public-model",1]}`
	channel := &Channel{Setting: &setting}

	err := channel.ValidateSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_hidden_model_mappings")
}

func TestChannelReconcileUserHiddenModelMappingsPreservesUnknownSettings(t *testing.T) {
	mapping := `{"kept":"upstream-b","new":"upstream-c"}`
	tests := []struct {
		name       string
		setting    string
		wantHidden []string
	}{
		{
			name:       "updates the hidden list",
			setting:    `{"proxy":"","future_flag":{"enabled":true},"user_hidden_model_mappings":["removed","kept","kept"]}`,
			wantHidden: []string{"kept"},
		},
		{
			name:    "deletes an empty hidden list",
			setting: `{"proxy":"","future_flag":{"enabled":true},"user_hidden_model_mappings":["removed"]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &Channel{ModelMapping: &mapping, Setting: &test.setting}
			changed, err := channel.ReconcileUserHiddenModelMappings()
			require.NoError(t, err)
			require.True(t, changed)
			var saved struct {
				FutureFlag              map[string]bool `json:"future_flag"`
				UserHiddenModelMappings *[]string       `json:"user_hidden_model_mappings"`
			}
			require.NotNil(t, channel.Setting)
			require.NoError(t, common.UnmarshalJsonStr(*channel.Setting, &saved))
			assert.Equal(t, map[string]bool{"enabled": true}, saved.FutureFlag)
			if test.wantHidden == nil {
				assert.Nil(t, saved.UserHiddenModelMappings)
				return
			}
			require.NotNil(t, saved.UserHiddenModelMappings)
			assert.Equal(t, test.wantHidden, *saved.UserHiddenModelMappings)
		})
	}
}

func TestChannelWritesRejectInvalidModelMappingWithoutHiddenModels(t *testing.T) {
	assertChannelWritesRejectInvalidModelMappingWithoutHiddenModels(t)
}

func assertChannelWritesRejectInvalidModelMappingWithoutHiddenModels(t *testing.T) {
	t.Helper()
	t.Run("batch insert rejects a non-object mapping", func(t *testing.T) {
		clearModelMappingTestChannels(t)
		mapping := `[]`
		channel := Channel{
			Type:         constant.ChannelTypeOpenAI,
			Key:          "secret",
			Status:       common.ChannelStatusEnabled,
			Name:         "invalid insert mapping",
			Models:       "public-model",
			Group:        "default",
			ModelMapping: &mapping,
		}

		require.Error(t, BatchInsertChannels([]Channel{channel}))
		var count int64
		require.NoError(t, DB.Model(&Channel{}).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("single edit rejects a non-string mapping value", func(t *testing.T) {
		clearModelMappingTestChannels(t)
		mapping := `{"public-model":"upstream-model"}`
		channel := Channel{
			Type:         constant.ChannelTypeOpenAI,
			Key:          "secret",
			Status:       common.ChannelStatusEnabled,
			Name:         "invalid single edit mapping",
			Models:       "public-model",
			Group:        "default",
			ModelMapping: &mapping,
		}
		require.NoError(t, DB.Create(&channel).Error)

		invalidMapping := `{"public-model":1}`
		update := Channel{Id: channel.Id, ModelMapping: &invalidMapping}
		require.Error(t, update.UpdateWithModelMappingReconciliation())

		var saved Channel
		require.NoError(t, DB.First(&saved, channel.Id).Error)
		assert.Equal(t, mapping, saved.GetModelMapping())
	})

	t.Run("tag edit rejects an empty mapping entry", func(t *testing.T) {
		clearModelMappingTestChannels(t)
		tag := "invalid-mapping"
		mapping := `{"public-model":"upstream-model"}`
		channel := Channel{
			Type:         constant.ChannelTypeOpenAI,
			Key:          "secret",
			Status:       common.ChannelStatusEnabled,
			Name:         "invalid tag edit mapping",
			Models:       "public-model",
			Group:        "default",
			Tag:          &tag,
			ModelMapping: &mapping,
		}
		require.NoError(t, DB.Create(&channel).Error)

		invalidMapping := `{"":"upstream-model"}`
		require.Error(t, EditChannelByTag(tag, nil, &invalidMapping, nil, nil, nil, nil, nil, nil))

		var saved Channel
		require.NoError(t, DB.First(&saved, channel.Id).Error)
		assert.Equal(t, mapping, saved.GetModelMapping())
	})
}

func clearModelMappingTestChannels(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Ability{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Channel{}).Error)
}

func TestChannelUpdateReconcilesUserHiddenModelMappingsWithPartialFields(t *testing.T) {
	truncateTables(t)
	mapping := `{"removed":"upstream-a","kept":"upstream-b"}`
	channel := Channel{
		Type:         constant.ChannelTypeOpenAI,
		Key:          "secret",
		Status:       common.ChannelStatusEnabled,
		Name:         "partial mapping update",
		Models:       "removed,kept",
		Group:        "default",
		ModelMapping: &mapping,
	}
	channel.SetSetting(dto.ChannelSettings{
		UserHiddenModelMappings: []string{"removed", "kept"},
	})
	require.NoError(t, DB.Create(&channel).Error)

	nextMapping := `{"kept":"upstream-b"}`
	mappingUpdate := Channel{Id: channel.Id, ModelMapping: &nextMapping}
	require.NoError(t, mappingUpdate.UpdateWithModelMappingReconciliation())
	assert.Equal(t, []string{"kept"}, mappingUpdate.GetSetting().UserHiddenModelMappings)

	settingUpdate := Channel{Id: channel.Id}
	settingUpdate.SetSetting(dto.ChannelSettings{
		UserHiddenModelMappings: []string{"removed", "kept", "kept"},
	})
	require.NoError(t, settingUpdate.UpdateWithModelMappingReconciliation())
	assert.Equal(t, []string{"kept"}, settingUpdate.GetSetting().UserHiddenModelMappings)
}

func TestChannelUpdateDoesNotParseUnrelatedSetting(t *testing.T) {
	truncateTables(t)
	invalidSetting := `{"user_hidden_model_mappings":`
	channel := Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "secret",
		Status:  common.ChannelStatusEnabled,
		Name:    "unrelated update",
		Models:  "test-model",
		Group:   "default",
		Setting: &invalidSetting,
	}
	require.NoError(t, DB.Create(&channel).Error)

	channel.Name = "updated name"
	require.NoError(t, channel.Update())
	assert.Equal(t, "updated name", channel.Name)
}

func TestEditChannelByTagReconcilesUserHiddenModelMappings(t *testing.T) {
	truncateTables(t)
	tag := "mapped"
	mapping := `{"removed":"upstream-a","kept":"upstream-b"}`
	channel := Channel{
		Type:         constant.ChannelTypeOpenAI,
		Key:          "secret",
		Status:       common.ChannelStatusEnabled,
		Name:         "mapped channel",
		Models:       "removed,kept",
		Group:        "default",
		Tag:          &tag,
		ModelMapping: &mapping,
	}
	channel.SetSetting(dto.ChannelSettings{
		UserHiddenModelMappings: []string{"removed", "kept"},
	})
	require.NoError(t, DB.Create(&channel).Error)

	nextMapping := `{"kept":"upstream-b"}`
	nextModels := "kept"
	require.NoError(t, EditChannelByTag(tag, nil, &nextMapping, &nextModels, nil, nil, nil, nil, nil))

	var saved Channel
	require.NoError(t, DB.First(&saved, channel.Id).Error)
	assert.Equal(t, nextModels, saved.Models)
	assert.Equal(t, []string{"kept"}, saved.GetSetting().UserHiddenModelMappings)
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, "kept", abilities[0].Model)
}

func TestEditChannelByTagPreservesExplicitZeroWeight(t *testing.T) {
	truncateTables(t)
	assertEditChannelByTagPreservesExplicitZeroWeight(t, DB)
}

func assertEditChannelByTagPreservesExplicitZeroWeight(t *testing.T, db *gorm.DB) {
	t.Helper()
	tag := "zero-weight-" + strings.ReplaceAll(t.Name(), "/", "-")
	initialWeight := uint(5)
	mapping := `{"model":"upstream"}`
	channel := Channel{
		Type:         constant.ChannelTypeOpenAI,
		Key:          "secret",
		Status:       common.ChannelStatusEnabled,
		Name:         "zero weight channel",
		Weight:       &initialWeight,
		Models:       "model",
		Group:        "default",
		Tag:          &tag,
		ModelMapping: &mapping,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(db))
	t.Cleanup(func() {
		_ = db.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
		_ = db.Delete(&Channel{}, channel.Id).Error
	})

	nextMapping := `{"model":"upstream-new"}`
	zeroWeight := uint(0)
	require.NoError(t, EditChannelByTag(tag, nil, &nextMapping, nil, nil, nil, &zeroWeight, nil, nil))

	var savedChannel Channel
	require.NoError(t, db.First(&savedChannel, channel.Id).Error)
	assert.Equal(t, 0, savedChannel.GetWeight())
	var savedAbility Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&savedAbility).Error)
	assert.Equal(t, uint(0), savedAbility.Weight)
}

func TestModelMappingVisibilityPersistenceConfiguredDatabases(t *testing.T) {
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
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

			previousDB := DB
			previousMainDatabaseType := common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(test.databaseType)
			initCol()
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousMainDatabaseType)
				initCol()
			})
			assertChannelWritesRejectInvalidModelMappingWithoutHiddenModels(t)
			assertEditChannelByTagPreservesExplicitZeroWeight(t, db)

			mapping := `{"removed":"upstream-a","kept":"upstream-b"}`
			channel := Channel{
				Type:         constant.ChannelTypeOpenAI,
				Key:          "secret",
				Status:       common.ChannelStatusEnabled,
				Name:         "mapped channel",
				Models:       "removed,kept",
				Group:        "default",
				ModelMapping: &mapping,
			}
			channel.SetSetting(dto.ChannelSettings{
				UserHiddenModelMappings: []string{"removed", "kept"},
			})
			require.NoError(t, DB.Create(&channel).Error)
			t.Cleanup(func() {
				_ = db.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
				_ = db.Delete(&Channel{}, channel.Id).Error
			})

			nextMapping := `{"kept":"upstream-b"}`
			mappingUpdate := Channel{Id: channel.Id, ModelMapping: &nextMapping}
			require.NoError(t, mappingUpdate.UpdateWithModelMappingReconciliation())
			assert.Equal(t, []string{"kept"}, mappingUpdate.GetSetting().UserHiddenModelMappings)

			settingUpdate := Channel{Id: channel.Id}
			settingUpdate.SetSetting(dto.ChannelSettings{
				UserHiddenModelMappings: []string{"removed", "kept", "kept"},
			})
			require.NoError(t, settingUpdate.UpdateWithModelMappingReconciliation())
			assert.Equal(t, []string{"kept"}, settingUpdate.GetSetting().UserHiddenModelMappings)

			if test.databaseType != common.DatabaseTypePostgreSQL {
				return
			}

			insertDB, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			insertSQLDB, err := insertDB.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = insertSQLDB.Close() })

			tag := fmt.Sprintf("model-mapping-snapshot-%d", channel.Id)
			taggedMapping := `{"removed":"upstream-a","kept":"upstream-b"}`
			taggedChannel := Channel{
				Type:         constant.ChannelTypeOpenAI,
				Key:          "secret",
				Status:       common.ChannelStatusEnabled,
				Name:         "tagged channel",
				Models:       "removed,kept",
				Group:        "default",
				Tag:          &tag,
				ModelMapping: &taggedMapping,
			}
			taggedChannel.SetSetting(dto.ChannelSettings{
				UserHiddenModelMappings: []string{"removed", "kept"},
			})
			require.NoError(t, db.Create(&taggedChannel).Error)
			require.NoError(t, taggedChannel.AddAbilities(db))
			var concurrentChannel Channel
			t.Cleanup(func() {
				channelIDs := []int{taggedChannel.Id}
				if concurrentChannel.Id != 0 {
					channelIDs = append(channelIDs, concurrentChannel.Id)
				}
				_ = insertDB.Where("channel_id IN ?", channelIDs).Delete(&Ability{}).Error
				_ = insertDB.Delete(&Channel{}, channelIDs).Error
			})

			queryCompleted := make(chan struct{})
			continueUpdate := make(chan struct{})
			const callbackName = "test:edit_channel_by_tag_snapshot"
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if _, ok := tx.Statement.Dest.(*[]Channel); !ok {
					return
				}
				// Pause after the locked query so the inserted row is outside this edit's snapshot.
				close(queryCompleted)
				<-continueUpdate
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

			nextTagMapping := `{"kept":"upstream-b"}`
			newTag := tag + "-updated"
			editDone := make(chan error, 1)
			go func() {
				editDone <- EditChannelByTag(tag, &newTag, &nextTagMapping, nil, nil, nil, nil, nil, nil)
			}()
			select {
			case <-queryCompleted:
			case err := <-editDone:
				require.NoError(t, err)
				require.Fail(t, "tag edit returned before selecting channels")
			}

			concurrentChannel = Channel{
				Type:         constant.ChannelTypeOpenAI,
				Key:          "secret",
				Status:       common.ChannelStatusEnabled,
				Name:         "concurrent tagged channel",
				Models:       "removed,kept",
				Group:        "default",
				Tag:          &tag,
				ModelMapping: &taggedMapping,
			}
			concurrentChannel.SetSetting(dto.ChannelSettings{
				UserHiddenModelMappings: []string{"removed", "kept"},
			})
			insertErr := insertDB.Create(&concurrentChannel).Error
			var addAbilityErr error
			if insertErr == nil {
				addAbilityErr = concurrentChannel.AddAbilities(insertDB)
			}
			close(continueUpdate)
			require.NoError(t, insertErr)
			require.NoError(t, addAbilityErr)
			require.NoError(t, <-editDone)

			var savedConcurrent Channel
			require.NoError(t, insertDB.First(&savedConcurrent, concurrentChannel.Id).Error)
			assert.Equal(t, taggedMapping, savedConcurrent.GetModelMapping())
			assert.Equal(t, tag, savedConcurrent.GetTag())
			assert.Equal(t, []string{"removed", "kept"}, savedConcurrent.GetSetting().UserHiddenModelMappings)
			var savedConcurrentAbility Ability
			require.NoError(t, insertDB.Where("channel_id = ?", concurrentChannel.Id).First(&savedConcurrentAbility).Error)
			require.NotNil(t, savedConcurrentAbility.Tag)
			assert.Equal(t, tag, *savedConcurrentAbility.Tag)
		})
	}
}

func TestAdvancedCustomChannelRequiresModelListRouteOnlyWhenUpdateChecksEnabled(t *testing.T) {
	inferenceRoute := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/chat/completions",
		UpstreamPath: "/v1/chat/completions",
		Converter:    "none",
	}

	tests := []struct {
		name          string
		checksEnabled bool
		routes        []dto.AdvancedCustomRoute
		wantErr       string
	}{
		{
			name:   "legacy channel without discovery route remains valid",
			routes: []dto.AdvancedCustomRoute{inferenceRoute},
		},
		{
			name:          "enabled checks require discovery route",
			checksEnabled: true,
			routes:        []dto.AdvancedCustomRoute{inferenceRoute},
			wantErr:       dto.AdvancedCustomModelListPath,
		},
		{
			name:          "enabled checks accept discovery route",
			checksEnabled: true,
			routes: []dto.AdvancedCustomRoute{
				inferenceRoute,
				{
					IncomingPath: dto.AdvancedCustomModelListPath,
					UpstreamPath: dto.AdvancedCustomModelListPath,
					Converter:    "none",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: constant.ChannelTypeAdvancedCustom}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				UpstreamModelUpdateCheckEnabled: tt.checksEnabled,
				AdvancedCustom: &dto.AdvancedCustomConfig{
					Routes: tt.routes,
				},
			})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
