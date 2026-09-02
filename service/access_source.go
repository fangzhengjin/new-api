package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/go-redis/redis/v8"
)

const (
	accessSourceNamespace                   = "accessSource:v1"
	accessSourceAssociationRetentionSeconds = int64(7 * 24 * 60 * 60)
	accessSourceStateRetentionSeconds       = int64(24 * 60 * 60)
	accessSourceRejectionLimit              = 10
)

type AccessSourceRejectionReason string

const (
	AccessSourceSwitchCooldown AccessSourceRejectionReason = "switch_cooldown"
	AccessSourceAccountIPLimit AccessSourceRejectionReason = "account_ip_limit"
	AccessSourceIPAccountLimit AccessSourceRejectionReason = "ip_account_limit"
)

var ErrAccessSourcePendingChanged = errors.New("access source pending changed")
var ErrAccessSourceCurrent = errors.New("current access source cannot be removed")
var ErrAccessSourceInvalidIP = errors.New("invalid access source IP")

type AccessSourceDecision struct {
	Allowed           bool
	Reason            AccessSourceRejectionReason
	RetryAfterSeconds int64
}

type AccessSourceAssociation struct {
	IP         string `json:"ip"`
	LastSeenAt int64  `json:"last_seen_at"`
	IsCurrent  bool   `json:"is_current"`
}

type AccessSourceRejection struct {
	EventID    string                      `json:"event_id"`
	IP         string                      `json:"ip"`
	Reason     AccessSourceRejectionReason `json:"reason"`
	OccurredAt int64                       `json:"occurred_at"`
}

type AccessSourceState struct {
	CurrentIP                string                    `json:"current_ip"`
	CurrentLastSeenAt        int64                     `json:"current_last_seen_at"`
	AssociatedCount          int64                     `json:"associated_count"`
	CooldownRemainingSeconds int64                     `json:"cooldown_remaining_seconds"`
	Associations             []AccessSourceAssociation `json:"associations"`
	Pending                  *AccessSourceRejection    `json:"pending"`
	RecentRejections         []AccessSourceRejection   `json:"recent_rejections"`
}

const checkAccessSourceScript = `
local now = redis.call('TIME')
local now_seconds = tonumber(now[1])
local now_us = now_seconds * 1000000 + tonumber(now[2])
local retention_us = tonumber(ARGV[7]) * 1000000
local window_us = tonumber(ARGV[3]) * 3600 * 1000000
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now_us - retention_us)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now_us - retention_us)

local function reject(reason, retry_after)
  local event_id = ARGV[10]
  local entry = event_id .. '\t' .. now[1] .. '\t' .. ARGV[2] .. '\t' .. reason
  redis.call('ZREMRANGEBYSCORE', KEYS[4], '-inf', now_us - tonumber(ARGV[8]) * 1000000)
  redis.call('ZADD', KEYS[4], now_us, entry)
  local history_count = redis.call('ZCARD', KEYS[4])
  if history_count > tonumber(ARGV[9]) then
    redis.call('ZREMRANGEBYRANK', KEYS[4], 0, history_count - tonumber(ARGV[9]) - 1)
  end
  redis.call('EXPIRE', KEYS[4], ARGV[8])
  redis.call('SET', KEYS[5], entry, 'EX', ARGV[8])
  return {0, reason, retry_after}
end

local current_ip = redis.call('HGET', KEYS[3], 'ip')
local current_last_seen = tonumber(redis.call('HGET', KEYS[3], 'last_seen_seconds') or '0') * 1000000 + tonumber(redis.call('HGET', KEYS[3], 'last_seen_microseconds') or '0')
if current_ip and current_ip ~= ARGV[2] and tonumber(ARGV[5]) > 0 then
  local remaining_us = current_last_seen + tonumber(ARGV[5]) * 60 * 1000000 - now_us
  if remaining_us > 0 then
    return reject('switch_cooldown', math.ceil(remaining_us / 1000000))
  end
end

local window_start = now_us - window_us
local account_seen = tonumber(redis.call('ZSCORE', KEYS[1], ARGV[2]) or '0')
local ip_seen = tonumber(redis.call('ZSCORE', KEYS[2], ARGV[1]) or '0')
local known = account_seen >= window_start and ip_seen >= window_start
if not known and tonumber(ARGV[4]) > 0 and redis.call('ZCOUNT', KEYS[1], window_start, '+inf') >= tonumber(ARGV[4]) then
  return reject('account_ip_limit', 0)
end
if not known and tonumber(ARGV[6]) > 0 and redis.call('ZCOUNT', KEYS[2], window_start, '+inf') >= tonumber(ARGV[6]) then
  return reject('ip_account_limit', 0)
end

redis.call('ZADD', KEYS[1], now_us, ARGV[2])
redis.call('ZADD', KEYS[2], now_us, ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[7])
redis.call('EXPIRE', KEYS[2], ARGV[7])
redis.call('HSET', KEYS[3], 'ip', ARGV[2], 'last_seen_seconds', now[1], 'last_seen_microseconds', now[2])
redis.call('EXPIRE', KEYS[3], ARGV[8])
local pending = redis.call('GET', KEYS[5])
if pending and string.match(pending, '^[^\t]*\t[^\t]*\t([^\t]*)\t') == ARGV[2] then
  redis.call('DEL', KEYS[5])
end
return {1, '', 0}
`

