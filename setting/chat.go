package setting

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const (
	DefaultChatMenuCollapseThreshold          = 3
	MaxChatMenuCollapseThreshold              = 20
	ChatsDefaultOptionKey                     = "ChatsDefault"
	ChatMenuCollapseThresholdDefaultOptionKey = "ChatMenuCollapseThresholdDefault"
	ChatOpenModeEmbedded                      = "embedded"
	ChatOpenModeNewTab                        = "new_tab"
	maxChatPresetIconLength                   = 64
)

var ChatMenuCollapseThreshold = DefaultChatMenuCollapseThreshold

func UpdateChatMenuCollapseThreshold(value string) error {
	threshold, err := ValidateChatMenuCollapseThreshold(value)
	if err != nil {
		return err
	}
	ChatMenuCollapseThreshold = threshold
	return nil
}

func ValidateChatMenuCollapseThreshold(value string) (int, error) {
	threshold, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || threshold < 0 || threshold > MaxChatMenuCollapseThreshold {
		return 0, fmt.Errorf("聊天菜单收纳数量必须是 0～%d 的整数", MaxChatMenuCollapseThreshold)
	}
	return threshold, nil
}

// ChatPreset is the persisted chat-menu entry shared by settings and launch flows.
type ChatPreset struct {
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	Enabled  bool      `json:"enabled"`
	Icon     string    `json:"icon,omitempty"`
	OpenMode string    `json:"open_mode,omitempty"`
	Sandbox  *[]string `json:"sandbox,omitempty"`
}

// ChatPresetVariables contains values rendered by the backend after a Token is selected.
type ChatPresetVariables struct {
	Address     string
	AuthCode    string
	TextModels  []string
	ImageModels []string
	VideoModels []string
}

var (
	chatsMu      sync.RWMutex
	defaultChats = []ChatPreset{
		{Name: "Cherry Studio", URL: "cherrystudio://providers/api-keys?v=1&data={cherryConfig}", Enabled: true},
		{Name: "AionUI", URL: "aionui://provider/add?v=1&data={aionuiConfig}", Enabled: true},
		{Name: "流畅阅读", URL: "fluentread", Enabled: true},
		{Name: "DeepChat", URL: "deepchat://provider/install?v=1&data={deepchatConfig}", Enabled: true},
		{Name: "Lobe Chat 官方示例", URL: "https://chat-preview.lobehub.com/?settings={\"keyVaults\":{\"openai\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\"}}}", Enabled: true},
		{Name: "AI as Workspace", URL: "https://aiaw.app/set-provider?provider={\"type\":\"openai\",\"settings\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\",\"compatibility\":\"strict\"}}", Enabled: true},
		{Name: "AMA 问天", URL: "ama://set-api-key?server={address}&key={key}", Enabled: true},
		{Name: "OpenCat", URL: "opencat://team/join?domain={address}&token={key}", Enabled: true},
	}
	chats = cloneChatPresets(defaultChats)
)

