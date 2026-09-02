package setting

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const (
	RequestLimitErrorTemplateDefaultsOptionKey = "RequestLimitErrorTemplateDefaults"

	ModelRequestRateLimitAccountTotalErrorTemplateOptionKey   = "ModelRequestRateLimitAccountTotalErrorTemplate"
	ModelRequestRateLimitAccountSuccessErrorTemplateOptionKey = "ModelRequestRateLimitAccountSuccessErrorTemplate"
	ModelRequestRateLimitIPTotalErrorTemplateOptionKey        = "ModelRequestRateLimitIPTotalErrorTemplate"
	ModelRequestRateLimitIPSuccessErrorTemplateOptionKey      = "ModelRequestRateLimitIPSuccessErrorTemplate"
	ModelRequestConcurrencyAccountErrorTemplateOptionKey      = "ModelRequestConcurrencyAccountErrorTemplate"
	ModelRequestConcurrencyIPErrorTemplateOptionKey           = "ModelRequestConcurrencyIPErrorTemplate"
	AccessSourceSwitchCooldownErrorTemplateOptionKey          = "AccessSourceSwitchCooldownErrorTemplate"
	AccessSourceAccountIPLimitErrorTemplateOptionKey          = "AccessSourceAccountIPLimitErrorTemplate"
	AccessSourceIPAccountLimitErrorTemplateOptionKey          = "AccessSourceIPAccountLimitErrorTemplate"

	MaxRequestLimitErrorTemplateLength = 2000
)

// RequestLimitErrorTemplateValues contains the only values exposed to administrator-defined rejection messages.
type RequestLimitErrorTemplateValues struct {
	Limit      string
	Period     string
	RetryAfter string
}

type requestLimitErrorTemplateSpec struct {
	defaultTemplate string
	variables       []string
}

var requestLimitErrorTemplateSpecs = map[string]requestLimitErrorTemplateSpec{
	ModelRequestRateLimitAccountTotalErrorTemplateOptionKey: {
		defaultTemplate: "Account request limit reached, maximum {{.Limit}} requests in {{.Period}} including failed requests, retry in {{.RetryAfter}}\n账号请求数已达到上限，{{.Period}} 内最多请求 {{.Limit}} 次，包括失败请求，请在 {{.RetryAfter}} 后重试",
		variables:       []string{"Limit", "Period", "RetryAfter"},
	},
	ModelRequestRateLimitAccountSuccessErrorTemplateOptionKey: {
		defaultTemplate: "Account successful request limit reached, maximum {{.Limit}} successful requests in {{.Period}}, retry in {{.RetryAfter}}\n账号成功请求数已达到上限，{{.Period}} 内最多成功请求 {{.Limit}} 次，请在 {{.RetryAfter}} 后重试",
		variables:       []string{"Limit", "Period", "RetryAfter"},
	},
	ModelRequestRateLimitIPTotalErrorTemplateOptionKey: {
		defaultTemplate: "IP request limit reached, maximum {{.Limit}} requests in {{.Period}} including failed requests, retry in {{.RetryAfter}}\nIP 请求数已达到上限，{{.Period}} 内最多请求 {{.Limit}} 次，包括失败请求，请在 {{.RetryAfter}} 后重试",
		variables:       []string{"Limit", "Period", "RetryAfter"},
	},
	ModelRequestRateLimitIPSuccessErrorTemplateOptionKey: {
		defaultTemplate: "IP successful request limit reached, maximum {{.Limit}} successful requests in {{.Period}}, retry in {{.RetryAfter}}\nIP 成功请求数已达到上限，{{.Period}} 内最多成功请求 {{.Limit}} 次，请在 {{.RetryAfter}} 后重试",
		variables:       []string{"Limit", "Period", "RetryAfter"},
	},
	ModelRequestConcurrencyAccountErrorTemplateOptionKey: {
		defaultTemplate: "Account concurrency limit reached, maximum {{.Limit}} requests can run at the same time, retry after an active request finishes\n账号并发请求数已达到上限，最多同时处理 {{.Limit}} 个请求，请等待正在处理的请求完成后重试",
		variables:       []string{"Limit"},
	},
	ModelRequestConcurrencyIPErrorTemplateOptionKey: {
		defaultTemplate: "IP concurrency limit reached, maximum {{.Limit}} requests can run at the same time, retry after an active request finishes\nIP 并发请求数已达到上限，最多同时处理 {{.Limit}} 个请求，请等待正在处理的请求完成后重试",
		variables:       []string{"Limit"},
	},
	AccessSourceSwitchCooldownErrorTemplateOptionKey: {
		defaultTemplate: "IP switch blocked because this account changed IPs too recently, retry in {{.RetryAfter}}\n账号切换 IP 过于频繁，请在 {{.RetryAfter}} 后重试",
		variables:       []string{"RetryAfter"},
	},
	AccessSourceAccountIPLimitErrorTemplateOptionKey: {
		defaultTemplate: "Account IP limit reached, this account can use a maximum of {{.Limit}} active IPs within {{.Period}}\n账号关联 IP 数已达到上限，{{.Period}} 内最多可使用 {{.Limit}} 个活跃 IP",
		variables:       []string{"Limit", "Period"},
	},
	AccessSourceIPAccountLimitErrorTemplateOptionKey: {
		defaultTemplate: "IP account limit reached, this IP can be used by a maximum of {{.Limit}} active accounts within {{.Period}}\nIP 关联账号数已达到上限，{{.Period}} 内最多可供 {{.Limit}} 个活跃账号使用",
		variables:       []string{"Limit", "Period"},
	},
}

