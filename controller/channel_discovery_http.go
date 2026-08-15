package controller

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
)

const (
	channelDiscoveryMaxInputBytes    = 256 * 1024
	channelDiscoveryMaxResponseBytes = 5 * 1024 * 1024
	channelDiscoveryMaxBlocks        = 10
	channelDiscoveryMaxKeys          = 50
	channelDiscoveryRequestTimeout   = 12 * time.Second
	channelDiscoveryOperationTimeout = 2 * time.Minute
)

var (
	channelDiscoveryURLPattern     = regexp.MustCompile(`(?i)https?://[^\s<>]+`)
	channelDiscoveryBareURLPattern = regexp.MustCompile(`(?i)(^|[^/@A-Za-z0-9.-])((?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}(?::\d{1,5})?(?:/[^\s,，;；<>]*)?)`)
	channelDiscoveryHostPattern    = regexp.MustCompile(`(?i)^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}(?::\d{1,5})?(?:/\S*)?$`)
	channelDiscoveryKeyPattern     = regexp.MustCompile(`^[A-Za-z0-9._~+/-]{6,}=*$`)
	channelDiscoveryStandaloneKey  = regexp.MustCompile(`^[A-Za-z0-9._~+/-]{8,}=*$`)
	channelDiscoveryPrefixedKey    = regexp.MustCompile(`(?i)\b(?:sk|key|token|sess|pk|ark)[-_][A-Za-z0-9._~+/-]{6,}=*`)
	channelDiscoveryLabeledKeys    = []*regexp.Regexp{
		regexp.MustCompile(`(?i)authorization["'\x60]?\s*[:=：]\s*["'\x60]?bearer\s+([A-Za-z0-9._~+/-]{6,}=*)`),
		regexp.MustCompile(`(?i)(?:x-api-key|api[_\s-]?key|apikey|token|密钥|key)["'\x60]?\s*[:=：]\s*["'\x60]?([A-Za-z0-9._~+/-]{6,}=*)`),
		regexp.MustCompile(`(?i)(?:x-api-key|api[_\s-]?key|apikey|token|密钥|key)\s+(?:is\s+)?["'\x60]?([A-Za-z0-9._~+/-]{6,}=*)`),
	}
)

type channelDiscoveryBlock struct {
	BaseURL    string
	InputURL   string
	Origin     string
	Keys       []string
	ModelsPath string
}

type channelDiscoveryFetchResult struct {
	Models             []string
	ModelsPath         string
	ModelsAuthType     string
	UsableKeyIndexes   []int
	RejectedKeyIndexes []int
	Error              error
}

