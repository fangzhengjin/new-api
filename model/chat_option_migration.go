package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

const (
	chatPresetsLegacyBackupOptionKey = "ChatsLegacyBackup"
	chatPresetsAbsentBackupValue     = "__OPTION_ABSENT__"
)

// MigrateChatPresetsOption converts legacy chat and Infinite Canvas options in one transaction.
func MigrateChatPresetsOption() error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var chatsOption Option
		chatsErr := tx.Where(&Option{Key: "Chats"}).First(&chatsOption).Error
		if chatsErr != nil && !errors.Is(chatsErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read Chats: %w", chatsErr)
		}

		var enabledOption Option
		enabledErr := tx.Where(&Option{Key: "InfiniteCanvasEnabled"}).First(&enabledOption).Error
		if enabledErr != nil && !errors.Is(enabledErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read InfiniteCanvasEnabled: %w", enabledErr)
		}
		var urlOption Option
		urlErr := tx.Where(&Option{Key: "InfiniteCanvasLaunchURL"}).First(&urlOption).Error
		if urlErr != nil && !errors.Is(urlErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read InfiniteCanvasLaunchURL: %w", urlErr)
		}
		if errors.Is(chatsErr, gorm.ErrRecordNotFound) && errors.Is(enabledErr, gorm.ErrRecordNotFound) && errors.Is(urlErr, gorm.ErrRecordNotFound) {
			return nil
		}

		rawChats := chatsOption.Value
		if errors.Is(chatsErr, gorm.ErrRecordNotFound) {
			rawChats = setting.Chats2JsonString()
		}
		presets, changed, err := normalizeLegacyChatPresets(rawChats)
		if err != nil {
			return err
		}

		canvasEnabled := false
		if enabledErr == nil {
			canvasEnabled, err = strconv.ParseBool(strings.TrimSpace(enabledOption.Value))
			if err != nil {
				return fmt.Errorf("InfiniteCanvasEnabled 必须是布尔值: %w", err)
			}
		}
		canvasURL := ""
		if urlErr == nil {
			canvasURL = strings.TrimSpace(urlOption.Value)
		}
		if canvasURL == "" && canvasEnabled {
			return errors.New("旧无限画布已启用，但跳转地址为空")
		}
		if canvasURL != "" && !hasCanvasPreset(presets) {
			presets = append(presets, setting.ChatPreset{
				Name:     "无限画布",
				URL:      strings.ReplaceAll(canvasURL, "{code}", "{authCode}"),
				Enabled:  canvasEnabled,
				OpenMode: setting.ChatOpenModeNewTab,
			})
			changed = true
		}

		encoded, err := common.Marshal(presets)
		if err != nil {
			return fmt.Errorf("encode migrated Chats: %w", err)
		}
		if _, err = setting.ParseChatsJSON(string(encoded)); err != nil {
			return fmt.Errorf("validate migrated Chats: %w", err)
		}
		if !changed {
			return nil
		}

		var backup Option
		backupErr := tx.Where(&Option{Key: chatPresetsLegacyBackupOptionKey}).First(&backup).Error
		if backupErr != nil && !errors.Is(backupErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read Chats rollback backup: %w", backupErr)
		}
		if errors.Is(backupErr, gorm.ErrRecordNotFound) {
			backup = Option{Key: chatPresetsLegacyBackupOptionKey, Value: rawChats}
			if errors.Is(chatsErr, gorm.ErrRecordNotFound) {
				backup.Value = chatPresetsAbsentBackupValue
			}
			if err = tx.Create(&backup).Error; err != nil {
				return fmt.Errorf("write Chats rollback backup: %w", err)
			}
		}

		chatsOption.Key = "Chats"
		chatsOption.Value = string(encoded)
		if err = tx.Save(&chatsOption).Error; err != nil {
			return fmt.Errorf("write migrated Chats: %w", err)
		}
		return nil
	})
}

