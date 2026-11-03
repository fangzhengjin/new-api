-- 将 PostgreSQL 中的 Claude 历史日志统一为 new-api 总输入口径。
--
-- 作用范围：
-- 1. 仅处理当前小时和上一小时之前的消费日志，避免触碰仍在聚合的数据
-- 2. logs.prompt_tokens 更新为普通输入 + 缓存读取 + 缓存写入
-- 3. other.cache_xxx 保持不变，并写入 input_tokens_total 作为统一口径标记
-- 4. quota_data.token_used 根据迁移后的完整日志重新聚合
-- 5. 不修改 quota、count、用户余额或实际扣费
-- 6. 缺失或 count/quota 不一致的 quota_data 组会跳过并在结果中报告
--
-- 脚本可重复执行；已经统一的日志不会再次累加缓存。执行前仍应完成数据库备份。

BEGIN;

SET LOCAL lock_timeout = '10s';

CREATE TEMP TABLE claude_token_migration_scope ON COMMIT DROP AS
SELECT
  FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) / 3600)::BIGINT * 3600 - 3600 AS cutoff;

CREATE OR REPLACE FUNCTION pg_temp.claude_log_json(log_id BIGINT, document TEXT)
RETURNS JSONB
LANGUAGE plpgsql
IMMUTABLE
AS $function$
BEGIN
  IF document IS NULL OR BTRIM(document) = '' THEN
    RETURN '{}'::JSONB;
  END IF;
  RETURN document::JSONB;
EXCEPTION WHEN OTHERS THEN
  RAISE EXCEPTION 'logs.id=% 的 other 不是合法 JSON: %', log_id, SQLERRM;
END
$function$;

CREATE TEMP TABLE claude_log_updates ON COMMIT DROP AS
WITH parsed AS (
  SELECT
    l.id,
    l.user_id,
    l.username,
    l.model_name,
    l.created_at - (l.created_at % 3600) AS hour_at,
    l."group" AS use_group,
    l.token_id,
    l.channel_id,
    l.prompt_tokens,
    pg_temp.claude_log_json(l.id, l.other) AS other_json
  FROM logs l
  CROSS JOIN claude_token_migration_scope scope
  WHERE l.type = 2
    AND l.created_at < scope.cutoff
), token_parts AS (
  SELECT
    parsed.*,
    CASE
      WHEN JSONB_TYPEOF(other_json -> 'input_tokens_total') = 'number'
        THEN (other_json ->> 'input_tokens_total')::BIGINT
      ELSE 0
    END AS existing_input_total,
    GREATEST(
      COALESCE((other_json ->> 'cache_tokens')::BIGINT, 0),
      0
    ) AS cache_read,
    GREATEST(
      COALESCE((other_json ->> 'cache_write_tokens')::BIGINT, 0),
      COALESCE((other_json ->> 'cache_creation_tokens')::BIGINT, 0),
      COALESCE((other_json ->> 'cache_creation_tokens_5m')::BIGINT, 0)
        + COALESCE((other_json ->> 'cache_creation_tokens_1h')::BIGINT, 0),
      0
    ) AS cache_write
  FROM parsed
  WHERE other_json ->> 'usage_semantic' = 'anthropic'
     OR (
       COALESCE(other_json ->> 'usage_semantic', '') = ''
       AND other_json ->> 'claude' = 'true'
     )
), normalized AS (
  SELECT
    token_parts.*,
    CASE
      WHEN existing_input_total > 0 THEN existing_input_total
      ELSE prompt_tokens::BIGINT + cache_read + cache_write
    END AS expected_prompt_tokens
  FROM token_parts
)
SELECT
  id,
  user_id,
  username,
  model_name,
  hour_at,
  use_group,
  token_id,
  channel_id,
  prompt_tokens AS old_prompt_tokens,
  expected_prompt_tokens,
  other_json
FROM normalized
WHERE expected_prompt_tokens IS DISTINCT FROM prompt_tokens::BIGINT
   OR (
     (cache_read > 0 OR cache_write > 0)
     AND existing_input_total IS DISTINCT FROM expected_prompt_tokens
   );