const allowLatestAccessSourceScript = `
if redis.call('GET', KEYS[4]) ~= ARGV[3] then
  return 0
end
local now = redis.call('TIME')
local now_us = tonumber(now[1]) * 1000000 + tonumber(now[2])
redis.call('ZADD', KEYS[1], now_us, ARGV[2])
redis.call('ZADD', KEYS[2], now_us, ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[4])
redis.call('EXPIRE', KEYS[2], ARGV[4])
redis.call('HSET', KEYS[3], 'ip', ARGV[2], 'last_seen_seconds', now[1], 'last_seen_microseconds', now[2])
redis.call('EXPIRE', KEYS[3], ARGV[5])
redis.call('DEL', KEYS[4])
return 1
`

const removeAccessSourceAssociationScript = `
if redis.call('HGET', KEYS[3], 'ip') == ARGV[2] then
  return -1
end
local removed = redis.call('ZREM', KEYS[1], ARGV[2]) + redis.call('ZREM', KEYS[2], ARGV[1])
return removed
`

func accessSourceAccountKey(userID int) string {
	return fmt.Sprintf("%s:account:%d:ips", accessSourceNamespace, userID)
}

func accessSourceIPKey(ip string) string {
	return fmt.Sprintf("%s:ip:%s:accounts", accessSourceNamespace, ip)
}

func accessSourceCurrentKey(userID int) string {
	return fmt.Sprintf("%s:account:%d:current", accessSourceNamespace, userID)
}

func accessSourceHistoryKey(userID int) string {
	return fmt.Sprintf("%s:account:%d:rejections", accessSourceNamespace, userID)
}

func accessSourcePendingKey(userID int) string {
	return fmt.Sprintf("%s:account:%d:pending", accessSourceNamespace, userID)
}

func accessSourceKeys(userID int, ip string) []string {
	return []string{
		accessSourceAccountKey(userID),
		accessSourceIPKey(ip),
		accessSourceCurrentKey(userID),
		accessSourceHistoryKey(userID),
		accessSourcePendingKey(userID),
	}
}

func normalizeAccessSourceIP(ip string) (string, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return "", ErrAccessSourceInvalidIP
	}
	return parsed.String(), nil
}

func validateAccessSourceLimits(limits setting.AccessSourceLimits) error {
	if limits.AssociationWindowHours < 1 || limits.AssociationWindowHours > setting.MaxAccessSourceAssociationWindowHours {
		return errors.New("访问来源关联统计周期无效")
	}
	if limits.MaxIPsPerUser < 0 || limits.MaxIPsPerUser > setting.MaxAccessSourceAssociations ||
		limits.MaxUsersPerIP < 0 || limits.MaxUsersPerIP > setting.MaxAccessSourceAssociations {
		return errors.New("访问来源关联数量无效")
	}
	if limits.SwitchCooldownMinutes < 0 || limits.SwitchCooldownMinutes > setting.MaxAccessSourceSwitchCooldownMinutes {
		return errors.New("访问来源切换等待时间无效")
	}
	return nil
}

