package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
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
			profile, err := model.GetSelfUserById(user.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.whitelist, profile.QuotaWhitelist)
			data, err := buildSelfUserData(profile)
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

func TestTemporaryQuotaEligibilityAcrossSelfLoginAndRefresh(t *testing.T) {
	for _, whitelist := range []bool{false, true} {
		name := "eligible"
		if whitelist {
			name = "whitelisted"
		}
		t.Run(name, func(t *testing.T) {
			user, identity := setupSecurityEnrollmentTest(t)
			previous := operation_setting.CycleQuotaManagementEnabled
			operation_setting.CycleQuotaManagementEnabled = true
			t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previous })
			require.NoError(t, model.DB.AutoMigrate(&model.QuotaCycle{}))
			now := time.Now().Unix()
			require.NoError(t, model.DB.Create(&model.QuotaCycle{Status: model.QuotaCycleStatusActive, CycleStartAt: now - 60, CycleEndAt: now + 3600}).Error)
			require.NoError(t, model.DB.Model(user).Update("quota_whitelist", whitelist).Error)
			self := securityEnrollmentRequest(http.MethodGet, "/api/user/self", "", "", identity, GetSelf)
			var selfResult struct {
				Success bool                   `json:"success"`
				Data    map[string]interface{} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(self.Body.Bytes(), &selfResult))
			require.True(t, selfResult.Success, self.Body.String())
			assert.Equal(t, !whitelist, selfResult.Data["temporary_quota_request_eligible"])
			bundle, err := service.CreateLoginSession(user.Id, "totp", "127.0.0.1", "quota-eligibility")
			require.NoError(t, err)
			login := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(login)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login/verify", nil)
			completeVerifiedLoginResponse(c, bundle, service.VerificationMethodTwoFA)
			var result struct {
				Success bool `json:"success"`
				Data    struct {
					User map[string]interface{} `json:"user"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(login.Body.Bytes(), &result))
			require.True(t, result.Success, login.Body.String())
			assert.Equal(t, !whitelist, result.Data.User["temporary_quota_request_eligible"])
			refresh := httptest.NewRecorder()
			c, _ = gin.CreateTestContext(refresh)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
			c.Request.AddCookie(&http.Cookie{Name: service.RefreshCookieName, Value: bundle.RefreshToken})
			RefreshAuth(c)
			require.NoError(t, common.Unmarshal(refresh.Body.Bytes(), &result))
			require.True(t, result.Success, refresh.Body.String())
			assert.Equal(t, !whitelist, result.Data.User["temporary_quota_request_eligible"])
			assert.NotContains(t, result.Data.User, "password")
			assert.NotContains(t, result.Data.User, "access_token")
		})
	}
}

func TestTemporaryQuotaLookupFailureDoesNotLeaveLoginSession(t *testing.T) {
	for _, verified := range []bool{false, true} {
		name := "first factor"
		if verified {
			name = "verified login"
		}
		t.Run(name, func(t *testing.T) {
			user, _ := setupSecurityEnrollmentTest(t)
			previous := operation_setting.CycleQuotaManagementEnabled
			operation_setting.CycleQuotaManagementEnabled = true
			t.Cleanup(func() { operation_setting.CycleQuotaManagementEnabled = previous })
			// The unavailable cycle table forces the real eligibility query to fail.
			require.False(t, model.DB.Migrator().HasTable(&model.QuotaCycle{}))
			var before int64
			require.NoError(t, model.DB.Model(&model.UserSession{}).Where("status = ?", model.UserSessionStatusActive).Count(&before).Error)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", nil)
			if verified {
				bundle, err := service.CreateLoginSession(user.Id, "totp", "127.0.0.1", "quota-query-failure")
				require.NoError(t, err)
				completeVerifiedLoginResponse(c, bundle, service.VerificationMethodTwoFA)
			} else {
				setupLoginAtAuthVersion(user, user.AuthVersion, c)
			}
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, http.StatusInternalServerError, recorder.Code)
			assert.JSONEq(t, `{"success":false,"code":"AUTH_INTERNAL_ERROR","message":"Internal Server Error"}`, recorder.Body.String())
			assert.Empty(t, recorder.Result().Cookies())
			var after int64
			require.NoError(t, model.DB.Model(&model.UserSession{}).Where("status = ?", model.UserSessionStatusActive).Count(&after).Error)
			assert.Equal(t, before, after)
		})
	}
}
