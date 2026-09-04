package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRewriteResponseModel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		paths []string
	}{
		{name: "openai", input: `{"model":"upstream"}`, paths: []string{"model"}},
		{name: "responses", input: `{"response":{"model":"upstream"}}`, paths: []string{"response.model"}},
		{name: "claude", input: `{"message":{"model":"upstream"}}`, paths: []string{"message.model"}},
		{name: "gemini", input: `{"modelVersion":"upstream"}`, paths: []string{"modelVersion"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RewriteResponseModel([]byte(tt.input), "requested")
			require.NoError(t, err)
			for _, path := range tt.paths {
				assert.Equal(t, "requested", gjson.GetBytes(got, path).String())
			}
		})
	}

	invalid := []byte("[DONE]")
	got, err := RewriteResponseModel(invalid, "requested")
	require.NoError(t, err)
	assert.Equal(t, invalid, got)

	withoutModel := []byte(`{"id":"response-id"}`)
	got, err = RewriteResponseModel(withoutModel, "requested")
	require.NoError(t, err)
	assert.Equal(t, withoutModel, got)
}