// CheckAccessSource atomically validates and records one accepted account/IP relationship.
func CheckAccessSource(ctx context.Context, userID int, ip string, limits setting.AccessSourceLimits) (AccessSourceDecision, error) {
	if userID <= 0 {
		return AccessSourceDecision{}, errors.New("账户或来源 IP 无效")
	}
	if !limits.Enabled {
		return AccessSourceDecision{Allowed: true}, nil
	}
	if err := validateAccessSourceLimits(limits); err != nil {
		return AccessSourceDecision{}, err
	}
	if limits.MaxIPsPerUser == 0 && limits.SwitchCooldownMinutes == 0 && limits.MaxUsersPerIP == 0 {
		return AccessSourceDecision{Allowed: true}, nil
	}
	ip, err := normalizeAccessSourceIP(ip)
	if err != nil {
		return AccessSourceDecision{}, err
	}
	if common.RedisEnabled {
		if common.RDB == nil {
			return AccessSourceDecision{}, errors.New("Redis 客户端未初始化")
		}
		values, err := common.RDB.Eval(ctx, checkAccessSourceScript, accessSourceKeys(userID, ip),
			userID,
			ip,
			limits.AssociationWindowHours,
			limits.MaxIPsPerUser,
			limits.SwitchCooldownMinutes,
			limits.MaxUsersPerIP,
			accessSourceAssociationRetentionSeconds,
			accessSourceStateRetentionSeconds,
			accessSourceRejectionLimit,
			common.GetUUID(),
		).Slice()
		if err != nil {
			return AccessSourceDecision{}, err
		}
		if len(values) != 3 {
			return AccessSourceDecision{}, fmt.Errorf("访问来源检查返回值长度无效: %d", len(values))
		}
		allowed, err := redisInteger(values[0])
		if err != nil {
			return AccessSourceDecision{}, err
		}
		retryAfter, err := redisInteger(values[2])
		if err != nil {
			return AccessSourceDecision{}, err
		}
		return AccessSourceDecision{
			Allowed:           allowed == 1,
			Reason:            AccessSourceRejectionReason(fmt.Sprint(values[1])),
			RetryAfterSeconds: retryAfter,
		}, nil
	}
	return checkAccessSourceMemory(userID, ip, limits, time.Now())
}

// GetAccessSourceState returns the current-window relationships and the latest ten rejection records.
func GetAccessSourceState(ctx context.Context, userID int, limits setting.AccessSourceLimits) (AccessSourceState, error) {
	if userID <= 0 {
		return AccessSourceState{}, errors.New("账户无效")
	}
	if err := validateAccessSourceLimits(limits); err != nil {
		return AccessSourceState{}, err
	}
	if common.RedisEnabled {
		return getAccessSourceStateRedis(ctx, userID, limits, time.Now())
	}
	return getAccessSourceStateMemory(userID, limits, time.Now())
}

// AllowLatestAccessSource accepts the latest rejected IP only when the pending event is unchanged.
func AllowLatestAccessSource(ctx context.Context, userID int, ip string, eventID string) (AccessSourceRejection, error) {
	if userID <= 0 || eventID == "" {
		return AccessSourceRejection{}, errors.New("待处理来源参数无效")
	}
	ip, err := normalizeAccessSourceIP(ip)
	if err != nil {
		return AccessSourceRejection{}, err
	}
	if common.RedisEnabled {
		if common.RDB == nil {
			return AccessSourceRejection{}, errors.New("Redis 客户端未初始化")
		}
		pendingRaw, err := common.RDB.Get(ctx, accessSourcePendingKey(userID)).Result()
		if errors.Is(err, redis.Nil) {
			return AccessSourceRejection{}, ErrAccessSourcePendingChanged
		}
		if err != nil {
			return AccessSourceRejection{}, err
		}
		pending, err := parseAccessSourceRejection(pendingRaw)
		if err != nil || pending.EventID != eventID || pending.IP != ip {
			return AccessSourceRejection{}, ErrAccessSourcePendingChanged
		}
		keys := []string{
			accessSourceAccountKey(userID),
			accessSourceIPKey(ip),
			accessSourceCurrentKey(userID),
			accessSourcePendingKey(userID),
		}
		allowed, err := common.RDB.Eval(ctx, allowLatestAccessSourceScript, keys,
			userID, ip, pendingRaw, accessSourceAssociationRetentionSeconds, accessSourceStateRetentionSeconds,
		).Int()
		if err != nil {
			return AccessSourceRejection{}, err
		}
		if allowed != 1 {
			return AccessSourceRejection{}, ErrAccessSourcePendingChanged
		}
		return pending, nil
	}
	return allowLatestAccessSourceMemory(userID, ip, eventID, time.Now())
}