CREATE UNIQUE INDEX claude_log_updates_id_idx ON claude_log_updates (id);

DO $migration$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM claude_log_updates
    WHERE expected_prompt_tokens < old_prompt_tokens
       OR expected_prompt_tokens > 2147483647
  ) THEN
    RAISE EXCEPTION '存在总输入小于原 prompt_tokens 或超过 Int32 的日志，迁移已取消';
  END IF;
END
$migration$;

CREATE TABLE IF NOT EXISTS migration_claude_token_logs_backup
  (LIKE logs INCLUDING DEFAULTS);
CREATE UNIQUE INDEX IF NOT EXISTS migration_claude_token_logs_backup_id_idx
  ON migration_claude_token_logs_backup (id);

INSERT INTO migration_claude_token_logs_backup
SELECT l.*
FROM logs l
JOIN claude_log_updates updates ON updates.id = l.id
ON CONFLICT (id) DO NOTHING;

UPDATE logs l
SET
  prompt_tokens = updates.expected_prompt_tokens::INTEGER,
  other = JSONB_SET(
    updates.other_json,
    '{input_tokens_total}',
    TO_JSONB(updates.expected_prompt_tokens),
    true
  )::TEXT
FROM claude_log_updates updates
WHERE l.id = updates.id;

CREATE TEMP TABLE claude_affected_groups ON COMMIT DROP AS
SELECT DISTINCT
  user_id,
  username,
  model_name,
  hour_at,
  use_group,
  token_id,
  channel_id
FROM claude_log_updates;

CREATE TEMP TABLE claude_quota_expected ON COMMIT DROP AS
SELECT
  affected.user_id,
  affected.username,
  affected.model_name,
  affected.hour_at,
  affected.use_group,
  affected.token_id,
  affected.channel_id,
  COUNT(*)::BIGINT AS expected_count,
  SUM(l.quota)::BIGINT AS expected_quota,
  SUM(l.prompt_tokens::BIGINT + l.completion_tokens::BIGINT) AS expected_token_used
FROM claude_affected_groups affected
JOIN logs l
  ON l.type = 2
 AND l.user_id = affected.user_id
 AND l.username IS NOT DISTINCT FROM affected.username
 AND l.model_name IS NOT DISTINCT FROM affected.model_name
 AND l.created_at >= affected.hour_at
 AND l.created_at < affected.hour_at + 3600
 AND l."group" IS NOT DISTINCT FROM affected.use_group
 AND l.token_id = affected.token_id
 AND l.channel_id = affected.channel_id
GROUP BY
  affected.user_id,
  affected.username,
  affected.model_name,
  affected.hour_at,
  affected.use_group,
  affected.token_id,
  affected.channel_id;

CREATE TEMP TABLE claude_quota_matches ON COMMIT DROP AS
SELECT
  expected.*,
  COUNT(q.id)::BIGINT AS matched_rows,
  MIN(q.id) AS quota_data_id,
  COALESCE(SUM(q.count), 0)::BIGINT AS current_count,
  COALESCE(SUM(q.quota), 0)::BIGINT AS current_quota,
  COALESCE(SUM(q.token_used), 0)::BIGINT AS current_token_used
FROM claude_quota_expected expected
LEFT JOIN quota_data q
  ON q.user_id = expected.user_id
 AND q.username IS NOT DISTINCT FROM expected.username
 AND q.model_name IS NOT DISTINCT FROM expected.model_name
 AND q.created_at = expected.hour_at
 AND q.use_group IS NOT DISTINCT FROM expected.use_group
 AND q.token_id = expected.token_id
 AND q.channel_id = expected.channel_id
GROUP BY
  expected.user_id,
  expected.username,
  expected.model_name,
  expected.hour_at,
  expected.use_group,
  expected.token_id,
  expected.channel_id,
  expected.expected_count,
  expected.expected_quota,
  expected.expected_token_used;

DO $migration$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM claude_quota_matches
    WHERE matched_rows > 0
      AND current_count = expected_count
      AND current_quota = expected_quota
      AND (
        expected_token_used < 0
        OR expected_token_used > 2147483647
      )
  ) THEN
    RAISE EXCEPTION '重算后的 quota_data.token_used 超出 Int32，迁移已回滚';
  END IF;
