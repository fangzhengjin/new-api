package common

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ResponseModel records upstream declarations before response conversion. It is
// diagnostic only: it must never change routing, pricing, or downstream output.
// Only the three names are stored; whether they disagree is computed on demand
// so every consumer applies the current comparison rule to old rows as well.
type ResponseModel struct {
	RequestedModel string `json:"requested_model"`
	UpstreamModel  string `json:"upstream_model"`
	ReturnedModel  string `json:"returned_model"`
}

// matches reports whether an upstream declaration is compatible with the
// requested or upstream model: equal ignoring case, a dated or variant name
// that extends it, or the same name behind a provider path such as
// "deepseek/deepseek-v4.1-flash".
func (r *ResponseModel) matches(model string) bool {
	returned := strings.ToLower(model)
	for _, expected := range []string{r.RequestedModel, r.UpstreamModel} {
		expected = strings.ToLower(expected)
		if expected != "" && (strings.HasPrefix(returned, expected) || strings.HasSuffix(returned, expected)) {
			return true
		}
	}
	return false
}

// Mismatch reports whether the retained upstream declaration disagrees with
// both the requested and upstream models.
func (r *ResponseModel) Mismatch() bool {
	return r != nil && r.ReturnedModel != "" && !r.matches(r.ReturnedModel)
}

// ObserveResponseModel retains the first differing model for inspection, with
// mismatches taking priority over provider-path, prefix, or case-only
// differences. A later matching or empty event cannot erase it. Only observe
// upstream declarations, never models synthesized by a response converter.
func (info *RelayInfo) ObserveResponseModel(model string) {
	if info == nil || strings.TrimSpace(model) == "" {
		return
	}
	if info.ResponseModel == nil {
		info.ResponseModel = &ResponseModel{
			RequestedModel: info.OriginModelName,
			UpstreamModel:  info.GetUpstreamModelName(),
		}
	}
	observation := info.ResponseModel
	if observation.Mismatch() {
		return
	}
	if observation.matches(model) && observation.ReturnedModel != "" &&
		observation.ReturnedModel != observation.RequestedModel && observation.ReturnedModel != observation.UpstreamModel {
		return
	}
	observation.ReturnedModel = model
}

// --- 用户可见的响应模型改写（本地能力，与上游观测并存） ---

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