// RemoveAccessSourceAssociation removes both directions of one stored relationship.
func RemoveAccessSourceAssociation(ctx context.Context, userID int, ip string) (bool, error) {
	if userID <= 0 {
		return false, errors.New("来源关联参数无效")
	}
	ip, err := normalizeAccessSourceIP(ip)
	if err != nil {
		return false, err
	}
	if common.RedisEnabled {
		if common.RDB == nil {
			return false, errors.New("Redis 客户端未初始化")
		}
		keys := accessSourceKeys(userID, ip)
		removed, err := common.RDB.Eval(ctx, removeAccessSourceAssociationScript, keys[:3], userID, ip).Int()
		if err != nil {
			return false, err
		}
		if removed < 0 {
			return false, ErrAccessSourceCurrent
		}
		return removed > 0, nil
	}
	return removeAccessSourceAssociationMemory(userID, ip, time.Now())
}

func redisInteger(value interface{}) (int64, error) {
	parsed, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("Redis 返回了无效整数 %q: %w", fmt.Sprint(value), err)
	}
	return parsed, nil
}

func parseAccessSourceRejection(raw string) (AccessSourceRejection, error) {
	parts := strings.Split(raw, "\t")
	if len(parts) != 4 {
		return AccessSourceRejection{}, errors.New("访问来源拒绝记录格式无效")
	}
	occurredAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return AccessSourceRejection{}, fmt.Errorf("访问来源拒绝时间无效: %w", err)
	}
	reason := AccessSourceRejectionReason(parts[3])
	if reason != AccessSourceSwitchCooldown && reason != AccessSourceAccountIPLimit && reason != AccessSourceIPAccountLimit {
		return AccessSourceRejection{}, errors.New("访问来源拒绝原因无效")
	}
	return AccessSourceRejection{EventID: parts[0], OccurredAt: occurredAt, IP: parts[2], Reason: reason}, nil
}

