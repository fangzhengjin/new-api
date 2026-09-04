package common

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const userResponseModelOverrideKey = "user_response_model_override"

var responseModelPaths = [...]string{
	"model",
	"modelVersion",
	"response.model",
	"message.model",
}

// SetUserResponseModelOverride stores the model name chosen after mapping.
func SetUserResponseModelOverride(c *gin.Context, model string) {
	if c != nil {
		c.Set(userResponseModelOverrideKey, model)
	}
}

// GetUserResponseModelOverride returns the model name captured after mapping.
func GetUserResponseModelOverride(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(userResponseModelOverrideKey)
}

// RewriteResponseModel replaces protocol-level model fields that already
// exist in a JSON response without adding fields to formats that omit them.
func RewriteResponseModel(data []byte, model string) ([]byte, error) {
	if model == "" || !gjson.ValidBytes(data) {
		return data, nil
	}

	rewritten := data
	for _, path := range responseModelPaths {
		value := gjson.GetBytes(rewritten, path)
		if !value.Exists() || value.Type != gjson.String {
			continue
		}
		var err error
		rewritten, err = sjson.SetBytes(rewritten, path, model)
		if err != nil {
			return nil, fmt.Errorf("rewrite response model at %s: %w", path, err)
		}
	}
	return rewritten, nil
}
