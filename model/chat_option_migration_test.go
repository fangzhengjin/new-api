package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateChatPresetsOptionConvertsLegacyDataAndKeepsRollbackBackup(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	legacyChats := `[{"Legacy":"https://legacy.example"},{"Disabled":{"url":"https://disabled.example","enabled":false}},{"CC Switch":"ccswitch"}]`
	require.NoError(t, db.Create(&[]Option{
		{Key: "Chats", Value: legacyChats},
		{Key: "InfiniteCanvasEnabled", Value: "true"},
		{Key: "InfiniteCanvasLaunchURL", Value: "https://canvas.example/image#code={code}"},
	}).Error)

	require.NoError(t, MigrateChatPresetsOption())

	migrated := requireOptionValue(t, db, "Chats")
	presets, err := setting.ParseChatsJSON(migrated)
	require.NoError(t, err)
	assert.Equal(t, []setting.ChatPreset{
		{Name: "Legacy", URL: "https://legacy.example", Enabled: true},
		{Name: "Disabled", URL: "https://disabled.example", Enabled: false},
		{Name: "无限画布", URL: "https://canvas.example/image#code={authCode}", Enabled: true, OpenMode: setting.ChatOpenModeNewTab},
	}, presets)
	assert.Equal(t, legacyChats, requireOptionValue(t, db, chatPresetsLegacyBackupOptionKey))
	assert.Equal(t, "true", requireOptionValue(t, db, "InfiniteCanvasEnabled"))
	assert.Equal(t, "https://canvas.example/image#code={code}", requireOptionValue(t, db, "InfiniteCanvasLaunchURL"))

	before, err := AllOption()
	require.NoError(t, err)
	require.NoError(t, MigrateChatPresetsOption())
	after, err := AllOption()
	require.NoError(t, err)
	assert.ElementsMatch(t, before, after)
}

func TestMigrateChatPresetsOptionPreservesMalformedSource(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	malformed := `[{"Broken":{"enabled":true}}]`
	require.NoError(t, db.Create(&Option{Key: "Chats", Value: malformed}).Error)

	err := MigrateChatPresetsOption()

	require.Error(t, err)
	assert.Equal(t, malformed, requireOptionValue(t, db, "Chats"))
	var backup Option
	assert.ErrorIs(t, db.Where(&Option{Key: chatPresetsLegacyBackupOptionKey}).First(&backup).Error, gorm.ErrRecordNotFound)
}

func TestMigrateChatPresetsOptionKeepsStandardCanvasEntryAuthoritative(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	standard := `[{"name":"无限画布","url":"https://new.example/image#code={authCode}","enabled":false,"icon":"Palette","open_mode":"new_tab"}]`
	require.NoError(t, db.Create(&[]Option{
		{Key: "Chats", Value: standard},
		{Key: "InfiniteCanvasEnabled", Value: "true"},
		{Key: "InfiniteCanvasLaunchURL", Value: "https://old.example/image#code={code}"},
	}).Error)

	require.NoError(t, MigrateChatPresetsOption())

	presets, err := setting.ParseChatsJSON(requireOptionValue(t, db, "Chats"))
	require.NoError(t, err)
	assert.Equal(t, []setting.ChatPreset{{
		Name: "无限画布", URL: "https://new.example/image#code={authCode}", Enabled: false, Icon: "Palette", OpenMode: setting.ChatOpenModeNewTab,
	}}, presets)
}

func TestRetiredCanvasOptionsRemainStoredButAreNotPublishedOrWritable(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	require.NoError(t, db.Create(&[]Option{
		{Key: "InfiniteCanvasEnabled", Value: "true"},
		{Key: "InfiniteCanvasLaunchURL", Value: "https://canvas.example/image#code={code}"},
	}).Error)
	previousMap := common.OptionMap
	common.OptionMap = map[string]string{
		"InfiniteCanvasEnabled":   "true",
		"InfiniteCanvasLaunchURL": "https://canvas.example/image#code={code}",
	}
	t.Cleanup(func() { common.OptionMap = previousMap })

	require.NoError(t, updateOptionMap("InfiniteCanvasEnabled", "true"))
	require.NoError(t, updateOptionMap("InfiniteCanvasLaunchURL", "https://canvas.example/image#code={code}"))
	_, enabledPublished := common.OptionMap["InfiniteCanvasEnabled"]
	_, urlPublished := common.OptionMap["InfiniteCanvasLaunchURL"]
	assert.False(t, enabledPublished)
	assert.False(t, urlPublished)
	assert.Error(t, UpdateOption("InfiniteCanvasEnabled", "false"))
	assert.Equal(t, "true", requireOptionValue(t, db, "InfiniteCanvasEnabled"))
}