func getAccessSourceStateRedis(ctx context.Context, userID int, limits setting.AccessSourceLimits, now time.Time) (AccessSourceState, error) {
	if common.RDB == nil {
		return AccessSourceState{}, errors.New("Redis 客户端未初始化")
	}
	redisNow, err := common.RDB.Time(ctx).Result()
	if err != nil {
		return AccessSourceState{}, err
	}
	now = redisNow
	nowMicros := now.UnixMicro()
	windowStart := nowMicros - int64(limits.AssociationWindowHours)*int64(time.Hour/time.Microsecond)
	retentionStart := nowMicros - accessSourceAssociationRetentionSeconds*int64(time.Second/time.Microsecond)
	pipe := common.RDB.Pipeline()
	pipe.ZRemRangeByScore(ctx, accessSourceAccountKey(userID), "-inf", strconv.FormatInt(retentionStart, 10))
	associationsCmd := pipe.ZRevRangeByScoreWithScores(ctx, accessSourceAccountKey(userID), &redis.ZRangeBy{
		Min: strconv.FormatInt(windowStart, 10),
		Max: "+inf",
	})
	currentCmd := pipe.HGetAll(ctx, accessSourceCurrentKey(userID))
	pendingCmd := pipe.Get(ctx, accessSourcePendingKey(userID))
	pipe.ZRemRangeByScore(ctx, accessSourceHistoryKey(userID), "-inf", strconv.FormatInt(nowMicros-accessSourceStateRetentionSeconds*int64(time.Second/time.Microsecond), 10))
	historyCmd := pipe.ZRevRange(ctx, accessSourceHistoryKey(userID), 0, accessSourceRejectionLimit-1)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return AccessSourceState{}, err
	}

	state := AccessSourceState{}
	current, err := currentCmd.Result()
	if err != nil {
		return AccessSourceState{}, err
	}
	state.CurrentIP = current["ip"]
	if rawSeconds := current["last_seen_seconds"]; rawSeconds != "" {
		lastSeenSeconds, err := strconv.ParseInt(rawSeconds, 10, 64)
		if err != nil {
			return AccessSourceState{}, fmt.Errorf("当前来源活动时间无效: %w", err)
		}
		lastSeenMicroseconds, err := strconv.ParseInt(current["last_seen_microseconds"], 10, 64)
		if err != nil {
			return AccessSourceState{}, fmt.Errorf("当前来源活动微秒无效: %w", err)
		}
		lastSeen := lastSeenSeconds*int64(time.Second/time.Microsecond) + lastSeenMicroseconds
		state.CurrentLastSeenAt = lastSeen / int64(time.Second/time.Microsecond)
		remaining := lastSeen + int64(limits.SwitchCooldownMinutes)*int64(time.Minute/time.Microsecond) - nowMicros
		if remaining > 0 {
			state.CooldownRemainingSeconds = (remaining + int64(time.Second/time.Microsecond) - 1) / int64(time.Second/time.Microsecond)
		}
	}

	associations, err := associationsCmd.Result()
	if err != nil {
		return AccessSourceState{}, err
	}
	state.AssociatedCount = int64(len(associations))
	state.Associations = make([]AccessSourceAssociation, 0, len(associations))
	for _, association := range associations {
		ip := fmt.Sprint(association.Member)
		state.Associations = append(state.Associations, AccessSourceAssociation{
			IP:         ip,
			LastSeenAt: int64(association.Score) / int64(time.Second/time.Microsecond),
			IsCurrent:  ip == state.CurrentIP,
		})
	}

	pendingRaw, err := pendingCmd.Result()
	if err == nil {
		pending, err := parseAccessSourceRejection(pendingRaw)
		if err != nil {
			return AccessSourceState{}, err
		}
		state.Pending = &pending
	} else if !errors.Is(err, redis.Nil) {
		return AccessSourceState{}, err
	}
	history, err := historyCmd.Result()
	if err != nil {
		return AccessSourceState{}, err
	}
	state.RecentRejections = make([]AccessSourceRejection, 0, len(history))
	for _, raw := range history {
		rejection, err := parseAccessSourceRejection(raw)
		if err != nil {
			return AccessSourceState{}, err
		}
		state.RecentRejections = append(state.RecentRejections, rejection)
	}
	return state, nil
}

type memoryAccessSourceAccount struct {
	associations    map[string]int64
	currentIP       string
	currentLastSeen int64
	rejections      []string
	pending         string
}

type memoryAccessSourceStore struct {
	mutex       sync.Mutex
	accounts    map[int]*memoryAccessSourceAccount
	ips         map[string]map[int]int64
	nextCleanup int64
	sequence    uint64
}

var accessSourceMemory = memoryAccessSourceStore{
	accounts: make(map[int]*memoryAccessSourceAccount),
	ips:      make(map[string]map[int]int64),
}

func (store *memoryAccessSourceStore) cleanupLocked(nowMicros int64) {
	if nowMicros < store.nextCleanup {
		return
	}
	retentionStart := nowMicros - accessSourceAssociationRetentionSeconds*int64(time.Second/time.Microsecond)
	for userID, account := range store.accounts {
		for ip, lastSeen := range account.associations {
			if lastSeen > retentionStart {
				continue
			}
			delete(account.associations, ip)
			if users := store.ips[ip]; users != nil {
				delete(users, userID)
				if len(users) == 0 {
					delete(store.ips, ip)
				}
			}
		}
		pruneMemoryAccountStateLocked(account, nowMicros)
		if len(account.associations) == 0 && account.currentIP == "" && len(account.rejections) == 0 && account.pending == "" {
			delete(store.accounts, userID)
		}
	}
	store.nextCleanup = nowMicros + int64(time.Hour/time.Microsecond)
}

