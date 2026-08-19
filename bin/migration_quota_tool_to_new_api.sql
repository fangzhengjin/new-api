-- 将独立额度工具的 PostgreSQL 表转换为 new-api 额度管理模型。
--
-- 执行要求：
-- 1. 仅适用于独立工具 001_create_tables.sql 创建的 PostgreSQL 表
-- 2. 执行前停止独立工具和 new-api，确认当前 search_path 指向这些表所在 schema
-- 3. 先完成数据库备份；时间戳会降为 Unix 秒，脚本不提供无损回滚
-- 4. 脚本不删除表或业务记录，任一校验失败会回滚整个事务
-- 5. quota 已是 new-api 原始单位；脚本不会对任何 quota 字段乘除或按汇率换算
-- 6. 脚本成功后启动 new-api；AutoMigrate 创建结算账本后会自动补录旧消费

BEGIN;

SET LOCAL lock_timeout = '10s';

DO $migration$
BEGIN
  IF to_regclass('tool_quota_cycles') IS NULL THEN
    RAISE EXCEPTION '缺少表 tool_quota_cycles';
  END IF;
  IF to_regclass('tool_quota_adjustment_plans') IS NULL THEN
    RAISE EXCEPTION '缺少表 tool_quota_adjustment_plans';
  END IF;
  IF to_regclass('tool_quota_adjustment_items') IS NULL THEN
    RAISE EXCEPTION '缺少表 tool_quota_adjustment_items';
  END IF;
  IF to_regclass('logs') IS NULL THEN
    RAISE EXCEPTION '缺少旧工具使用的 logs 表，无法补录周期消费';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'tool_quota_cycles'
      AND column_name = 'initial_stage_percent'
  ) THEN
    RAISE EXCEPTION '缺少旧工具列 initial_stage_percent，源表不是受支持的版本';
  END IF;
END
$migration$;

LOCK TABLE
  tool_quota_cycles,
  tool_quota_adjustment_plans,
  tool_quota_adjustment_items
IN ACCESS EXCLUSIVE MODE;

-- 当前站点按 500000 quota = 1 USD = 1 CNY 展示。提前校验配置，避免迁移后
-- 因站点配置漂移而把旧工具的同一 raw quota 显示成不同金额。
DO $migration$
DECLARE
  quota_per_unit NUMERIC;
  usd_exchange_rate NUMERIC;
  quota_display_type TEXT;
BEGIN
  IF to_regclass('options') IS NULL THEN
    RAISE EXCEPTION '缺少 new-api options 表，无法确认额度展示配置';
  END IF;

  SELECT value::NUMERIC INTO quota_per_unit
  FROM options
  WHERE "key" = 'QuotaPerUnit';
  quota_per_unit := COALESCE(quota_per_unit, 500000);

  SELECT value::NUMERIC INTO usd_exchange_rate
  FROM options
  WHERE "key" = 'USDExchangeRate';

  SELECT value INTO quota_display_type
  FROM options
  WHERE "key" = 'general_setting.quota_display_type';

  IF quota_per_unit <> 500000 THEN
    RAISE EXCEPTION 'QuotaPerUnit 必须为 500000，实际为 %', quota_per_unit;
  END IF;
  IF usd_exchange_rate IS DISTINCT FROM 1 THEN
    RAISE EXCEPTION 'USDExchangeRate 必须显式设置为 1，实际为 %', usd_exchange_rate;
  END IF;
  IF quota_display_type IS DISTINCT FROM 'CNY' THEN
    RAISE EXCEPTION 'quota_display_type 必须为 CNY，实际为 %', quota_display_type;
  END IF;
END
$migration$;

-- 兼容源工具曾支持的“JSONB 中再包一层 JSON 字符串”数据。
CREATE OR REPLACE FUNCTION pg_temp.quota_json_object(document JSONB)
RETURNS JSONB
LANGUAGE plpgsql
IMMUTABLE
AS $function$
DECLARE
  normalized JSONB;