// parseChannelDiscoveryBlocks treats each detected URL as a block boundary.
// Keys on the URL line belong to that URL regardless of order; leading keys are
// held for the next URL, while later key-only lines stay with the preceding URL
// unless a blank line separates completed connections. This accepts common paste
// formats without sending one block's keys to another.
func parseChannelDiscoveryBlocks(text string) ([]channelDiscoveryBlock, error) {
	if len(text) > channelDiscoveryMaxInputBytes {
		return nil, fmt.Errorf("connection input exceeds %d KiB", channelDiscoveryMaxInputBytes/1024)
	}
	blocks := make([]channelDiscoveryBlock, 0)
	var current *channelDiscoveryBlock
	pendingKeys := make([]string, 0)
	totalKeys := 0

	for _, rawLine := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(strings.TrimLeft(rawLine, "-*•> \t"))
		if line == "" {
			// A blank line closes only a complete block. This lets URL, blank,
			// Key remain one connection while separating later Key-before-URL blocks.
			if current != nil && len(current.Keys) > 0 {
				blocks = append(blocks, *current)
				current = nil
			}
			continue
		}
		keyFragments := []string{line}
		explicitURLs := channelDiscoveryURLPattern.FindAllStringIndex(line, -1)
		bareURLs := channelDiscoveryBareURLPattern.FindAllStringSubmatchIndex(line, -1)
		if len(explicitURLs)+len(bareURLs) > 1 {
			return nil, errors.New("put each upstream URL on its own line")
		}
		var urlLocation []int
		rawURL := ""
		if len(explicitURLs) == 1 {
			urlLocation = explicitURLs[0]
			rawURL = line[urlLocation[0]:urlLocation[1]]
		} else if len(bareURLs) == 1 {
			match := bareURLs[0]
			urlLocation = []int{match[4], match[5]}
			rawURL = "https://" + line[match[4]:match[5]]
		}
		if urlLocation != nil {
			block, err := newChannelDiscoveryBlock(rawURL)
			if err != nil {
				return nil, err
			}
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &block
			for _, key := range pendingKeys {
				if !slices.Contains(current.Keys, key) {
					current.Keys = append(current.Keys, key)
					totalKeys++
				}
			}
			pendingKeys = nil
			keyFragments = []string{line[:urlLocation[0]], line[urlLocation[1]:]}
		}
		for _, fragment := range keyFragments {
			for _, key := range extractChannelDiscoveryKeys(fragment, urlLocation == nil) {
				if current == nil {
					if !slices.Contains(pendingKeys, key) {
						pendingKeys = append(pendingKeys, key)
					}
					continue
				}
				if !slices.Contains(current.Keys, key) {
					current.Keys = append(current.Keys, key)
					totalKeys++
				}
			}
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}
	if len(blocks) == 0 {
		return nil, errors.New("no upstream URL found")
	}
	if len(pendingKeys) > 0 {
		return nil, errors.New("API key has no following upstream URL")
	}
	if len(blocks) > channelDiscoveryMaxBlocks {
		return nil, fmt.Errorf("at most %d connection blocks are allowed", channelDiscoveryMaxBlocks)
	}
	if totalKeys == 0 {
		return nil, errors.New("no API key found")
	}
	if totalKeys > channelDiscoveryMaxKeys {
		return nil, fmt.Errorf("at most %d keys are allowed", channelDiscoveryMaxKeys)
	}
	for index := range blocks {
		if len(blocks[index].Keys) == 0 {
			return nil, fmt.Errorf("connection block %d has no API key", index+1)
		}
	}
	return blocks, nil
}