func (store *memoryAccessSourceStore) accountLocked(userID int) *memoryAccessSourceAccount {
	account := store.accounts[userID]
	if account == nil {
		account = &memoryAccessSourceAccount{associations: make(map[string]int64)}
		store.accounts[userID] = account
	}
	return account
}

func pruneMemoryAccountStateLocked(account *memoryAccessSourceAccount, nowMicros int64) {
	stateStart := nowMicros - accessSourceStateRetentionSeconds*int64(time.Second/time.Microsecond)
	if account.currentLastSeen <= stateStart {
		account.currentIP = ""
		account.currentLastSeen = 0
	}
	kept := account.rejections[:0]
	for _, raw := range account.rejections {
		rejection, err := parseAccessSourceRejection(raw)
		if err == nil && rejection.OccurredAt*int64(time.Second/time.Microsecond) > stateStart {
			kept = append(kept, raw)
		}
	}
	account.rejections = kept
	if pending, err := parseAccessSourceRejection(account.pending); err != nil || pending.OccurredAt*int64(time.Second/time.Microsecond) <= stateStart {
		account.pending = ""
	}
}

func (store *memoryAccessSourceStore) rejectLocked(account *memoryAccessSourceAccount, ip string, reason AccessSourceRejectionReason, retryAfter int64, now time.Time) AccessSourceDecision {
	store.sequence++
	raw := fmt.Sprintf("%d-%d\t%d\t%s\t%s", now.UnixMicro(), store.sequence, now.Unix(), ip, reason)
	account.rejections = append([]string{raw}, account.rejections...)
	if len(account.rejections) > accessSourceRejectionLimit {
		account.rejections = account.rejections[:accessSourceRejectionLimit]
	}
	account.pending = raw
	return AccessSourceDecision{Reason: reason, RetryAfterSeconds: retryAfter}
}

func checkAccessSourceMemory(userID int, ip string, limits setting.AccessSourceLimits, now time.Time) (AccessSourceDecision, error) {
	store := &accessSourceMemory
	store.mutex.Lock()
	defer store.mutex.Unlock()
	nowMicros := now.UnixMicro()
	store.cleanupLocked(nowMicros)
	account := store.accountLocked(userID)
	pruneMemoryAccountStateLocked(account, nowMicros)
	if account.currentIP != "" && account.currentIP != ip && limits.SwitchCooldownMinutes > 0 {
		remaining := account.currentLastSeen + int64(limits.SwitchCooldownMinutes)*int64(time.Minute/time.Microsecond) - nowMicros
		if remaining > 0 {
			retryAfter := (remaining + int64(time.Second/time.Microsecond) - 1) / int64(time.Second/time.Microsecond)
			return store.rejectLocked(account, ip, AccessSourceSwitchCooldown, retryAfter, now), nil
		}
	}
	ipAccounts := store.ips[ip]
	windowStart := nowMicros - int64(limits.AssociationWindowHours)*int64(time.Hour/time.Microsecond)
	known := account.associations[ip] >= windowStart && ipAccounts != nil && ipAccounts[userID] >= windowStart
	if !known && limits.MaxIPsPerUser > 0 {
		count := 0
		for _, lastSeen := range account.associations {
			if lastSeen >= windowStart {
				count++
			}
		}
		if count >= limits.MaxIPsPerUser {
			return store.rejectLocked(account, ip, AccessSourceAccountIPLimit, 0, now), nil
		}
	}
	if !known && limits.MaxUsersPerIP > 0 {
		count := 0
		for _, lastSeen := range ipAccounts {
			if lastSeen >= windowStart {
				count++
			}
		}
		if count >= limits.MaxUsersPerIP {
			return store.rejectLocked(account, ip, AccessSourceIPAccountLimit, 0, now), nil
		}
	}
	if ipAccounts == nil {
		ipAccounts = make(map[int]int64)
		store.ips[ip] = ipAccounts
	}
	account.associations[ip] = nowMicros
	ipAccounts[userID] = nowMicros
	account.currentIP = ip
	account.currentLastSeen = nowMicros
	if pending, err := parseAccessSourceRejection(account.pending); err == nil && pending.IP == ip {
		account.pending = ""
	}
	return AccessSourceDecision{Allowed: true}, nil
}

