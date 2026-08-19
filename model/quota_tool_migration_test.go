package model

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const quotaToolLegacyFixtureSQL = `
CREATE TABLE options (
  "key" VARCHAR(255) PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT INTO options ("key", value) VALUES
  ('QuotaPerUnit', '500000'),
  ('USDExchangeRate', '1'),
  ('general_setting.quota_display_type', 'CNY');

CREATE TABLE logs (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  type INTEGER NOT NULL,
  quota INTEGER NOT NULL,
  request_id VARCHAR(64) NOT NULL
);

CREATE TABLE tool_quota_cycles (
  id SERIAL PRIMARY KEY,
  cycle_start_at TIMESTAMPTZ NOT NULL,
  cycle_end_at TIMESTAMPTZ NOT NULL,
  budget_quota BIGINT NOT NULL CHECK (budget_quota > 0),
  initial_grant_quota BIGINT NOT NULL CHECK (initial_grant_quota >= 0),
  initial_stage_percent INTEGER NOT NULL DEFAULT 6500 CHECK (initial_stage_percent >= 0 AND initial_stage_percent <= 10000),
  status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed', 'scheduled')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by VARCHAR(255),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_by VARCHAR(255),
  CONSTRAINT unique_cycle_start UNIQUE (cycle_start_at),
  CONSTRAINT valid_cycle_duration CHECK (cycle_end_at > cycle_start_at)
);
CREATE UNIQUE INDEX idx_tool_cycles_single_active ON tool_quota_cycles (status) WHERE status = 'active';
CREATE INDEX idx_tool_cycles_time_range ON tool_quota_cycles (cycle_start_at, cycle_end_at);

CREATE TABLE tool_quota_adjustment_plans (
  id SERIAL PRIMARY KEY,
  cycle_id INTEGER NOT NULL REFERENCES tool_quota_cycles(id),
  plan_type VARCHAR(20) NOT NULL CHECK (plan_type IN ('initialization', 'adjustment')),
  stage_percent INTEGER NOT NULL CHECK (stage_percent >= 0 AND stage_percent <= 10000),
  snapshot_at TIMESTAMPTZ NOT NULL,
  next_adjustment_at TIMESTAMPTZ,
  algorithm_version VARCHAR(50) NOT NULL,
  parameters JSONB NOT NULL DEFAULT '{}',
  budget_quota_snapshot BIGINT NOT NULL,
  total_spend_quota BIGINT NOT NULL,
  managed_balance_quota BIGINT NOT NULL,
  planned_delta_quota BIGINT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'executed', 'cancelled')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by VARCHAR(255),
  executed_at TIMESTAMPTZ,
  executed_by VARCHAR(255),
  cancelled_at TIMESTAMPTZ,
  cancelled_by VARCHAR(255),
  cancel_reason TEXT
);
CREATE INDEX idx_tool_plans_cycle_status ON tool_quota_adjustment_plans (cycle_id, status);
CREATE INDEX idx_tool_plans_snapshot ON tool_quota_adjustment_plans (snapshot_at);

CREATE TABLE tool_quota_adjustment_items (
  id SERIAL PRIMARY KEY,
  plan_id INTEGER NOT NULL REFERENCES tool_quota_adjustment_plans(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL,
  username VARCHAR(255) NOT NULL,
  display_name VARCHAR(255),
  email VARCHAR(255),
  action VARCHAR(20) NOT NULL CHECK (action IN ('initialize', 'increase', 'decrease', 'grant', 'reclaim')),
  snapshot_balance_quota BIGINT NOT NULL,
  adjustment_quota BIGINT NOT NULL,
  retained_quota BIGINT NOT NULL DEFAULT 0,
  calculation_data JSONB NOT NULL DEFAULT '{}',
  basis_text TEXT NOT NULL,
  actual_before_quota BIGINT,
  actual_after_quota BIGINT,
  log_content TEXT,
  email_status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (email_status IN ('pending', 'sent', 'failed', 'skipped')),
  email_sent_at TIMESTAMPTZ,
  email_error TEXT,
  CONSTRAINT unique_plan_user UNIQUE (plan_id, user_id)
);
CREATE INDEX idx_tool_items_plan ON tool_quota_adjustment_items (plan_id);
CREATE INDEX idx_tool_items_user ON tool_quota_adjustment_items (user_id);
CREATE INDEX idx_tool_items_email_status ON tool_quota_adjustment_items (plan_id, email_status);

INSERT INTO tool_quota_cycles (
  cycle_start_at, cycle_end_at, budget_quota, initial_grant_quota, status
) VALUES
  (TO_TIMESTAMP(1), TO_TIMESTAMP(1000), 1000, 100, 'closed'),
  (TO_TIMESTAMP(1000), TO_TIMESTAMP(2000), 1000, 100, 'active');

INSERT INTO tool_quota_adjustment_plans (
  cycle_id, plan_type, stage_percent, snapshot_at, algorithm_version, parameters,
  budget_quota_snapshot, total_spend_quota, managed_balance_quota, planned_delta_quota,
  status, executed_at
) VALUES (
  2, 'adjustment', 8500, TO_TIMESTAMP(1500), '1.6.1',
  '{"basisMode":"actual","calculationDays":1,"totalWorkdays":1,"remainingWorkdays":1}',
  1000, 100, 200, 10, 'executed', TO_TIMESTAMP(1501)
);

INSERT INTO tool_quota_adjustment_items (
  plan_id, user_id, username, action, snapshot_balance_quota, adjustment_quota,
  retained_quota, calculation_data, basis_text, actual_before_quota,
  actual_after_quota, log_content, email_status
) VALUES (
  1, 1, 'legacy-user', 'increase', 100, 10, 110,
  '{"baseQuota":"10"}', 'legacy basis', 100, 110, 'legacy log', 'sent'
);

INSERT INTO logs (user_id, created_at, type, quota, request_id) VALUES
  (1, 500, 2, 50, 'closed-legacy'),
  (1, 1200, 2, 100, 'active-legacy');
`

func quotaToolMigrationPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"postgres", "postgresql"}, parsed.Scheme)

	adminDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN: dsn, PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	schemaName := fmt.Sprintf("quota_tool_migration_%d", time.Now().UnixNano())
	require.NoError(t, adminDB.Exec(`CREATE SCHEMA "`+schemaName+`"`).Error)

	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	testDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN: parsed.String(), PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	testSQLDB, err := testDB.DB()
	require.NoError(t, err)
	adminSQLDB, err := adminDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = testSQLDB.Close()
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error
		_ = adminSQLDB.Close()
	})

	require.NoError(t, testDB.Exec(quotaToolLegacyFixtureSQL).Error)
	migrationSQL, err := os.ReadFile(filepath.Join("..", "bin", "migration_quota_tool_to_new_api.sql"))
	require.NoError(t, err)
	require.NoError(t, testDB.Exec(string(migrationSQL)).Error)
	require.NoError(t, testDB.AutoMigrate(&QuotaCycle{}, &QuotaPlan{}, &QuotaItem{}, &QuotaCycleSettlement{}))

	previousDB := DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	DB = testDB
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, common.DatabaseTypePostgreSQL)
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	return testDB
}

func quotaToolConstraintDefinition(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var definition string
	require.NoError(t, db.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = ?`, name).Scan(&definition).Error)
	return definition
}

func restoreLegacyQuotaToolConstraints(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("ALTER TABLE tool_quota_adjustment_plans DROP CONSTRAINT tool_quota_adjustment_plans_plan_type_check").Error)
	require.NoError(t, db.Exec("ALTER TABLE tool_quota_adjustment_plans ADD CONSTRAINT tool_quota_adjustment_plans_plan_type_check CHECK (plan_type IN ('initialization', 'adjustment'))").Error)
	require.NoError(t, db.Exec("ALTER TABLE tool_quota_adjustment_items DROP CONSTRAINT tool_quota_adjustment_items_action_check").Error)
	require.NoError(t, db.Exec("ALTER TABLE tool_quota_adjustment_items ADD CONSTRAINT tool_quota_adjustment_items_action_check CHECK (action IN ('initialize', 'increase', 'decrease', 'grant', 'reclaim'))").Error)
}

func TestQuotaToolMigrationBackfillsSettlementsAndUpdatesConstraints(t *testing.T) {
	db := quotaToolMigrationPostgres(t)
	assert.Contains(t, quotaToolConstraintDefinition(t, db, "tool_quota_adjustment_plans_plan_type_check"), "settlement")
	assert.Contains(t, quotaToolConstraintDefinition(t, db, "tool_quota_adjustment_items_action_check"), "restore")

	restoreLegacyQuotaToolConstraints(t, db)
	require.NoError(t, db.Create(&QuotaCycleSettlement{
		BusinessKey: quotaToolLegacyLogPrefix + "2", CycleId: 2, UserId: 1,
		BillingAt: 1200, Quota: 100, UpdatedAt: 1600,
	}).Error)
	require.NoError(t, db.Create(&QuotaCycleSettlement{
		BusinessKey: "active-current", CycleId: 2, UserId: 2,
		BillingAt: 1600, Quota: 30, UpdatedAt: 1600,
	}).Error)

	require.NoError(t, migrateQuotaToolData())
	assert.False(t, db.Migrator().HasColumn(&QuotaCycle{}, "initial_stage_percent"))
	assert.Contains(t, quotaToolConstraintDefinition(t, db, "tool_quota_adjustment_plans_plan_type_check"), "settlement")
	assert.Contains(t, quotaToolConstraintDefinition(t, db, "tool_quota_adjustment_items_action_check"), "restore")

	var migratedCount int64
	var migratedSpend int64
	require.NoError(t, db.Model(&QuotaCycleSettlement{}).
		Where("business_key LIKE ?", quotaToolLegacyLogPrefix+"%").Count(&migratedCount).Error)
	require.NoError(t, db.Model(&QuotaCycleSettlement{}).
		Select("COALESCE(SUM(quota), 0)").
		Where("business_key LIKE ?", quotaToolLegacyLogPrefix+"%").Scan(&migratedSpend).Error)
	assert.Equal(t, int64(2), migratedCount)
	assert.Equal(t, int64(150), migratedSpend)

	plan := QuotaPlan{
		CycleId: 2, PlanType: QuotaPlanTypeSettlement, StagePercent: 10_000,
		SnapshotAt: 1800, AlgorithmVersion: "1.8", Parameters: "{}",
		BudgetQuotaSnapshot: 1000, TotalSpendQuota: 130, ManagedBalanceQuota: 100,
		PlannedDeltaQuota: -100, Status: QuotaPlanStatusDraft, CreatedAt: 1800,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&QuotaItem{
		PlanId: plan.Id, UserId: 1, Username: "legacy-user", Action: QuotaAdjustmentActionRestore,
		SnapshotBalanceQuota: 0, AdjustmentQuota: 100, RetainedQuota: 100,
		CalculationData: "{}", BasisText: "restore", LogStatus: QuotaNotificationStatusPending,
		EmailStatus: QuotaNotificationStatusPending,
	}).Error)

	require.NoError(t, migrateQuotaToolData())
	var countAfterSecondRun int64
	require.NoError(t, db.Model(&QuotaCycleSettlement{}).
		Where("business_key LIKE ?", quotaToolLegacyLogPrefix+"%").Count(&countAfterSecondRun).Error)
	assert.Equal(t, migratedCount, countAfterSecondRun)
}

func TestQuotaToolMigrationRollsBackOnInvalidLegacySpend(t *testing.T) {
	db := quotaToolMigrationPostgres(t)
	restoreLegacyQuotaToolConstraints(t, db)
	require.NoError(t, db.Exec(`INSERT INTO logs (user_id, created_at, type, quota, request_id)
		VALUES (1, 1300, 2, -1, 'invalid-negative')`).Error)

	err := migrateQuotaToolData()
	require.ErrorContains(t, err, "负额度或无效用户")
	assert.True(t, db.Migrator().HasColumn(&QuotaCycle{}, "initial_stage_percent"))
	assert.NotContains(t, quotaToolConstraintDefinition(t, db, "tool_quota_adjustment_plans_plan_type_check"), "settlement")
	assert.NotContains(t, quotaToolConstraintDefinition(t, db, "tool_quota_adjustment_items_action_check"), "restore")

	var migratedCount int64
	require.NoError(t, db.Model(&QuotaCycleSettlement{}).
		Where("business_key LIKE ?", quotaToolLegacyLogPrefix+"%").Count(&migratedCount).Error)
	assert.Zero(t, migratedCount)
}