// newChannelDiscoveryBlock validates rawURL and separates the exact discovery
// input from the relay Base URL. Known endpoint suffixes are removed only from
// the Base URL; the returned origin remains available for same-origin checks.
func newChannelDiscoveryBlock(rawURL string) (channelDiscoveryBlock, error) {
	cleaned := strings.TrimRight(strings.TrimSpace(rawURL), "\\)]}>\"'`,;，。；")
	parsed, err := url.Parse(cleaned)
	if err != nil || parsed.Host == "" {
		return channelDiscoveryBlock{}, errors.New("invalid upstream URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return channelDiscoveryBlock{}, errors.New("upstream URL must use http or https")
	}
	if parsed.User != nil {
		return channelDiscoveryBlock{}, errors.New("upstream URL must not contain credentials")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	inputURL := strings.TrimRight(parsed.String(), "/")
	base := *parsed
	path := strings.TrimRight(base.Path, "/")
	// Known API endpoints are stripped only to recover the upstream base. The
	// successful model-list path is retained separately for Advanced Custom routes.
	endpointSuffixes := []string{
		"/v1/chat/completions",
		"/v1/images/generations",
		"/v1/images/edits",
		"/v1/responses/compact",
		"/v1/responses",
		"/v1/messages",
		"/v1/models",
		"/models",
	}
	for _, suffix := range endpointSuffixes {
		if strings.HasSuffix(strings.ToLower(path), suffix) {
			path = path[:len(path)-len(suffix)]
			break
		}
	}
	path = strings.TrimSuffix(path, "/v1")
	base.Path = strings.TrimRight(path, "/")
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	baseURL := strings.TrimRight(base.String(), "/")
	if baseURL == "" {
		baseURL = parsed.Scheme + "://" + parsed.Host
	}
	return channelDiscoveryBlock{
		BaseURL:  baseURL,
		InputURL: inputURL,
		Origin:   parsed.Scheme + "://" + parsed.Host,
	}, nil
}

func normalizeChannelDiscoveryKey(line string) string {
	value := strings.TrimSpace(line)
	parts := strings.Fields(value)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		value = parts[1]
	}
	value = strings.Trim(value, "\"'`()[]{}<>,;，；")
	if !channelDiscoveryKeyPattern.MatchString(value) || channelDiscoveryHostPattern.MatchString(value) {
		return ""
	}
	return value
}

// extractChannelDiscoveryKeys recognizes the labeled, prefixed, and standalone
// credential formats accepted by the original channel tool. Standalone values
// are allowed only when the fragment occupies its own line, avoiding prose words.
func extractChannelDiscoveryKeys(text string, allowStandalone bool) []string {
	keys := make([]string, 0)
	add := func(raw string) {
		key := normalizeChannelDiscoveryKey(raw)
		if key != "" && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	for _, pattern := range channelDiscoveryLabeledKeys {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			add(match[1])
		}
	}
	for _, match := range channelDiscoveryPrefixedKey.FindAllString(text, -1) {
		add(match)
	}
	if allowStandalone {
		key := normalizeChannelDiscoveryKey(text)
		if channelDiscoveryStandaloneKey.MatchString(key) {
			add(key)
		}
	}
	return keys
}

// discoverChannelBlock tests each key in block under ctx, reuses the first
// successful endpoint/auth pair, and returns the union of visible models plus
// usable and rejected key indexes.
func discoverChannelBlock(ctx context.Context, block channelDiscoveryBlock) channelDiscoveryFetchResult {
	result := channelDiscoveryFetchResult{}
	models := map[string]struct{}{}
	var endpoint string
	var authType string

	// The first usable key identifies the model endpoint and auth scheme. Reusing
	// them for sibling keys avoids repeating the full endpoint/auth search.
	for keyIndex, key := range block.Keys {
		var fetched []string
		var err error
		if endpoint == "" {
			fetched, endpoint, authType, err = discoverModelsWithKey(ctx, block, key)
		} else {
			fetched, err = fetchChannelDiscoveryModels(ctx, endpoint, block.Origin, key, authType)
		}
		if err != nil {
			result.RejectedKeyIndexes = append(result.RejectedKeyIndexes, keyIndex)
			result.Error = err
			continue
		}
		result.UsableKeyIndexes = append(result.UsableKeyIndexes, keyIndex)
		if result.ModelsPath == "" {
			parsed, _ := url.Parse(endpoint)
			result.ModelsPath = parsed.Path
			result.ModelsAuthType = authType
		}
		for _, modelName := range fetched {
			models[modelName] = struct{}{}
		}
	}
	result.Models = sortedSetValues(models)
	if len(result.UsableKeyIndexes) > 0 {
		result.Error = nil
	} else if result.Error == nil {
		result.Error = errors.New("no key could access the model discovery endpoint")
	}
	return result
}

// discoverModelsWithKey searches the bounded endpoint/auth combinations for key.
// It returns models together with the successful endpoint and auth scheme so
// sibling keys do not repeat the search.
func discoverModelsWithKey(ctx context.Context, block channelDiscoveryBlock, key string) ([]string, string, string, error) {
	var lastErr error
	for _, endpoint := range channelDiscoveryModelEndpoints(block) {
		for _, authType := range []string{"bearer", "anthropic"} {
			models, err := fetchChannelDiscoveryModels(ctx, endpoint, block.Origin, key, authType)
			if err == nil {
				return models, endpoint, authType, nil
			}
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no model discovery endpoint found")
	}
	return nil, "", "", lastErr
}

func channelDiscoveryModelEndpoints(block channelDiscoveryBlock) []string {
	// An explicitly pasted Models endpoint wins, then the bounded fallback list
	// covers common base-path and origin-root deployments without guessing routes.
	result := make([]string, 0, 6)
	input, _ := url.Parse(block.InputURL)
	if input != nil && strings.HasSuffix(strings.TrimRight(strings.ToLower(input.Path), "/"), "/models") {
		result = append(result, strings.TrimRight(input.String(), "/"))
	}
	base, _ := url.Parse(block.BaseURL)
	basePath := strings.TrimRight(base.Path, "/")
	appendPath := func(path string) {
		next := *base
		next.Path = path
		result = append(result, next.String())
	}
	appendPath(basePath + "/v1/models")
	appendPath(basePath + "/models")
	appendPath("/v1/models")
	appendPath("/models")
	return uniqueStrings(result)
}

// channelDiscoveryRouteTarget converts an origin path into the form consumed by
// Advanced Custom: relative to Base URL when possible, otherwise a same-origin URL.
func channelDiscoveryRouteTarget(block channelDiscoveryBlock, endpointPath string) string {
	endpointPath = strings.TrimSpace(endpointPath)
	base, _ := url.Parse(block.BaseURL)
	basePath := strings.TrimRight(base.Path, "/")
	if basePath == "" {
		return endpointPath
	}
	if endpointPath == basePath {
		return "/"
	}
	if strings.HasPrefix(endpointPath, basePath+"/") {
		return strings.TrimPrefix(endpointPath, basePath)
	}
	return block.Origin + endpointPath
}

// fetchChannelDiscoveryModels requests endpoint under ctx using the selected
// authType and key. origin constrains redirects; the returned model IDs are
// normalized and bounded, and request errors are sanitized before propagation.
func fetchChannelDiscoveryModels(ctx context.Context, endpoint string, origin string, key string, authType string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if authType == "anthropic" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := channelDiscoveryHTTPClient(origin).Do(req)
	if err != nil {
		return nil, sanitizeChannelDiscoveryError(err, key)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("model discovery returned HTTP %d", response.StatusCode)
	}
	body, err := readChannelDiscoveryBody(response.Body)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("model discovery response is not valid JSON")
	}
	items := payload
	if object, ok := payload.(map[string]any); ok {
		items = object["data"]
	}
	list, ok := items.([]any)
	if !ok {
		return nil, errors.New("model discovery response has no model array")
	}
	models := make([]string, 0, len(list))
	for _, item := range list {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := object["id"].(string); ok && strings.TrimSpace(id) != "" {
			models = append(models, strings.TrimSpace(id))
		}
	}
	if len(list) > 0 && len(models) == 0 {
		return nil, errors.New("model discovery entries have no id")
	}
	sort.Strings(models)
	return uniqueStrings(models), nil
}

// probeChannelDiscoveryEndpoint makes minimal inference requests for protocol
// under ctx. It returns the first successful same-origin path, never a full URL
// that could be persisted as a foreign route.
func probeChannelDiscoveryEndpoint(ctx context.Context, block channelDiscoveryBlock, key string, protocol string, modelName string) (string, error) {
	for _, endpoint := range channelDiscoveryProtocolEndpoints(block, block.ModelsPath, protocol) {
		for _, stream := range []bool{true, false} {
			req, err := newChannelDiscoveryProbeRequest(ctx, endpoint, key, protocol, modelName, stream)
			if err != nil {
				return "", err
			}
			response, err := channelDiscoveryHTTPClient(block.Origin).Do(req)
			if err != nil {
				continue
			}
			valid := false
			if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
				if stream {
					valid, _ = readChannelDiscoveryStreamProbe(protocol, response.Body)
				} else {
					body, readErr := readChannelDiscoveryBody(response.Body)
					valid = readErr == nil && isChannelDiscoveryProbeResponse(protocol, body)
				}
			}
			_ = response.Body.Close()
			if valid {
				parsed, _ := url.Parse(endpoint)
				return channelDiscoveryRouteTarget(block, parsed.Path), nil
			}
		}
	}
	return "", fmt.Errorf("%s protocol probe failed", protocol)
}