var backendChatVariables = []string{"{authCode}", "{textModels}", "{imageModels}", "{videoModels}"}
var directSecretChatVariables = []string{"{key}", "{cherryConfig}", "{aionuiConfig}", "{deepchatConfig}"}
var chatPresetIconPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _-]*$`)
var allowedChatSandboxPermissions = map[string]struct{}{
	"allow-downloads":                {},
	"allow-forms":                    {},
	"allow-modals":                   {},
	"allow-popups":                   {},
	"allow-popups-to-escape-sandbox": {},
	"allow-presentation":             {},
	"allow-same-origin":              {},
	"allow-scripts":                  {},
}

func cloneChatPreset(preset ChatPreset) ChatPreset {
	if preset.Sandbox != nil {
		sandbox := make([]string, len(*preset.Sandbox))
		copy(sandbox, *preset.Sandbox)
		preset.Sandbox = &sandbox
	}
	return preset
}

func cloneChatPresets(presets []ChatPreset) []ChatPreset {
	result := make([]ChatPreset, len(presets))
	for index, preset := range presets {
		result[index] = cloneChatPreset(preset)
	}
	return result
}

// EffectiveOpenMode returns the default embedded mode for HTTP(S) entries.
func (preset ChatPreset) EffectiveOpenMode() string {
	if preset.OpenMode == "" {
		return ChatOpenModeEmbedded
	}
	return preset.OpenMode
}

// RequiresBackendLaunch reports whether selecting a Token and rendering server variables is required.
func (preset ChatPreset) RequiresBackendLaunch() bool {
	for _, variable := range backendChatVariables {
		if strings.Contains(preset.URL, variable) {
			return true
		}
	}
	return false
}

// RequiresAuthCode reports whether the launch must create a one-time bearer code.
func (preset ChatPreset) RequiresAuthCode() bool {
	return strings.Contains(preset.URL, "{authCode}")
}

// RequiresModelList reports whether the launch must render Token-filtered model lists.
func (preset ChatPreset) RequiresModelList() bool {
	return strings.Contains(preset.URL, "{textModels}") ||
		strings.Contains(preset.URL, "{imageModels}") ||
		strings.Contains(preset.URL, "{videoModels}")
}

// RenderURL substitutes backend-owned variables with URL-escaped values.
func (preset ChatPreset) RenderURL(variables ChatPresetVariables) (string, error) {
	if err := validateChatPreset(preset); err != nil {
		return "", err
	}
	values := map[string]string{
		"{address}":     variables.Address,
		"{authCode}":    variables.AuthCode,
		"{textModels}":  strings.Join(variables.TextModels, ","),
		"{imageModels}": strings.Join(variables.ImageModels, ","),
		"{videoModels}": strings.Join(variables.VideoModels, ","),
	}
	rendered := preset.URL
	for placeholder, value := range values {
		rendered = strings.ReplaceAll(rendered, placeholder, url.QueryEscape(value))
	}
	return rendered, nil
}

// GetChats returns a copy of the current authoritative chat preset list.
func GetChats() []ChatPreset {
	chatsMu.RLock()
	defer chatsMu.RUnlock()
	return cloneChatPresets(chats)
}

// GetDefaultChats returns an isolated copy of the built-in chat presets.
func GetDefaultChats() []ChatPreset {
	return cloneChatPresets(defaultChats)
}

// GetChatPreset returns a preset by its unique name.
func GetChatPreset(name string) (ChatPreset, bool) {
	name = strings.TrimSpace(name)
	chatsMu.RLock()
	defer chatsMu.RUnlock()
	for _, preset := range chats {
		if preset.Name == name {
			return cloneChatPreset(preset), true
		}
	}
	return ChatPreset{}, false
}

func setChats(value []ChatPreset) {
	chatsMu.Lock()
	chats = cloneChatPresets(value)
	chatsMu.Unlock()
}

func UpdateChatsByJsonString(jsonString string) error {
	parsed, err := ParseChatsJSON(jsonString)
	if err != nil {
		return err
	}
	setChats(parsed)
	return nil
}

// ParseChatsJSON validates the current standard object-array format.
func ParseChatsJSON(jsonString string) ([]ChatPreset, error) {
	if !strings.HasPrefix(strings.TrimSpace(jsonString), "[") {
		return nil, fmt.Errorf("聊天预设必须是 JSON 数组")
	}
	var raw []map[string]any
	if err := common.UnmarshalJsonStr(jsonString, &raw); err != nil {
		return nil, fmt.Errorf("聊天预设 JSON 无效: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("聊天预设必须是 JSON 数组")
	}
	presets := make([]ChatPreset, 0, len(raw))
	names := make(map[string]struct{}, len(raw))
	for index, item := range raw {
		for key := range item {
			if key != "name" && key != "url" && key != "enabled" && key != "icon" && key != "open_mode" && key != "sandbox" {
				return nil, fmt.Errorf("聊天预设第 %d 项包含不支持的字段 %q", index+1, key)
			}
		}
		name, nameOK := item["name"].(string)
		chatURL, urlOK := item["url"].(string)
		enabled, enabledOK := item["enabled"].(bool)
		preset := ChatPreset{Name: strings.TrimSpace(name), URL: strings.TrimSpace(chatURL), Enabled: enabled}
		if icon, exists := item["icon"]; exists {
			var iconOK bool
			preset.Icon, iconOK = icon.(string)
			if !iconOK {
				return nil, fmt.Errorf("聊天预设 %q 的 icon 必须是字符串", preset.Name)
			}
			preset.Icon = strings.TrimSpace(preset.Icon)
		}
		if mode, exists := item["open_mode"]; exists {
			var modeOK bool
			preset.OpenMode, modeOK = mode.(string)
			if !modeOK {
				return nil, fmt.Errorf("聊天预设 %q 的 open_mode 必须是字符串", preset.Name)
			}
		}
		if rawSandbox, exists := item["sandbox"]; exists {
			values, sandboxOK := rawSandbox.([]any)
			if !sandboxOK {
				return nil, fmt.Errorf("聊天预设 %q 的 sandbox 必须是字符串数组", preset.Name)
			}
			sandbox := make([]string, len(values))
			for valueIndex, value := range values {
				permission, permissionOK := value.(string)
				if !permissionOK {
					return nil, fmt.Errorf("聊天预设 %q 的 sandbox 必须是字符串数组", preset.Name)
				}
				sandbox[valueIndex] = permission
			}
			preset.Sandbox = &sandbox
		}
		if !nameOK || !urlOK || !enabledOK {
			return nil, fmt.Errorf("聊天预设第 %d 项必须包含 name、url 和 enabled", index+1)
		}
		if _, exists := names[preset.Name]; exists {
			return nil, fmt.Errorf("聊天预设名称 %q 重复", preset.Name)
		}
		if err := validateChatPreset(preset); err != nil {
			return nil, err
		}
		names[preset.Name] = struct{}{}
		presets = append(presets, preset)
	}
	return presets, nil
}

func validateChatPreset(preset ChatPreset) error {
	if preset.Name == "" {
		return fmt.Errorf("聊天预设名称不能为空")
	}
	if preset.URL == "" {
		return fmt.Errorf("聊天预设 %q 的 URL 不能为空", preset.Name)
	}
	if len(preset.Icon) > maxChatPresetIconLength || (preset.Icon != "" && !chatPresetIconPattern.MatchString(preset.Icon)) {
		return fmt.Errorf("聊天预设 %q 的 icon 必须是 1～%d 位字母、数字、空格、下划线或连字符", preset.Name, maxChatPresetIconLength)
	}
	isHTTP := strings.HasPrefix(strings.ToLower(preset.URL), "http://") || strings.HasPrefix(strings.ToLower(preset.URL), "https://")
	if preset.OpenMode != "" && preset.OpenMode != ChatOpenModeEmbedded && preset.OpenMode != ChatOpenModeNewTab {
		return fmt.Errorf("聊天预设 %q 的 open_mode 只能是 embedded 或 new_tab", preset.Name)
	}
	if preset.OpenMode != "" && !isHTTP {
		return fmt.Errorf("聊天预设 %q 只有 HTTP(S) URL 可以设置 open_mode", preset.Name)
	}
	if preset.Sandbox != nil {
		if !isHTTP || preset.EffectiveOpenMode() != ChatOpenModeEmbedded {
			return fmt.Errorf("聊天预设 %q 只有嵌入模式的 HTTP(S) URL 可以设置 sandbox", preset.Name)
		}
		seen := make(map[string]struct{}, len(*preset.Sandbox))
		for _, permission := range *preset.Sandbox {
			if _, ok := allowedChatSandboxPermissions[permission]; !ok {
				return fmt.Errorf("聊天预设 %q 包含不支持的 sandbox 权限 %q", preset.Name, permission)
			}
			if _, exists := seen[permission]; exists {
				return fmt.Errorf("聊天预设 %q 的 sandbox 权限 %q 重复", preset.Name, permission)
			}
			seen[permission] = struct{}{}
		}
	}
	if !preset.RequiresBackendLaunch() {
		return nil
	}
	for _, variable := range directSecretChatVariables {
		if strings.Contains(preset.URL, variable) {
			return fmt.Errorf("聊天预设 %q 不能混用后端授权变量和 %s", preset.Name, variable)
		}
	}
	if strings.Count(preset.URL, "{authCode}") == 0 {
		return nil
	}
	if strings.Count(preset.URL, "{authCode}") != 1 {
		return fmt.Errorf("聊天预设 %q 必须且只能包含一个 {authCode}", preset.Name)
	}
	parsed, err := url.Parse(preset.URL)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("聊天预设 %q 的授权地址无效", preset.Name)
	}
	if parsed.Scheme != "https" {
		hostname := strings.ToLower(parsed.Hostname())
		ip := net.ParseIP(hostname)
		if parsed.Scheme != "http" || (hostname != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return fmt.Errorf("聊天预设 %q 的授权地址必须使用 HTTPS，本地调试除外", preset.Name)
		}
	}
	return nil
}

func Chats2JsonString() string {
	jsonBytes, err := common.Marshal(GetChats())
	if err != nil {
		common.SysLog("error marshalling chats: " + err.Error())
		return "[]"
	}
	return string(jsonBytes)
}
