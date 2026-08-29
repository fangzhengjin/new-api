package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfUserTemporaryQuotaMenuEligibility(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.QuotaCycle{}, &model.TemporaryQuotaRequest{}))
	previous := operation_setting.CycleQuotaManagementEnabled
	t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previous })
	user := &model.User{Username: "quota-menu-user", AffCode: "quota-menu-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	now := time.Now().Unix()
	cycle := &model.QuotaCycle{Status: model.QuotaCycleStatusActive, CycleStartAt: now - 60, CycleEndAt: now + 3600}
	require.NoError(t, db.Create(cycle).Error)
	for _, tc := range []struct {
		name               string
		enabled, whitelist bool
		status             int
		end                int64
		want               bool
	}{
		{"feature disabled", false, false, common.UserStatusEnabled, now + 3600, false},
		{"active with exhausted reserve", true, false, common.UserStatusEnabled, now + 3600, true},
		{"expired cycle", true, false, common.UserStatusEnabled, now - 1, false},
		{"whitelisted", true, true, common.UserStatusEnabled, now + 3600, false},
		{"disabled account", true, false, common.UserStatusDisabled, now + 3600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.CycleQuotaManagementEnabled = tc.enabled
			require.NoError(t, db.Model(cycle).Update("cycle_end_at", tc.end).Error)
			require.NoError(t, db.Model(user).Updates(map[string]interface{}{"quota_whitelist": tc.whitelist, "status": tc.status}).Error)
			data, err := buildSelfUserData(user)
			require.NoError(t, err)
			assert.Equal(t, tc.want, data["temporary_quota_request_eligible"])
			assert.Equal(t, user.Id, data["id"])
		})
	}
	operation_setting.CycleQuotaManagementEnabled = true
	require.NoError(t, db.Migrator().DropTable(&model.QuotaCycle{}))
	_, err := buildSelfUserData(user)
	require.ErrorContains(t, err, "查询临时额度菜单资格失败")
}

func TestSelfTemporaryQuotaResponseOmitsInternalReviewFields(t *testing.T) {
	planID := 12
	response := selfTemporaryQuotaRequestResponse(model.TemporaryQuotaRequest{
		Id: 7, CycleId: 3, UserId: 9, Username: "private-user", DisplayName: "Private User",
		RequestedQuota: 100, Project: "Project A", Reason: "delivery", Status: model.TemporaryQuotaRequestStatusExecuted,
		Decision: model.TemporaryQuotaDecisionAuto, ApprovedQuota: 80, PlanId: &planID,
		ReviewedBy: "system", ReviewReason: "approved", CreatedAt: 1,
	})
	data, err := common.Marshal(response)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(data, &payload))

	assert.Equal(t, float64(7), payload["id"])
	assert.Equal(t, "100", payload["requested_quota"])
	for _, field := range []string{"cycle_id", "user_id", "username", "display_name", "decision", "plan_id", "reviewed_by"} {
		assert.NotContains(t, payload, field)
	}
}