// IsRequestLimitErrorTemplateOptionKey reports whether key configures a request-limit rejection message.
func IsRequestLimitErrorTemplateOptionKey(key string) bool {
	_, ok := requestLimitErrorTemplateSpecs[key]
	return ok
}

// GetDefaultRequestLimitErrorTemplates returns the built-in bilingual templates shown by the settings UI.
func GetDefaultRequestLimitErrorTemplates() map[string]string {
	defaults := make(map[string]string, len(requestLimitErrorTemplateSpecs))
	for key, spec := range requestLimitErrorTemplateSpecs {
		defaults[key] = spec.defaultTemplate
	}
	return defaults
}

// ValidateRequestLimitErrorTemplate checks length, syntax, and scenario-specific variables.
func ValidateRequestLimitErrorTemplate(key string, value string) error {
	spec, ok := requestLimitErrorTemplateSpecs[key]
	if !ok {
		return fmt.Errorf("未知的请求限制错误模板 %s", key)
	}
	if utf8.RuneCountInString(value) > MaxRequestLimitErrorTemplateLength {
		return fmt.Errorf("请求限制错误模板不能超过 %d 个字符", MaxRequestLimitErrorTemplateLength)
	}
	if strings.TrimSpace(value) == "" {
		return nil
	}
	_, err := parseRequestLimitErrorTemplate(key, value, spec)
	return err
}

// RenderRequestLimitErrorTemplate renders the configured template or the built-in default when unset.
func RenderRequestLimitErrorTemplate(key string, values RequestLimitErrorTemplateValues) (string, error) {
	spec, ok := requestLimitErrorTemplateSpecs[key]
	if !ok {
		return "", fmt.Errorf("未知的请求限制错误模板 %s", key)
	}
	common.OptionMapRWMutex.RLock()
	configured := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	templateText := configured
	if strings.TrimSpace(templateText) == "" {
		templateText = spec.defaultTemplate
	}
	tmpl, err := parseRequestLimitErrorTemplate(key, templateText, spec)
	if err != nil {
		if templateText == spec.defaultTemplate {
			return "", err
		}
		tmpl, fallbackErr := parseRequestLimitErrorTemplate(key, spec.defaultTemplate, spec)
		if fallbackErr != nil {
			return "", fallbackErr
		}
		message, fallbackErr := executeRequestLimitErrorTemplate(tmpl, values)
		return message, errors.Join(err, fallbackErr)
	}
	return executeRequestLimitErrorTemplate(tmpl, values)
}

// FormatRequestLimitDuration formats a language-neutral duration for rejection messages.
func FormatRequestLimitDuration(seconds int64) string {
	if seconds < 60 {
		if seconds < 0 {
			seconds = 0
		}
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 60*60 {
		minutes := seconds / 60
		remainingSeconds := seconds % 60
		if remainingSeconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, remainingSeconds)
	}
	hours := seconds / (60 * 60)
	minutes := seconds % (60 * 60) / 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func parseRequestLimitErrorTemplate(key string, value string, spec requestLimitErrorTemplateSpec) (*template.Template, error) {
	tmpl, err := template.New(key).Option("missingkey=error").Parse(value)
	if err != nil {
		return nil, fmt.Errorf("请求限制错误模板语法无效: %w", err)
	}
	if len(tmpl.Templates()) != 1 {
		return nil, fmt.Errorf("请求限制错误模板只支持文本和变量")
	}
	if err := validateRequestLimitTemplateNode(tmpl.Tree.Root, spec.variables); err != nil {
		return nil, err
	}
	return tmpl, nil
}

func validateRequestLimitTemplateNode(node parse.Node, allowed []string) error {
	switch typed := node.(type) {
	case *parse.ListNode:
		for _, child := range typed.Nodes {
			if err := validateRequestLimitTemplateNode(child, allowed); err != nil {
				return err
			}
		}
		return nil
	case *parse.TextNode:
		return nil
	case *parse.ActionNode:
		if len(typed.Pipe.Decl) != 0 || len(typed.Pipe.Cmds) != 1 || len(typed.Pipe.Cmds[0].Args) != 1 {
			return fmt.Errorf("请求限制错误模板只能插入可用变量")
		}
		field, ok := typed.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
		if !ok || len(field.Ident) != 1 {
			return fmt.Errorf("请求限制错误模板只能使用 {{.变量名}} 格式")
		}
		for _, variable := range allowed {
			if field.Ident[0] == variable {
				return nil
			}
		}
		return fmt.Errorf("当前场景不支持变量 {{.%s}}", field.Ident[0])
	default:
		return fmt.Errorf("请求限制错误模板只支持文本和变量")
	}
}

func executeRequestLimitErrorTemplate(tmpl *template.Template, values RequestLimitErrorTemplateValues) (string, error) {
	var output bytes.Buffer
	if err := tmpl.Execute(&output, values); err != nil {
		return "", fmt.Errorf("渲染请求限制错误模板: %w", err)
	}
	return output.String(), nil
}