BEGIN
  IF jsonb_typeof(document) = 'object' THEN
    RETURN document;
  END IF;
  IF jsonb_typeof(document) = 'string' THEN
    normalized := (document #>> '{}')::JSONB;
    IF jsonb_typeof(normalized) = 'object' THEN
      RETURN normalized;
    END IF;
  END IF;
  RAISE EXCEPTION '额度工具 JSON 必须是对象，实际类型为 %', jsonb_typeof(document);
END
$function$;

CREATE OR REPLACE FUNCTION pg_temp.quota_rename_json_key(
  document JSONB,
  source_key TEXT,
  target_key TEXT
)
RETURNS JSONB
LANGUAGE plpgsql
IMMUTABLE
AS $function$
DECLARE
  source_value JSONB;
BEGIN
  IF NOT document ? source_key THEN
    RETURN document;
  END IF;
  source_value := document -> source_key;
  document := document - source_key;
  IF NOT document ? target_key THEN
    document := document || jsonb_build_object(target_key, source_value);
  END IF;
  RETURN document;
END
$function$;

CREATE OR REPLACE FUNCTION pg_temp.quota_normalize_plan_parameters(document JSONB)
RETURNS JSONB
LANGUAGE plpgsql
IMMUTABLE
AS $function$
DECLARE
  calculation_days TEXT;
BEGIN
  document := pg_temp.quota_json_object(document);

  IF NOT document ? 'calculation_days_hundred' THEN
    calculation_days := COALESCE(
      document ->> 'calculationDays',
      document ->> 'calculation_days'
    );
    IF calculation_days IS NOT NULL THEN
      document := document || jsonb_build_object(
        'calculation_days_hundred',
        ROUND(calculation_days::NUMERIC * 100)::BIGINT
      );
    END IF;
  END IF;
  document := document - 'calculationDays' - 'calculation_days';

  document := pg_temp.quota_rename_json_key(document, 'basisMode', 'basis_mode');
  document := pg_temp.quota_rename_json_key(document, 'totalWorkdays', 'total_workdays');
  document := pg_temp.quota_rename_json_key(document, 'remainingWorkdays', 'remaining_workdays');
  document := pg_temp.quota_rename_json_key(document, 'earlyReclaim', 'early_reclaim');
  document := pg_temp.quota_rename_json_key(document, 'reclaimCapPercent', 'reclaim_cap_percent');
  document := pg_temp.quota_rename_json_key(document, 'usageBonusPercent', 'usage_bonus_percent');
  document := pg_temp.quota_rename_json_key(document, 'thoroughRelease', 'thorough_release');
  RETURN document;
END
$function$;

CREATE OR REPLACE FUNCTION pg_temp.quota_normalize_calculation_data(document JSONB)
RETURNS JSONB
LANGUAGE plpgsql
IMMUTABLE
AS $function$
BEGIN
  document := pg_temp.quota_json_object(document);
  document := pg_temp.quota_rename_json_key(document, 'initialGrantQuota', 'initial_grant_quota');
  document := pg_temp.quota_rename_json_key(document, 'previousBalance', 'previous_balance');
  document := pg_temp.quota_rename_json_key(document, 'weeklyDemand', 'weekly_demand');
  document := pg_temp.quota_rename_json_key(document, 'periodSpend', 'period_spend');
  document := pg_temp.quota_rename_json_key(document, 'recentSpend', 'recent_spend');
  document := pg_temp.quota_rename_json_key(document, 'historicalDecrease', 'historical_decrease');
  document := pg_temp.quota_rename_json_key(
    document,
    'cumulativeReclaimPercent',
    'cumulative_reclaim_percent'
  );
  document := pg_temp.quota_rename_json_key(document, 'decreaseKind', 'decrease_kind');
  document := pg_temp.quota_rename_json_key(document, 'baseQuota', 'base_quota');
  document := pg_temp.quota_rename_json_key(document, 'bonusQuota', 'bonus_quota');
  document := pg_temp.quota_rename_json_key(document, 'weightedPoolQuota', 'weighted_pool_quota');

  IF document ->> 'decrease_kind' = 'finalLow' THEN
    document := jsonb_set(document, '{decrease_kind}', '"final_low"'::JSONB, false);
  END IF;
  RETURN document;
END
$function$;

-- 先规范 JSON 内容，再将 JSONB 转成 Go 模型使用的 TEXT。
DO $migration$
DECLARE
  column_type TEXT;
BEGIN
  SELECT data_type INTO column_type
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'tool_quota_adjustment_plans'
    AND column_name = 'parameters';

  IF column_type = 'jsonb' THEN
    UPDATE tool_quota_adjustment_plans
    SET parameters = pg_temp.quota_normalize_plan_parameters(parameters);
    ALTER TABLE tool_quota_adjustment_plans ALTER COLUMN parameters DROP DEFAULT;
    ALTER TABLE tool_quota_adjustment_plans
      ALTER COLUMN parameters TYPE TEXT USING parameters::TEXT;
    ALTER TABLE tool_quota_adjustment_plans ALTER COLUMN parameters SET DEFAULT '{}';
  ELSIF column_type = 'text' THEN
    UPDATE tool_quota_adjustment_plans
    SET parameters = pg_temp.quota_normalize_plan_parameters(parameters::JSONB)::TEXT;
  ELSE
    RAISE EXCEPTION 'parameters 类型应为 jsonb 或 text，实际为 %', column_type;
  END IF;

  SELECT data_type INTO column_type
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'tool_quota_adjustment_items'
    AND column_name = 'calculation_data';

  IF column_type = 'jsonb' THEN
    UPDATE tool_quota_adjustment_items
    SET calculation_data = pg_temp.quota_normalize_calculation_data(calculation_data);
    ALTER TABLE tool_quota_adjustment_items ALTER COLUMN calculation_data DROP DEFAULT;
    ALTER TABLE tool_quota_adjustment_items
      ALTER COLUMN calculation_data TYPE TEXT USING calculation_data::TEXT;
    ALTER TABLE tool_quota_adjustment_items ALTER COLUMN calculation_data SET DEFAULT '{}';
  ELSIF column_type = 'text' THEN
    UPDATE tool_quota_adjustment_items
    SET calculation_data = pg_temp.quota_normalize_calculation_data(calculation_data::JSONB)::TEXT;
  ELSE
    RAISE EXCEPTION 'calculation_data 类型应为 jsonb 或 text，实际为 %', column_type;
  END IF;
END
$migration$;

-- 该约束同时比较两个时间列，逐列改类型时会出现 TIMESTAMPTZ 与 BIGINT
-- 的短暂混合状态。转换前移除，两列完成后按原语义恢复。
ALTER TABLE tool_quota_cycles
  DROP CONSTRAINT IF EXISTS valid_cycle_duration;

-- 将源工具的 TIMESTAMPTZ 统一转换为 Unix 秒 BIGINT。
DO $migration$
DECLARE
  target RECORD;
  column_type TEXT;
BEGIN
  FOR target IN
    SELECT * FROM (VALUES
      ('tool_quota_cycles', 'cycle_start_at'),
      ('tool_quota_cycles', 'cycle_end_at'),
      ('tool_quota_cycles', 'created_at'),
      ('tool_quota_cycles', 'updated_at'),
      ('tool_quota_adjustment_plans', 'snapshot_at'),
      ('tool_quota_adjustment_plans', 'next_adjustment_at'),
      ('tool_quota_adjustment_plans', 'created_at'),
      ('tool_quota_adjustment_plans', 'executed_at'),
      ('tool_quota_adjustment_plans', 'cancelled_at'),
      ('tool_quota_adjustment_items', 'email_sent_at')
    ) AS columns_to_convert(table_name, column_name)
  LOOP
    SELECT data_type INTO column_type
    FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = target.table_name
      AND column_name = target.column_name;

    IF column_type = 'timestamp with time zone' THEN
      EXECUTE format(
        'ALTER TABLE %I ALTER COLUMN %I DROP DEFAULT',
        target.table_name,
        target.column_name
      );
      EXECUTE format(
        'ALTER TABLE %I ALTER COLUMN %I TYPE BIGINT USING FLOOR(EXTRACT(EPOCH FROM %I))::BIGINT',
        target.table_name,
        target.column_name,
        target.column_name
      );
    ELSIF column_type <> 'bigint' THEN
      RAISE EXCEPTION '%.% 类型应为 timestamptz 或 bigint，实际为 %',
        target.table_name, target.column_name, column_type;
    END IF;
  END LOOP;
END
$migration$;

ALTER TABLE tool_quota_cycles
  ADD CONSTRAINT valid_cycle_duration CHECK (cycle_end_at > cycle_start_at);

-- 旧工具的检查约束不认识当前周期结算和额度恢复类型。
-- GORM AutoMigrate 不会删除模型中未声明的旧约束，因此在这里显式替换。
ALTER TABLE tool_quota_adjustment_plans
  DROP CONSTRAINT IF EXISTS tool_quota_adjustment_plans_plan_type_check;
ALTER TABLE tool_quota_adjustment_plans
  ADD CONSTRAINT tool_quota_adjustment_plans_plan_type_check
  CHECK (plan_type IN ('initialization', 'adjustment', 'settlement'));

ALTER TABLE tool_quota_adjustment_items
  DROP CONSTRAINT IF EXISTS tool_quota_adjustment_items_action_check;
ALTER TABLE tool_quota_adjustment_items
  ADD CONSTRAINT tool_quota_adjustment_items_action_check
  CHECK (action IN ('initialize', 'increase', 'decrease', 'grant', 'reclaim', 'restore'));

-- 补齐目标模型新增的单活跃周期键和日志投递状态。
ALTER TABLE tool_quota_cycles
  ADD COLUMN IF NOT EXISTS active_key INTEGER;

UPDATE tool_quota_cycles
SET active_key = CASE WHEN status = 'active' THEN 1 ELSE NULL END
WHERE active_key IS DISTINCT FROM CASE WHEN status = 'active' THEN 1 ELSE NULL END;

ALTER TABLE tool_quota_adjustment_items
  ADD COLUMN IF NOT EXISTS log_status VARCHAR(20),
  ADD COLUMN IF NOT EXISTS log_sent_at BIGINT,
  ADD COLUMN IF NOT EXISTS log_error TEXT;

UPDATE tool_quota_adjustment_items AS item
SET
  log_status = CASE WHEN plan.status = 'executed' THEN 'sent' ELSE 'pending' END,
  log_sent_at = CASE WHEN plan.status = 'executed' THEN plan.executed_at ELSE NULL END
FROM tool_quota_adjustment_plans AS plan
WHERE item.plan_id = plan.id
  AND (item.log_status IS NULL OR item.log_status NOT IN ('pending', 'sent', 'failed', 'skipped'));

UPDATE tool_quota_adjustment_items
SET
  email_status = CASE WHEN action = 'reclaim' THEN 'skipped' ELSE email_status END,
  display_name = COALESCE(display_name, ''),
  email = COALESCE(email, ''),
  log_content = COALESCE(log_content, ''),
  log_error = COALESCE(log_error, ''),
  email_error = COALESCE(email_error, '');

UPDATE tool_quota_cycles
SET
  created_by = COALESCE(created_by, ''),
  updated_by = COALESCE(updated_by, '');

UPDATE tool_quota_adjustment_plans
SET
  created_by = COALESCE(created_by, ''),
  executed_by = COALESCE(executed_by, ''),
  cancelled_by = COALESCE(cancelled_by, ''),
  cancel_reason = COALESCE(cancel_reason, '');

-- 旧草稿使用迁移前的展示和生成上下文，保留记录但禁止迁入后直接执行。
UPDATE tool_quota_adjustment_plans
SET
  status = 'cancelled',
  cancelled_at = COALESCE(cancelled_at, FLOOR(EXTRACT(EPOCH FROM CURRENT_TIMESTAMP))::BIGINT),
  cancelled_by = 'migration',
  cancel_reason = '迁移至 new-api 后需按当前配置重新生成'
WHERE status = 'draft';

UPDATE tool_quota_adjustment_items AS item
SET
  log_status = 'skipped',
  log_sent_at = NULL,
  log_error = '',
  email_status = 'skipped',
  email_sent_at = NULL,
  email_error = ''
FROM tool_quota_adjustment_plans AS plan
WHERE item.plan_id = plan.id
  AND plan.status = 'cancelled'
  AND plan.cancelled_by = 'migration'
  AND plan.cancel_reason = '迁移至 new-api 后需按当前配置重新生成';

ALTER TABLE tool_quota_adjustment_items
  ALTER COLUMN log_status SET DEFAULT 'pending',
  ALTER COLUMN log_status SET NOT NULL,
  ALTER COLUMN log_error SET DEFAULT '',
  ALTER COLUMN log_error SET NOT NULL;

DO $migration$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'tool_quota_adjustment_items'::REGCLASS
      AND conname = 'chk_tool_quota_items_log_status'
  ) THEN
    ALTER TABLE tool_quota_adjustment_items
      ADD CONSTRAINT chk_tool_quota_items_log_status
      CHECK (log_status IN ('pending', 'sent', 'failed', 'skipped'));
  END IF;