func normalizeLegacyChatPresets(raw string) ([]setting.ChatPreset, bool, error) {
	var items []map[string]any
	if err := common.UnmarshalJsonStr(raw, &items); err != nil || items == nil {
		if err == nil {
			err = errors.New("value is not an array")
		}
		return nil, false, fmt.Errorf("旧 Chats 无法解析: %w", err)
	}
	presets := make([]setting.ChatPreset, 0, len(items))
	changed := false
	for index, item := range items {
		preset, standard, err := normalizeLegacyChatPreset(item)
		if err != nil {
			return nil, false, fmt.Errorf("旧 Chats 第 %d 项无效: %w", index+1, err)
		}
		if preset.Name == "CC Switch" && preset.URL == "ccswitch" {
			changed = true
			continue
		}
		if !standard {
			changed = true
		}
		trimmedName := strings.TrimSpace(preset.Name)
		trimmedURL := strings.TrimSpace(preset.URL)
		updatedURL := strings.ReplaceAll(trimmedURL, "{code}", "{authCode}")
		if trimmedName != preset.Name || updatedURL != preset.URL {
			changed = true
		}
		preset.Name = trimmedName
		preset.URL = updatedURL
		presets = append(presets, preset)
	}
	encoded, err := common.Marshal(presets)
	if err != nil {
		return nil, false, err
	}
	if _, err = setting.ParseChatsJSON(string(encoded)); err != nil {
		return nil, false, err
	}
	return presets, changed, nil
}

func normalizeLegacyChatPreset(item map[string]any) (setting.ChatPreset, bool, error) {
	if _, hasName := item["name"]; hasName {
		if _, hasEnabled := item["enabled"]; hasEnabled {
			encoded, err := common.Marshal([]map[string]any{item})
			if err != nil {
				return setting.ChatPreset{}, true, err
			}
			presets, err := setting.ParseChatsJSON(string(encoded))
			if err != nil {
				return setting.ChatPreset{}, true, err
			}
			return presets[0], true, nil
		}
		for key := range item {
			if key != "name" && key != "url" && key != "enabled" && key != "open_mode" {
				return setting.ChatPreset{}, true, fmt.Errorf("包含不支持的字段 %q", key)
			}
		}
		name, nameOK := item["name"].(string)
		chatURL, urlOK := item["url"].(string)
		if !nameOK || !urlOK {
			return setting.ChatPreset{}, true, errors.New("name 和 url 必须是字符串")
		}
		mode, err := legacyChatOpenMode(item)
		if err != nil {
			return setting.ChatPreset{}, true, err
		}
		return setting.ChatPreset{Name: name, URL: chatURL, Enabled: true, OpenMode: mode}, false, nil
	}

	if len(item) != 1 {
		return setting.ChatPreset{}, false, errors.New("旧格式必须且只能包含一个名称")
	}
	for name, value := range item {
		if chatURL, ok := value.(string); ok {
			return setting.ChatPreset{Name: name, URL: chatURL, Enabled: true}, false, nil
		}
		config, ok := value.(map[string]any)
		if !ok {
			return setting.ChatPreset{}, false, errors.New("旧格式值必须是 URL 或配置对象")
		}
		for key := range config {
			if key != "url" && key != "enabled" {
				return setting.ChatPreset{}, false, fmt.Errorf("包含不支持的字段 %q", key)
			}
		}
		chatURL, ok := config["url"].(string)
		if !ok {
			return setting.ChatPreset{}, false, errors.New("url 必须是字符串")
		}
		enabled := true
		if value, exists := config["enabled"]; exists {
			enabled, ok = value.(bool)
			if !ok {
				return setting.ChatPreset{}, false, errors.New("enabled 必须是布尔值")
			}
		}
		return setting.ChatPreset{Name: name, URL: chatURL, Enabled: enabled}, false, nil
	}
	return setting.ChatPreset{}, false, errors.New("聊天预设不能为空")
}

func legacyChatOpenMode(item map[string]any) (string, error) {
	value, exists := item["open_mode"]
	if !exists {
		return "", nil
	}
	mode, ok := value.(string)
	if !ok {
		return "", errors.New("open_mode 必须是字符串")
	}
	return mode, nil
}

func hasCanvasPreset(presets []setting.ChatPreset) bool {
	for _, preset := range presets {
		if preset.Name == "无限画布" || preset.Name == "Infinite Canvas" {
			return true
		}
	}
	return false
}
