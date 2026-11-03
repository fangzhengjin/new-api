package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateChatMenuCollapseThreshold(t *testing.T) {
	for _, value := range []string{"0", "3", "20"} {
		_, err := ValidateChatMenuCollapseThreshold(value)
		require.NoError(t, err)
	}
	for _, value := range []string{"-1", "21", "1.5", ""} {
		_, err := ValidateChatMenuCollapseThreshold(value)
		assert.Error(t, err)
	}
}

func TestParseChatsJSONAcceptsStandardEntriesAndDefaultsOpenMode(t *testing.T) {
	chats, err := ParseChatsJSON(`[
		{"name":"Embedded","url":"https://embedded.example","enabled":true,"icon":"Palette","sandbox":["allow-scripts","allow-same-origin"]},
		{"name":"New tab","url":"https://new-tab.example","enabled":false,"open_mode":"new_tab"},
		{"name":"Native","url":"cherrystudio://import","enabled":true}
	]`)
	require.NoError(t, err)
	require.Len(t, chats, 3)
	sandbox := []string{"allow-scripts", "allow-same-origin"}
	assert.Equal(t, ChatPreset{Name: "Embedded", URL: "https://embedded.example", Enabled: true, Icon: "Palette", Sandbox: &sandbox}, chats[0])
	assert.Equal(t, ChatPreset{Name: "New tab", URL: "https://new-tab.example", Enabled: false, OpenMode: ChatOpenModeNewTab}, chats[1])
	assert.Equal(t, ChatPreset{Name: "Native", URL: "cherrystudio://import", Enabled: true}, chats[2])
}

func TestParseChatsJSONRejectsInvalidEntryState(t *testing.T) {
	invalid := []string{
		`null`,
		`[{"Legacy":"https://legacy.example"}]`,
		`[{"name":"Missing URL","enabled":false}]`,
		`[{"name":"Invalid enabled","url":"https://example.com","enabled":"false"}]`,
		`[{"name":"Invalid icon type","url":"https://example.com","enabled":true,"icon":1}]`,
		`[{"name":"Invalid icon characters","url":"https://example.com","enabled":true,"icon":"<Palette>"}]`,
		`[{"name":"Icon too long","url":"https://example.com","enabled":true,"icon":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]`,
		`[{"name":"Unknown","url":"https://example.com","enabled":true,"other":true}]`,
		`[{"name":"Duplicate","url":"https://one.example","enabled":true},{"name":"Duplicate","url":"https://two.example","enabled":true}]`,
		`[{"name":"Bad mode","url":"https://example.com","enabled":true,"open_mode":"popup"}]`,
		`[{"name":"Native mode","url":"cherrystudio://import","enabled":true,"open_mode":"new_tab"}]`,
		`[{"name":"Unknown sandbox","url":"https://example.com","enabled":true,"sandbox":["allow-navigation"]}]`,
		`[{"name":"Duplicate sandbox","url":"https://example.com","enabled":true,"sandbox":["allow-scripts","allow-scripts"]}]`,
		`[{"name":"Invalid sandbox type","url":"https://example.com","enabled":true,"sandbox":"allow-scripts"}]`,
		`[{"name":"New tab sandbox","url":"https://example.com","enabled":true,"open_mode":"new_tab","sandbox":["allow-scripts"]}]`,
		`[{"name":"Native sandbox","url":"cherrystudio://import","enabled":true,"sandbox":["allow-scripts"]}]`,
		`[{"name":"Two codes","url":"https://example.com/#{authCode}-{authCode}","enabled":true}]`,
		`[{"name":"Mixed secret","url":"https://example.com/#{authCode}&key={key}","enabled":true}]`,
		`[{"name":"Insecure auth","url":"http://example.com/#{authCode}","enabled":true}]`,
	}
	for _, value := range invalid {
		_, err := ParseChatsJSON(value)
		assert.Error(t, err, value)
	}
}

func TestParseChatsJSONAcceptsAuthCodeAnywhereInHTTPSURL(t *testing.T) {
	for _, chatURL := range []string{
		"https://example.com/{authCode}",
		"https://example.com/?code={authCode}",
		"https://example.com/#code={authCode}",
	} {
		chats, err := ParseChatsJSON(`[{"name":"Authorized","url":"` + chatURL + `","enabled":true}]`)

		require.NoError(t, err)
		assert.Equal(t, chatURL, chats[0].URL)
	}
}

func TestUpdateChatsKeepsCurrentValueWhenValidationFails(t *testing.T) {
	previous := GetChats()
	t.Cleanup(func() { setChats(previous) })
	setChats([]ChatPreset{{Name: "Existing", URL: "https://existing.example", Enabled: true}})

	err := UpdateChatsByJsonString(`[{"name":"Broken","enabled":false}]`)

	require.Error(t, err)
	assert.Equal(t, []ChatPreset{{Name: "Existing", URL: "https://existing.example", Enabled: true}}, GetChats())
}

func TestDefaultChatsSnapshotIsIsolated(t *testing.T) {
	defaults := GetDefaultChats()
	require.NotEmpty(t, defaults)
	defaults[0].Name = "Changed"

	assert.Equal(t, "Cherry Studio", GetDefaultChats()[0].Name)
}

func TestChatsJSONPreservesExplicitEmptySandbox(t *testing.T) {
	previous := GetChats()
	t.Cleanup(func() { setChats(previous) })
	require.NoError(t, UpdateChatsByJsonString(`[{"name":"Static","url":"https://static.example","enabled":true,"sandbox":[]}]`))

	assert.JSONEq(t, `[{"name":"Static","url":"https://static.example","enabled":true,"sandbox":[]}]`, Chats2JsonString())
}

func TestRenderChatPresetURLReplacesBackendVariablesWithEscapedValues(t *testing.T) {
	preset := ChatPreset{
		Name:    "Canvas",
		URL:     "https://canvas.example/image?text={textModels}&image={imageModels}&video={videoModels}#code={authCode}",
		Enabled: true,
	}

	launchURL, err := preset.RenderURL(ChatPresetVariables{
		AuthCode:    "code+/=",
		TextModels:  []string{"gpt-4o", "a model"},
		ImageModels: []string{"image/1"},
	})

	require.NoError(t, err)
	assert.Equal(t, "https://canvas.example/image?text=gpt-4o%2Ca+model&image=image%2F1&video=#code=code%2B%2F%3D", launchURL)
}