END
$migration$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_quota_cycles_active_key
  ON tool_quota_cycles (active_key);
CREATE INDEX IF NOT EXISTS idx_tool_quota_cycles_status
  ON tool_quota_cycles (status);
CREATE INDEX IF NOT EXISTS idx_tool_quota_cycles_created_at
  ON tool_quota_cycles (created_at);
CREATE INDEX IF NOT EXISTS idx_tool_quota_items_log_status
  ON tool_quota_adjustment_items (log_status);
CREATE INDEX IF NOT EXISTS idx_tool_quota_items_email_status
  ON tool_quota_adjustment_items (email_status);
CREATE INDEX IF NOT EXISTS idx_tool_quota_adjustment_items_action
  ON tool_quota_adjustment_items (action);
CREATE INDEX IF NOT EXISTS idx_tool_quota_adjustment_plans_created_at
  ON tool_quota_adjustment_plans (created_at);

-- 复用源工具已有索引，避免 AutoMigrate 再创建等价索引。
DO $migration$
BEGIN
  IF to_regclass('idx_tool_quota_cycles_time') IS NULL
    AND to_regclass('idx_tool_cycles_time_range') IS NOT NULL THEN
    ALTER INDEX idx_tool_cycles_time_range RENAME TO idx_tool_quota_cycles_time;
  END IF;
  IF to_regclass('idx_tool_quota_plans_cycle_status') IS NULL
    AND to_regclass('idx_tool_plans_cycle_status') IS NOT NULL THEN
    ALTER INDEX idx_tool_plans_cycle_status RENAME TO idx_tool_quota_plans_cycle_status;
  END IF;
  IF to_regclass('idx_tool_quota_adjustment_plans_snapshot_at') IS NULL
    AND to_regclass('idx_tool_plans_snapshot') IS NOT NULL THEN
    ALTER INDEX idx_tool_plans_snapshot RENAME TO idx_tool_quota_adjustment_plans_snapshot_at;
  END IF;
  IF to_regclass('idx_tool_quota_adjustment_items_plan_id') IS NULL
    AND to_regclass('idx_tool_items_plan') IS NOT NULL THEN
    ALTER INDEX idx_tool_items_plan RENAME TO idx_tool_quota_adjustment_items_plan_id;
  END IF;
  IF to_regclass('idx_tool_quota_adjustment_items_user_id') IS NULL
    AND to_regclass('idx_tool_items_user') IS NOT NULL THEN
    ALTER INDEX idx_tool_items_user RENAME TO idx_tool_quota_adjustment_items_user_id;
  END IF;