END
$migration$;

CREATE TEMP TABLE claude_quota_updates ON COMMIT DROP AS
SELECT
  matches.quota_data_id,
  q.token_used::BIGINT
    + matches.expected_token_used
    - matches.current_token_used AS new_token_used
FROM claude_quota_matches matches
JOIN quota_data q ON q.id = matches.quota_data_id
WHERE matches.matched_rows > 0
  AND matches.current_count = matches.expected_count
  AND matches.current_quota = matches.expected_quota
  AND matches.current_token_used <> matches.expected_token_used;

CREATE UNIQUE INDEX claude_quota_updates_id_idx
  ON claude_quota_updates (quota_data_id);

DO $migration$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM claude_quota_updates
    WHERE new_token_used < 0
       OR new_token_used > 2147483647
  ) THEN
    RAISE EXCEPTION '调整后的 quota_data.token_used 超出 Int32，迁移已回滚';
  END IF;
END
$migration$;

CREATE TABLE IF NOT EXISTS migration_claude_token_quota_data_backup
  (LIKE quota_data INCLUDING DEFAULTS);
CREATE UNIQUE INDEX IF NOT EXISTS migration_claude_token_quota_data_backup_id_idx
  ON migration_claude_token_quota_data_backup (id);

INSERT INTO migration_claude_token_quota_data_backup
SELECT q.*
FROM quota_data q
JOIN claude_quota_updates updates ON updates.quota_data_id = q.id
ON CONFLICT (id) DO NOTHING;

UPDATE quota_data q
SET token_used = updates.new_token_used::INTEGER
FROM claude_quota_updates updates
WHERE q.id = updates.quota_data_id;

DO $migration$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM claude_quota_matches expected
    JOIN quota_data q
      ON q.user_id = expected.user_id
     AND q.username IS NOT DISTINCT FROM expected.username
     AND q.model_name IS NOT DISTINCT FROM expected.model_name
     AND q.created_at = expected.hour_at
     AND q.use_group IS NOT DISTINCT FROM expected.use_group
     AND q.token_id = expected.token_id
     AND q.channel_id = expected.channel_id
    WHERE expected.matched_rows > 0
      AND expected.current_count = expected.expected_count
      AND expected.current_quota = expected.expected_quota
    GROUP BY
      expected.user_id,
      expected.username,
      expected.model_name,
      expected.hour_at,
      expected.use_group,
      expected.token_id,
      expected.channel_id,
      expected.expected_token_used
    HAVING SUM(q.token_used)::BIGINT <> expected.expected_token_used
  ) THEN
    RAISE EXCEPTION 'quota_data.token_used 验证失败，迁移已回滚';
  END IF;
END
$migration$;

SELECT
  scope.cutoff,
  TO_TIMESTAMP(scope.cutoff) AS migrated_before,
  (SELECT COUNT(*) FROM claude_log_updates) AS updated_logs,
  (SELECT COUNT(*) FROM claude_quota_updates) AS updated_quota_rows,
  (
    SELECT COUNT(*)
    FROM claude_quota_matches
    WHERE matched_rows = 0
  ) AS skipped_missing_quota_groups,
  (
    SELECT COUNT(*)
    FROM claude_quota_matches
    WHERE matched_rows > 0
      AND (
        current_count <> expected_count
        OR current_quota <> expected_quota
      )
  ) AS skipped_mismatched_quota_groups,
  (
    SELECT COUNT(*)
    FROM claude_quota_matches
    WHERE matched_rows > 1
  ) AS multi_row_quota_groups
FROM claude_token_migration_scope scope;

COMMIT;

-- 如需回滚本脚本已经迁移过的全部记录，请单独执行：
-- BEGIN;
-- UPDATE logs l
-- SET
--   prompt_tokens = backup.prompt_tokens,
--   other = backup.other
-- FROM migration_claude_token_logs_backup backup
-- WHERE l.id = backup.id;
-- UPDATE quota_data q
-- SET token_used = backup.token_used
-- FROM migration_claude_token_quota_data_backup backup
-- WHERE q.id = backup.id;
-- COMMIT;