// newChannelDiscoveryProbeRequest builds the exact stream or JSON probe sent
// before a channel exists, including the shared OpenAI identity fallback.
func newChannelDiscoveryProbeRequest(ctx context.Context, endpoint string, key string, protocol string, modelName string, stream bool) (*http.Request, error) {
	encoded, err := common.Marshal(channelDiscoveryProbeBody(protocol, modelName, stream))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("Content-Type", "application/json")
	if protocol == "messages" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
		return req, nil
	}
	req.Header.Set("Authorization", "Bearer "+key)

	format := types.RelayFormatOpenAI
	var request dto.Request = &dto.GeneralOpenAIRequest{Model: modelName}
	if protocol == "responses" {
		format = types.RelayFormatOpenAIResponses
		request = &dto.OpenAIResponsesRequest{Model: modelName}
	}
	relaychannel.ApplyCodexRequestHeaderFallback(nil, req, &relaycommon.RelayInfo{
		RelayFormat: format,
		Request:     request,
	})
	return req, nil
}

func channelDiscoveryProtocolEndpoints(block channelDiscoveryBlock, modelsPath string, protocol string) []string {
	suffixes := map[string][]string{
		"responses": {"/v1/responses", "/responses"},
		"messages":  {"/v1/messages", "/messages"},
		"chat":      {"/v1/chat/completions", "/chat/completions"},
	}[protocol]
	base, _ := url.Parse(block.BaseURL)
	basePath := strings.TrimRight(base.Path, "/")
	result := make([]string, 0, len(suffixes)*3)
	modelsPath = strings.TrimRight(modelsPath, "/")
	modelsPrefix := ""
	if strings.HasSuffix(strings.ToLower(modelsPath), "/models") {
		modelsPrefix = modelsPath[:len(modelsPath)-len("/models")]
	}
	for _, suffix := range suffixes {
		if strings.HasPrefix(modelsPrefix, "/") {
			next := *base
			next.Path = modelsPrefix + strings.TrimPrefix(suffix, "/v1")
			result = append(result, next.String())
		}
		next := *base
		next.Path = basePath + suffix
		result = append(result, next.String())
		next.Path = suffix
		result = append(result, next.String())
	}
	return uniqueStrings(result)
}