END
$migration$;

-- 提交前验证目标模型依赖的关键不变量。
DO $migration$
DECLARE
  invalid_count BIGINT;
BEGIN
  IF EXISTS (
    SELECT 1 FROM tool_quota_cycles
    WHERE budget_quota <= 0 OR initial_grant_quota <= 0
  ) THEN
    RAISE EXCEPTION '存在采购总额或首次额度不大于 0 的周期，请先修正';
  END IF;

  IF EXISTS (
    SELECT 1 FROM tool_quota_cycles
    WHERE active_key IS DISTINCT FROM CASE WHEN status = 'active' THEN 1 ELSE NULL END
  ) THEN
    RAISE EXCEPTION 'active_key 与周期状态不一致';
  END IF;

  IF EXISTS (
    SELECT 1 FROM tool_quota_adjustment_plans
    WHERE jsonb_typeof(parameters::JSONB) <> 'object'
  ) THEN
    RAISE EXCEPTION '存在无法转换为目标 PlanParameters 的方案参数';
  END IF;

  IF EXISTS (
    SELECT 1 FROM tool_quota_adjustment_items
    WHERE jsonb_typeof(calculation_data::JSONB) <> 'object'
  ) THEN
    RAISE EXCEPTION '存在非对象类型的 calculation_data';
  END IF;

  IF EXISTS (
    SELECT 1 FROM tool_quota_adjustment_items
    WHERE log_status NOT IN ('pending', 'sent', 'failed', 'skipped')
      OR email_status NOT IN ('pending', 'sent', 'failed', 'skipped')
  ) THEN
    RAISE EXCEPTION '存在不支持的通知状态';
  END IF;

  IF EXISTS (
    SELECT 1 FROM tool_quota_adjustment_plans
    WHERE status = 'draft'
  ) THEN
    RAISE EXCEPTION '迁移后仍存在未作废草稿';
  END IF;

  SELECT COUNT(*) INTO invalid_count FROM tool_quota_cycles;
  RAISE NOTICE '额度周期迁移完成：% 条', invalid_count;
  SELECT COUNT(*) INTO invalid_count FROM tool_quota_adjustment_plans;
  RAISE NOTICE '额度方案迁移完成：% 条', invalid_count;
  SELECT COUNT(*) INTO invalid_count FROM tool_quota_adjustment_items;
  RAISE NOTICE '额度明细迁移完成：% 条', invalid_count;
  RAISE NOTICE '启动 new-api 后将自动补录旧消费；补录成功前不得生成或执行调配方案';
END
$migration$;

COMMIT;