func getAccessSourceStateMemory(userID int, limits setting.AccessSourceLimits, now time.Time) (AccessSourceState, error) {
	store := &accessSourceMemory
	store.mutex.Lock()
	defer store.mutex.Unlock()
	nowMicros := now.UnixMicro()
	store.cleanupLocked(nowMicros)
	account := store.accounts[userID]
	if account == nil {
		return AccessSourceState{}, nil
	}
	pruneMemoryAccountStateLocked(account, nowMicros)
	state := AccessSourceState{CurrentIP: account.currentIP, CurrentLastSeenAt: account.currentLastSeen / int64(time.Second/time.Microsecond)}
	remaining := account.currentLastSeen + int64(limits.SwitchCooldownMinutes)*int64(time.Minute/time.Microsecond) - nowMicros
	if account.currentIP != "" && remaining > 0 {
		state.CooldownRemainingSeconds = (remaining + int64(time.Second/time.Microsecond) - 1) / int64(time.Second/time.Microsecond)
	}
	windowStart := nowMicros - int64(limits.AssociationWindowHours)*int64(time.Hour/time.Microsecond)
	for ip, lastSeen := range account.associations {
		if lastSeen < windowStart {
			continue
		}
		state.Associations = append(state.Associations, AccessSourceAssociation{
			IP: ip, LastSeenAt: lastSeen / int64(time.Second/time.Microsecond), IsCurrent: ip == account.currentIP,
		})
	}
	state.AssociatedCount = int64(len(state.Associations))
	sort.Slice(state.Associations, func(i, j int) bool {
		return state.Associations[i].LastSeenAt > state.Associations[j].LastSeenAt
	})
	if account.pending != "" {
		pending, err := parseAccessSourceRejection(account.pending)
		if err != nil {
			return AccessSourceState{}, err
		}
		state.Pending = &pending
	}
	for _, raw := range account.rejections {
		rejection, err := parseAccessSourceRejection(raw)
		if err != nil {
			return AccessSourceState{}, err
		}
		state.RecentRejections = append(state.RecentRejections, rejection)
	}
	return state, nil
}

func allowLatestAccessSourceMemory(userID int, ip string, eventID string, now time.Time) (AccessSourceRejection, error) {
	store := &accessSourceMemory
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.cleanupLocked(now.UnixMicro())
	account := store.accounts[userID]
	if account == nil || account.pending == "" {
		return AccessSourceRejection{}, ErrAccessSourcePendingChanged
	}
	pruneMemoryAccountStateLocked(account, now.UnixMicro())
	if account.pending == "" {
		return AccessSourceRejection{}, ErrAccessSourcePendingChanged
	}
	pending, err := parseAccessSourceRejection(account.pending)
	if err != nil || pending.EventID != eventID || pending.IP != ip {
		return AccessSourceRejection{}, ErrAccessSourcePendingChanged
	}
	nowMicros := now.UnixMicro()
	account.associations[ip] = nowMicros
	ipAccounts := store.ips[ip]
	if ipAccounts == nil {
		ipAccounts = make(map[int]int64)
		store.ips[ip] = ipAccounts
	}
	ipAccounts[userID] = nowMicros
	account.currentIP = ip
	account.currentLastSeen = nowMicros
	account.pending = ""
	return pending, nil
}

func removeAccessSourceAssociationMemory(userID int, ip string, now time.Time) (bool, error) {
	store := &accessSourceMemory
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.cleanupLocked(now.UnixMicro())
	account := store.accounts[userID]
	if account == nil {
		return false, nil
	}
	if account.currentIP == ip {
		return false, ErrAccessSourceCurrent
	}
	_, removed := account.associations[ip]
	delete(account.associations, ip)
	if ipAccounts := store.ips[ip]; ipAccounts != nil {
		if _, ok := ipAccounts[userID]; ok {
			removed = true
		}
		delete(ipAccounts, userID)
		if len(ipAccounts) == 0 {
			delete(store.ips, ip)
		}
	}
	return removed, nil
}