func isChannelDiscoveryProbeResponse(protocol string, body []byte) bool {
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil || payload["error"] != nil {
		return false
	}
	field := map[string]string{
		"responses": "output",
		"chat":      "choices",
		"messages":  "content",
	}[protocol]
	_, ok := payload[field].([]any)
	return field != "" && ok
}

func isChannelDiscoveryStreamProbePayload(protocol string, body []byte) bool {
	if isChannelDiscoveryProbeResponse(protocol, body) {
		return true
	}
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil || payload["error"] != nil {
		return false
	}
	eventType, _ := payload["type"].(string)
	switch protocol {
	case "responses":
		return strings.HasPrefix(eventType, "response.") && eventType != "response.failed"
	case "messages":
		return strings.HasPrefix(eventType, "message_") || strings.HasPrefix(eventType, "content_block_")
	default:
		return false
	}
}

func readChannelDiscoveryStreamProbe(protocol string, reader io.Reader) (bool, error) {
	limited := io.LimitReader(reader, channelDiscoveryMaxResponseBytes+1)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64*1024), channelDiscoveryMaxResponseBytes+1)
	total := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		total += len(line) + 1
		if total > channelDiscoveryMaxResponseBytes {
			return false, fmt.Errorf("upstream response exceeds %d MiB", channelDiscoveryMaxResponseBytes/(1024*1024))
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if isChannelDiscoveryStreamProbePayload(protocol, payload) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func channelDiscoveryProbeBody(protocol string, modelName string, stream bool) any {
	// Protocol probes are real inference calls, so keep their output bound small.
	message := "Reply with OK."
	body := map[string]any{"model": modelName}
	if protocol == "responses" {
		body["input"] = message
		body["max_output_tokens"] = 32
	} else {
		body["messages"] = []map[string]string{{"role": "user", "content": message}}
		body["max_tokens"] = 32
	}
	if stream {
		body["stream"] = true
	}
	return body
}

func channelDiscoveryHTTPClient(origin string) *http.Client {
	// Copy the shared protected client before changing timeout/redirect behavior;
	// redirects remain same-origin so pasted credentials cannot cross hosts.
	base := service.GetSSRFProtectedHTTPClient()
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	client.Timeout = channelDiscoveryRequestTimeout
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme+"://"+req.URL.Host != origin {
			return errors.New("cross-origin redirect blocked")
		}
		return service.ValidateSSRFProtectedFetchURL(req.URL.String())
	}
	return &client
}

func readChannelDiscoveryBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, channelDiscoveryMaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > channelDiscoveryMaxResponseBytes {
		return nil, fmt.Errorf("upstream response exceeds %d MiB", channelDiscoveryMaxResponseBytes/(1024*1024))
	}
	return body, nil
}

func sanitizeChannelDiscoveryError(err error, key string) error {
	message := err.Error()
	if key != "" {
		message = strings.ReplaceAll(message, key, "[REDACTED]")
		message = strings.ReplaceAll(message, url.QueryEscape(key), "[REDACTED]")
	}
	return errors.New(message)
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
